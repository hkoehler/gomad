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
	_ "net/http"
	"os"
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

func main() {
	flag.StringVar(&ConfigPath, "config", "/etc/mad.json", "Path to confg file")
	flag.Parse()
	log.Printf("Config path: %s", ConfigPath)

	if f, err := os.Open(ConfigPath); err != nil {
		log.Fatal(err)
	} else {
		parseConfig(f)
	}
}
