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
	fmt.Fprintf(w, "<html>\n")
	fmt.Fprintf(w, "<head>\n")
	fmt.Fprintf(w, "<title> Registered Commands </title>\n")
	fmt.Fprintf(w, "</head>\n")
	fmt.Fprintf(w, "<body>\n")
	fmt.Fprintf(w, "<h1> Registered Commands </h1>\n")
	fmt.Fprintf(w, "<div> </div>\n")
	for path, entry := range Registry {
		fmt.Fprintf(w, "<a href=\"%s\"> %s </a> <br>\n", path, entry.Name())
	}
	fmt.Fprintf(w, "</body>")
	fmt.Fprintf(w, "</html>")
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
