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
	"os/exec"
	"strings"
)

type ConfigEntry struct {
	Name string
	Cmd  string
	URL  string
}

func (entry ConfigEntry) String() string {
	return fmt.Sprintf("(Name: \"%s\", Command: \"%s\", URL: \"%s\")",
		entry.Name, entry.Cmd, entry.URL)
}

var (
	ConfigPath string
	Port       int
	Config     []ConfigEntry
)

func parseConfig(f *os.File) {
	dec := json.NewDecoder(f)
	for {
		var entry ConfigEntry

		if err := dec.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}
		log.Println(entry)
		Config = append(Config, entry)
	}
}

func httpHandler(w http.ResponseWriter, r *http.Request) {
	for _, entry := range Config {
		if entry.URL == r.URL.Path {
			// execute command
			cmd := strings.Split(entry.Cmd, " ")
			if out, err := exec.Command(cmd[0], cmd[1:]...).Output(); err == nil {
				fmt.Fprintln(w, string(out));
			} else {
				fmt.Fprintf(w, "Error executing command %s: %v\n", entry.Cmd, err)
			}
			return
		}
	}
	fmt.Fprintf(w, "Unknown command")
}

func main() {
	flag.StringVar(&ConfigPath, "config", "/etc/mad.json", "Path to confg file")
	flag.IntVar(&Port, "port", 8080, "Server port")
	flag.Parse()
	log.Printf("Config path: %s", ConfigPath)

	if f, err := os.Open(ConfigPath); err != nil {
		log.Fatal(err)
	} else {
		parseConfig(f)
	}
	http.HandleFunc("/", httpHandler)
	http.ListenAndServe(fmt.Sprintf(":%d", Port), nil)
}
