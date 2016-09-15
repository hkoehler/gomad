/*
 * Monitoring and Alerting Daemon
 *
 * Copyright (C) 2016, Heiko Koehler
 *
 * HTTP service for monitoring configured commands. MAD also lets user configure alerts on top of
 * commands.
 */

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
)

type HandlerConfig struct {
	Type string
	Name string
	Cmd  string
	URL  string
	Regex string
	Submatches []string
}

type Config struct {
	Port     int // TCP port for HTTP service
	Handlers []*HandlerConfig
}

var (
	ConfigPath string
	Port       int
	Registry   = make(map[string]Handler)
)

func (conf HandlerConfig) String() string {
	return fmt.Sprintf("Handler(Name: \"%s\", Type: \"%s\", Command: \"%s\", URL: \"%s\")",
		conf.Name, conf.Type, conf.Cmd, conf.URL)
}

// register handler
func registerHandler(entry Handler) {
	Registry[entry.Path()] = entry
	http.Handle(entry.Path(), entry)
}

// create HTTP handlers from config
func loadConfig(f *os.File) {
	var config Config

	dec := json.NewDecoder(f)
	if err := dec.Decode(&config); err != nil {
		log.Fatal(err)
	}

	if config.Port != 0 {
		Port = config.Port
		log.Printf("Port: %d\n", Port)
	}
	for _, handlerConf := range config.Handlers {
		log.Println(handlerConf)
		if handler, err := NewHandler(*handlerConf); err == nil {
			registerHandler(handler)
		} else {
			log.Fatal(err)
		}
	}
}

// root handler listing all other handlers
func rootHandler(w http.ResponseWriter, req *http.Request) {
	type Entry struct {
		Path, Name string
	}

	entries := make([]Entry, 0, len(Registry))
	for path, entry := range Registry {
		entries = append(entries, Entry{path, entry.Name()})
	}

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

func main() {
	flag.StringVar(&ConfigPath, "config", "/etc/mad.json", "Path to confg file")
	flag.IntVar(&Port, "port", 8080, "Server port")
	flag.Parse()
	log.Printf("Config path: %s", ConfigPath)

	if f, err := os.Open(ConfigPath); err != nil {
		log.Fatal(err)
	} else {
		loadConfig(f)
		configHandler := NewConfigHandler("/config", ConfigPath)
		registerHandler(configHandler)
	}
	http.HandleFunc("/", rootHandler)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", Port), nil); err != nil {
		log.Fatal(err)
	}
}
