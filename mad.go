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
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	flag.StringVar(&ConfigPath, "config", "/etc/mad.json", "Path to confg file")
	flag.IntVar(&Port, "port", 8080, "Server port")
	flag.Parse()
	log.Printf("Config path: %s", ConfigPath)

	if f, err := os.Open(ConfigPath); err != nil {
		log.Fatal(err)
	} else {
		LoadConfig(f)
		configHandler := NewConfigHandler("/config", ConfigPath)
		RegisterHandler(configHandler)
		rootHandler := NewRootHandler()
		RegisterHandler(rootHandler)
		cpuHandler, _ := NewCPULoadHandler()
		RegisterHandler(cpuHandler)
	}
	StartScheduler()
	if err := http.ListenAndServe(fmt.Sprintf(":%d", Port), nil); err != nil {
		log.Fatal(err)
	}
}
