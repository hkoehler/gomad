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

var (
	Registry    = make(map[string]Handler)
	masterTempl *template.Template
)

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
	Tmpl       *template.Template
}

// compile regular expression and create time series tables
func NewCommandHandler(conf HandlerConfig) (Handler, error) {
	var pollInterval time.Duration
	var propMap = make(map[string]Property)
	var tmpl *template.Template
	var err error

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

	const tmplStr = `
		<!DOCTYPE html>
		<html>
			<head>
			{{template "style"}}
			<title> {{.Cmd}} </title>
			</head>
			<body>
				{{template "header"}}
				<h1 style="text-align:center"> {{.Cmd}} </h1>
				<table style="width:100%;border:1px solid black">
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
				<h3 style="text-align:center"> Last 5 minutes </h3>
				<img src="{{.Path}}/2" alt="{{.Name}}" width="100%" style="border:1px solid black"> <br>
				<h3 style="text-align:center"> Last 5 hours </h3>
				<img src="{{.Path}}/1" alt="{{.Name}}" width="100%" style="border:1px solid black"> <br>
				<h3 style="text-align:center"> Last 10 days </h3>
				<img src="{{.Path}}/0" alt="{{.Name}}" width="100%" style="border:1px solid black"> <br>
				{{end}}
			</body>
		</html>
	`

	if tmpl, err = masterTempl.New("command").Parse(tmplStr); err != nil {
		log.Fatal(err)
	}

	return &CommandHandler{HandlerImpl: HandlerImpl{conf.URL, conf.Name, pollInterval},
			CmdLine: conf.Cmd, Properties: propMap, Charts: conf.Charts, Tmpl: tmpl},
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
	//fmt.Println(props)
	for key, val := range props {
		var floatVal float64

		// map property name to time series
		prop := handler.Properties[key]
		fmt.Sscanf(val, "%f", &floatVal)
		prop.TS.Add(floatVal)
	}
}

func (handler CommandHandler) ServeChart(w http.ResponseWriter, req *http.Request, relPath string) {
	w.Header().Set("Content-Type", "image/svg+xml")
	var ts = make([]*TimeSeries, 0)
	var legend = make([]string, 0)
	var level int

	comps := strings.Split(relPath, "/")
	if len(comps) != 2 {
		fmt.Fprintf(w, "Invalid Path")
		return
	}

	chartName, levelStr := comps[0], comps[1]
	fmt.Sscanf(levelStr, "%d", &level)
	for _, chart := range handler.Charts {
		if chart.Name == chartName {
			for _, prop := range chart.Properties {
				ts = append(ts, handler.Properties[prop].TS.TS[level])
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
	if err := handler.Tmpl.Execute(w, page); err != nil {
		log.Fatal(err)
	}
}

// HTTP handler displaying config
type ConfigHandler struct {
	HandlerImpl
	ConfigPath string
	Tmpl       *template.Template
}

func NewConfigHandler(path string, configPath string) Handler {
	const tmplStr = `
		<!DOCTYPE html>
		<html>
			<head>
			{{template "style"}}
			<title> Config </title>
			</head>
			<body>
				{{template "header"}}
				<h1 style="text-align:center"> Config </h1>
				<iframe src="{{.}}" style="border:1px solid black" height=1000 width=100%>
			</body>
		</html>	`

	if tmpl, err := masterTempl.New("config").Parse(tmplStr); err != nil {
		log.Panic(fmt.Sprintf("Failed to parse HTML template: %v", err))
		return nil
	} else {
		return &ConfigHandler{HandlerImpl: HandlerImpl{path, "Config", 0},
			ConfigPath: configPath, Tmpl: tmpl}
	}
}

func (handler ConfigHandler) Execute() {
}

func (handler ConfigHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var txtPath = filepath.Join(handler.Path(), "text")

	if req.URL.Path == txtPath {
		if f, err := os.Open(handler.ConfigPath); err == nil {
			if _, err := io.Copy(w, f); err != nil {
				fmt.Println("Failed to read %s: %v", handler.ConfigPath, err)
			}
		} else {
			fmt.Fprintf(w, "Couldn't open %s: %v", handler.ConfigPath, err)
		}
	} else {
		if err := handler.Tmpl.Execute(w, txtPath); err != nil {
			log.Fatal(err)
		}
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
	Tmpl     *template.Template
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
	const tmplStr = `
		<!DOCTYPE html>
		<html>
			<head>
			{{template "style"}}
			<title> CPU Load </title>
			</head>
			<body>
				{{template "header"}}
				<h1 style="text-align:center"> CPU Load </h1>
				<h2 style="text-align:center"> Last 5 Minutes </h2>
				<img src="{{.}}/2" width="100%" style="border:1px solid black"> <br>
				<h2 style="text-align:center"> Last 5 Hours </h2>
				<img src="{{.}}/1" width="100%" style="border:1px solid black"> <br>
				<h2 style="text-align:center"> Last 10 Days </h2>
				<img src="{{.}}/0" width="100%" style="border:1px solid black"> <br>
			</body>
		</html>	`
	if tmpl, err := masterTempl.New("cpu").Parse(tmplStr); err != nil {
		log.Fatal(err)
		return nil, err
	} else {
		return &CPULoadHandler{HandlerImpl: HandlerImpl{"/cpu", "CPU Load", pollInterval},
			UserTS: userTS, SystemTS: systemTS, IdleTS: idleTS,
			Load: NewSystemLoad(), Tmpl: tmpl}, nil
	}
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

	if err := handler.Tmpl.Execute(w, handler.Path()); err != nil {
		log.Fatal(err)
	}
}

type RootHandler struct {
	HandlerImpl
	Tmpl *template.Template
}

func NewRootHandler() Handler {
	const tmplStr = `
		<html>
			<head>
			{{template "style"}}
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
	if tmpl, err := masterTempl.New("index").Parse(tmplStr); err != nil {
		log.Fatal(err)
		return nil
	} else {
		return &RootHandler{HandlerImpl: HandlerImpl{"/", "Root", 0}, Tmpl: tmpl}
	}
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
	if err := handler.Tmpl.Execute(w, entries); err != nil {
		log.Fatal(err)
	}
}

func init() {
	const headerStr = `
		<div style="background-color:powderblue; font-size:20px;border:20px solid powderblue">
		<a href="/" style="padding: 21px 50px 21px 20px;"> Monitoring and Alerting Daemon </a>
		<a href="http://opensource.org/licenses/MIT" style="float: right; padding-right: 20px; padding-left: 50px"> License </a>
		<a href="http://www.github.com/hkoehler/gomad" style="float: right; padding-left: 50px"> Project Site </a>
		</div>
		<br>
	`
	
	const styleStr = `
		<style>
		body 	{background-color: white;}
		</style>
	`

	if templ, err := template.New("header").Parse(headerStr); err != nil {
		log.Fatal(err)
	} else {
		masterTempl = templ
	}
	masterTempl.New("style").Parse(styleStr)
}
