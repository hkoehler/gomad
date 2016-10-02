// Copyright (C) 2016, Heiko Koehler
// define different kinds of HTTP request handlers
package main

import (
	"errors"
	"fmt"
	"github.com/wcharczuk/go-chart"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"math"
)

var Registry = make(map[string]Handler)

// Registry entry is HTTP handler with path
type Handler interface {
	http.Handler
	// execute handler
	// might generate stats and store them in time series logs
	Execute()
	Path() string
	Name() string
	PollInterval() time.Duration
}

// Common implementation of registry entry for commands
type HandlerImpl struct {
	path         string
	name         string
	pollInterval time.Duration
}

func (entry HandlerImpl) Path() string {
	return entry.path
}

func (entry HandlerImpl) Name() string {
	return entry.name
}

func (entry HandlerImpl) PollInterval() time.Duration {
	return entry.pollInterval
}

// register handler
func RegisterHandler(entry Handler) {
	Registry[entry.Path()] = entry
	http.Handle(entry.Path(), entry)
	// handle sub pages for generated content
	http.Handle(entry.Path()+"/", entry)
}

// HTTP handler executing command line
type CommandHandler struct {
	HandlerImpl
	CmdLine    string
	Regex      *regexp.Regexp
	Submatches []string
	// map properties to time series
	TS map[string]*TimeSeriesTable
}

func NewCommandHandler(conf HandlerConfig) (Handler, error) {
	if re, err := regexp.Compile(conf.Regex); err != nil {
		return nil, err
	} else {
		var pollInterval time.Duration
		var tsMap = make(map[string]*TimeSeriesTable)
		var err error

		if conf.PollInterval != "" {
			if pollInterval, err = time.ParseDuration(conf.PollInterval); err != nil {
				return nil, err
			}
		}

		if conf.TimeSeries != nil {
			for _, prop := range conf.Submatches {
				tsPath := filepath.Join(os.TempDir(), "mad", conf.URL, prop)
				if ts, err := NewTimeSeriesTable(tsPath, conf.TimeSeries); err != nil {
					return nil, err
				} else {
					tsMap[prop] = ts
				}
			}
		}
		return &CommandHandler{HandlerImpl: HandlerImpl{conf.URL, conf.Name, pollInterval},
				CmdLine: conf.Cmd, Regex: re, Submatches: conf.Submatches, TS: tsMap},
			nil
	}
}

func NewHandler(conf HandlerConfig) (Handler, error) {
	if conf.Type == "" {
		conf.Type = "command"
	}
	switch strings.ToLower(conf.Type) {
	case "command":
		return NewCommandHandler(conf)
	}
	return nil, errors.New(fmt.Sprintf("Unknown handler type %s", conf.Type))
}

func (handler CommandHandler) Stat() (string, map[string]string) {
	cmd := strings.Split(handler.CmdLine, " ")
	if cmdOut, err := exec.Command(cmd[0], cmd[1:]...).Output(); err == nil {
		var attrs = make(map[string]string)

		out := string(cmdOut)
		if handler.Regex.MatchString(out) {
			subMatches := handler.Regex.FindStringSubmatch(out)
			subMatches = subMatches[1:]
			for i, m := range subMatches {
				attrs[handler.Submatches[i]] = m
			}
		}
		return out, attrs
	} else {
		return fmt.Sprintf("Error executing command line \"%s\": %v\n", handler.CmdLine, err), nil
	}
}

// query properties and store them in time series logs
func (handler CommandHandler) Execute() {
	_, props := handler.Stat()
	for key, val := range props {
		var floatVal float64

		// map property name to time series
		ts := handler.TS[key]
		fmt.Sscanf(val, "%f", &floatVal)
		ts.Add(floatVal)
	}
}

func (handler CommandHandler) ServeChart(w http.ResponseWriter, req *http.Request, prop string) {
	xvalues := make([]time.Time, 0)
	yvalues := make([]float64, 0)
	var min, max float64 = 0xFFFFFFFF, 0

	// look up time series by name
	tbl := handler.TS[prop]
	if data, err := tbl.TopLevel().ReadAll(); err == nil {
		for _, dp := range data {
			xvalues = append(xvalues, dp.Tstamp)
			yvalues = append(yvalues, dp.Val)
			max = math.Max(max, dp.Val)
			min = math.Min(min, dp.Val)
		}
	}
	if min == max {
		max = min+1	
	}

	graph := chart.Chart{
		XAxis: chart.XAxis{
			Style: chart.Style{Show: true},
		},
		YAxis: chart.YAxis{
			Style: chart.Style{Show: true},
			Range: &chart.ContinuousRange{Min: 0, Max: max},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				Name: prop,
				Style: chart.Style{
					Show:        true,
					StrokeColor: chart.GetDefaultColor(0),
				},
				XValues: xvalues,
				YValues: yvalues,
			},
		},
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	graph.Render(chart.SVG, w)
}

func (handler CommandHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	type Chart struct {
		Path     string
		Property string
	}

	type Page struct {
		Cmd             string
		FirstLine       string
		AdditionalLines []string
		Charts          []Chart
	}

	if relPath, err := filepath.Rel(handler.Path(), req.URL.Path); err == nil {
		fmt.Printf("URL=%s, relPath=%s\n", req.URL.Path, relPath)
		if relPath != "." {
			handler.ServeChart(w, req, relPath)
			return
		}
	}

	const tmplStr = `
		<!DOCTYPE html>
		<html>
			<head>
			<title> {{.Cmd}} </title>
			</head>
			<body>
				<h1 style="text-align:center"> {{.Cmd}} </h1>
				<table border="line" style="width:100%">
					<caption> {{.Cmd}} Output </caption>
					<tr> 
						<td text-align: left>
						<code> {{ .FirstLine }} </code>
						{{range .AdditionalLines}} <br> <code> {{.}} </code> {{end}}
						</td>
					</tr>
				</table>
				{{range .Charts}}
				<h2 style="text-align:center"> {{.Property}} </h2>
				<img src="{{.Path}}" alt="{{.Property}}" style="width:100%"> <br>
				{{end}}
			</body>
		</html>
	`

	out, props := handler.Stat()
	charts := make([]Chart, 0)
	for prop, _ := range props {
		imgPath := filepath.Join(handler.Path(), prop)
		charts = append(charts, Chart{Path: imgPath, Property: prop})
	}
	lines := strings.Split(out, "\n")
	page := Page{Cmd: handler.CmdLine,
		FirstLine: lines[0],
		Charts:    charts}
	if len(lines) > 1 {
		page.AdditionalLines = lines[1:]
	}
	if tmpl, err := template.New("command").Parse(tmplStr); err != nil {
		log.Fatal(err)
	} else if err := tmpl.Execute(w, page); err != nil {
		log.Fatal(err)
	}

	/*fmt.Fprint(w, out)
	fmt.Fprint(w, "\n")
	// iterate thru time series associated with properties
	for key, currVal := range props {
		fmt.Fprintf(w, "\"%s\"\n", key)
		tbl := handler.TS[key]
		// iterate thru all time series levels
		for i, ts := range tbl.TS {
			fmt.Fprintf(w, "Level: %d\n", i)
			if data, err := ts.ReadAll(); err == nil {
				for _, dp := range data {
					fmt.Fprintf(w, "\"%v\" = \"%v\"\n", dp.Tstamp, dp.Val)
				}
			}
		}
		fmt.Fprintf(w, "\"%s\" = \"%s\"\n", key, currVal)
	}*/
}

// HTTP handler displaying config
type ConfigHandler struct {
	HandlerImpl
	ConfigPath string
}

func NewConfigHandler(path string, configPath string) Handler {
	return &ConfigHandler{HandlerImpl: HandlerImpl{path, "Config", 0},
		ConfigPath: configPath}
}

func (handler ConfigHandler) Execute() {
}

func (handler ConfigHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if f, err := os.Open(handler.ConfigPath); err == nil {
		io.Copy(w, f)
	} else {
		fmt.Fprintf(w, "Couldn't open %s: %v", handler.Path, err)
	}
}

// HTTP handler listing all registered handlers
type RootHandler struct {
	HandlerImpl
}

func NewRootHandler() Handler {
	return &RootHandler{HandlerImpl: HandlerImpl{"/", "Root", 0}}
}

func (handler RootHandler) Execute() {
}

type Entry struct {
	Path, Name string
}

// implement sort interface on []Entry
type ByName []Entry

func (a ByName) Len() int           { return len(a) }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByName) Less(i, j int) bool { return a[i].Path < a[j].Path }

// root handler listing all other handlers
func (handler RootHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	entries := make([]Entry, 0, len(Registry))
	for path, entry := range Registry {
		entries = append(entries, Entry{path, entry.Name()})
	}

	sort.Sort(ByName(entries))
	const tmplStr = `
		<html>
			<head>
			<title> Registered Commands </title>
			</head>
			<body>
				<h1> Registered Commands </h1>
				<div> </div>
				{{range .}} <a href="{{.Path}}"> {{.Name}} </a> <br>
				{{end}}
			</body>
		</html>
	`
	if tmpl, err := template.New("index").Parse(tmplStr); err != nil {
		log.Fatal(err)
	} else if err := tmpl.Execute(w, entries); err != nil {
		log.Fatal(err)
	}
}
