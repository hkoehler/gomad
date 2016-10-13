// Copyright (C) 2016, Heiko Koehler
// define different kinds of HTTP request handlers
package main

import (
	"errors"
	"fmt"
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

// Property definition w/ regex
type Property struct {
	Regex *regexp.Regexp
	TS    *TimeSeriesTable
}

// HTTP handler executing command line
type CommandHandler struct {
	HandlerImpl
	CmdLine string
	// map property name to regex and time series
	Properties map[string]Property
	Charts     []ChartConfig
}

// compile regular expression and create time series tables
func NewCommandHandler(conf HandlerConfig) (Handler, error) {
	var pollInterval time.Duration
	var propMap = make(map[string]Property)

	for _, propConfig := range conf.Properties {
		if re, err := regexp.Compile(propConfig.Regex); err != nil {
			return nil, err
		} else {
			prop := propConfig.Name
			tsPath := filepath.Join(os.TempDir(), "mad", conf.URL, prop)
			tsProps := []TimeSeriesProps{{60, 300}, {60, 300}, {60, 240}}
			if ts, err := NewTimeSeriesTable(tsPath, tsProps); err != nil {
				return nil, err
			} else {
				propMap[prop] = Property{Regex: re, TS: ts}
			}
		}
	}

	if conf.PollInterval != "" {
		var err error

		pollInterval, err = time.ParseDuration(conf.PollInterval)
		if err != nil {
			return nil, err
		}
	}
	return &CommandHandler{HandlerImpl: HandlerImpl{conf.URL, conf.Name, pollInterval},
			CmdLine: conf.Cmd, Properties: propMap, Charts: conf.Charts},
		nil
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
	var err error
	// map property name to current value
	var props = make(map[string]string)
	// command output
	var out []byte

	// execute command
	cmd := strings.Split(handler.CmdLine, " ")
	out, err = exec.Command(cmd[0], cmd[1:]...).Output()
	if err != nil {
		return fmt.Sprintf("Error executing command line \"%s\": %v\n", handler.CmdLine, err), nil
	}

	// parse/grep property values from command output
	lines := strings.Split(string(out), "\n")
	for name, prop := range handler.Properties {
		for _, line := range lines {
			subMatches := prop.Regex.FindStringSubmatch(line)
			// we only expect one group in regex
			if len(subMatches) == 2 {
				props[name] = subMatches[1]
			}
		}
	}
	return string(out), props
}

// query properties and store them in time series logs
func (handler CommandHandler) Execute() {
	_, props := handler.Stat()
	fmt.Println(props)
	for key, val := range props {
		var floatVal float64

		// map property name to time series
		prop := handler.Properties[key]
		fmt.Sscanf(val, "%f", &floatVal)
		prop.TS.Add(floatVal)
	}
}

func (handler CommandHandler) ServeChart(w http.ResponseWriter, req *http.Request, chartName string) {
	w.Header().Set("Content-Type", "image/svg+xml")
	var ts = make([]*TimeSeries, 0)
	var legend = make([]string, 0)

	for _, chart := range handler.Charts {
		if chart.Name == chartName {
			for _, prop := range chart.Properties {
				ts = append(ts, handler.Properties[prop].TS.TopLevel())
				legend = append(legend, prop)
			}
			break
		}
	}
	PlotTimeSeries(w, ts, legend)
}

func (handler CommandHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	type Chart struct {
		Path string
		Name string
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
				<h2 style="text-align:center"> {{.Name}} </h2>
				<img src="{{.Path}}" alt="{{.Name}}" style="width:100%"> <br>
				{{end}}
			</body>
		</html>
	`

	out, _ := handler.Stat()
	charts := make([]Chart, 0)
	for _, chart := range handler.Charts {
		imgPath := filepath.Join(handler.Path(), chart.Name)
		charts = append(charts, Chart{Path: imgPath, Name: chart.Name})
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

const (
	USER      = iota
	NICE      = iota
	SYSTEM    = iota
	IDLE      = iota
	IOWAIT    = iota
	IRQ       = iota
	SOFTIRQ   = iota
	NUM_STATS = iota
)

type SystemLoad struct {
	Stats [NUM_STATS]uint32
}

type RelativeSystemLoad struct {
	Stats [NUM_STATS]float64
}

func NewSystemLoad() (res SystemLoad) {
	if f, err := os.Open("/proc/stat"); err == nil {
		defer f.Close()
		fmt.Fscanf(f, "cpu")
		for i := 0; i < NUM_STATS; i++ {
			fmt.Fscanf(f, "%d", &res.Stats[i])
		}
	} else {
		log.Fatal(err)
	}
	return
}

func (curr SystemLoad) Diff(prev SystemLoad) (res SystemLoad) {
	for i := range curr.Stats {
		res.Stats[i] = curr.Stats[i] - prev.Stats[i]
	}
	return
}

func (curr SystemLoad) Total() (total uint32) {
	for _, c := range curr.Stats {
		total += c
	}
	return
}

func (curr SystemLoad) ToRelative() (res RelativeSystemLoad) {
	t := curr.Total()
	for i, stat := range curr.Stats {
		res.Stats[i] = float64(stat) / float64(t)
	}
	return
}

type CPULoadHandler struct {
	HandlerImpl
	UserTS   *TimeSeriesTable
	SystemTS *TimeSeriesTable
	IdleTS   *TimeSeriesTable
	Load     SystemLoad
}

func timeSeriesPath(url, prop string) string {
	return filepath.Join(os.TempDir(), "mad", url, prop)
}

func NewCPULoadHandler() (Handler, error) {
	var userTS, systemTS, idleTS *TimeSeriesTable
	var url = "/cpu"
	var err error

	tsProps := []TimeSeriesProps{{60, 300}, {60, 300}, {60, 240}}
	if userTS, err = NewTimeSeriesTable(timeSeriesPath(url, "user"), tsProps); err != nil {
		return nil, err
	}
	if systemTS, err = NewTimeSeriesTable(timeSeriesPath(url, "system"), tsProps); err != nil {
		return nil, err
	}
	if idleTS, err = NewTimeSeriesTable(timeSeriesPath(url, "idle"), tsProps); err != nil {
		return nil, err
	}
	pollInterval, _ := time.ParseDuration("1s")
	return &CPULoadHandler{HandlerImpl: HandlerImpl{"/cpu", "CPU Load", pollInterval},
		UserTS: userTS, SystemTS: systemTS, IdleTS: idleTS,
		Load: NewSystemLoad()}, nil
}

func (handler *CPULoadHandler) Execute() {
	curr := NewSystemLoad()
	diff := curr.Diff(handler.Load)
	rd := diff.ToRelative()
	//fmt.Printf("user=%f, system=%f, idle=%f, total=%d\n",
	//	rd.Stats[USER], rd.Stats[SYSTEM], rd.Stats[IDLE], diff.Total())
	handler.Load = curr

	if err := handler.UserTS.Add(rd.Stats[USER]); err != nil {
		log.Fatal(err)
	}
	if err := handler.SystemTS.Add(rd.Stats[SYSTEM]); err != nil {
		log.Fatal(err)
	}
	if err := handler.IdleTS.Add(rd.Stats[IDLE]); err != nil {
		log.Fatal(err)
	}
}

func (handler *CPULoadHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	const tmplStr = `
		<!DOCTYPE html>
		<html>
			<head>
			<title> CPU Load </title>
			</head>
			<body>
				<h1 style="text-align:center"> CPU Load </h1>
				<h2 style="text-align:center"> Last 5 Minutes </h2>
				<img src="{{.}}/2" alt="Idle" style="width:100%"> <br>
				<h2 style="text-align:center"> Last 5 Hours </h2>
				<img src="{{.}}/1" alt="Idle" style="width:100%"> <br>
				<h2 style="text-align:center"> Last 10 Days </h2>
				<img src="{{.}}/0" alt="Idle" style="width:100%"> <br>
			</body>
		</html>	`

	if relPath, err := filepath.Rel(handler.Path(), req.URL.Path); err == nil {
		if relPath != "." {
			var level int

			w.Header().Set("Content-Type", "image/svg+xml")
			fmt.Sscan(relPath, &level)
			if level < len(handler.UserTS.TS) {
				PlotTimeSeries(w, []*TimeSeries{handler.UserTS.TS[level],
					handler.SystemTS.TS[level],
					handler.IdleTS.TS[level]},
					[]string{"user", "system", "idle"})
			}
			return
		}
	}

	if tmpl, err := template.New("command").Parse(tmplStr); err != nil {
		log.Fatal(err)
	} else if err := tmpl.Execute(w, handler.Path()); err != nil {
		log.Fatal(err)
	}
}

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
