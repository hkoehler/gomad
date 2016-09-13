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
	"io"
	"log"
	"net/http"
	"os"
)

type ConfigEntry struct {
	Name string
	Cmd  string
	URL  string
}

var (
	ConfigPath string
	Port       int
	Registry   = make(map[string]RegistryEntry)
)

func (entry ConfigEntry) String() string {
	return fmt.Sprintf("(Name: \"%s\", Command: \"%s\", URL: \"%s\")",
		entry.Name, entry.Cmd, entry.URL)
}

// register handler
func registerHandler(entry RegistryEntry) {
	Registry[entry.Path()] = entry
	http.Handle(entry.Path(), entry)
}

// create HTTP handlers from config
func loadConfig(f *os.File) {
	dec := json.NewDecoder(f)
	for {
		var entry ConfigEntry

		if err := dec.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}
		log.Println(entry)
		handler := NewCommandHandler(entry)
		registerHandler(handler)
	}
}

// root handler listing all other handlers
func rootHandler(w http.ResponseWriter, req *http.Request) {
	type Entry struct {
		Path, Name string
	}

	entries := make([]Entry, 0)
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
				{{range .}}
				<a href="{{.Path}}"> {{.Name}} </a> <br>
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
