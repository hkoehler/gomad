// Copyright (C) 2016, Heiko Koehler

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

type Config struct {
	Port     int // TCP port for HTTP service
	Handlers []*HandlerConfig
}

type HandlerConfig struct {
	Type         string
	Name         string
	Cmd          string
	URL          string
	Regex        string
	Submatches   []string
	PollInterval string
}

func (conf HandlerConfig) String() string {
	return fmt.Sprintf("Handler(Name: \"%s\", Type: \"%s\", Command: \"%s\", URL: \"%s\")",
		conf.Name, conf.Type, conf.Cmd, conf.URL)
}

var (
	ConfigPath string
	Port       int
)

// create HTTP handlers from config
func LoadConfig(f *os.File) {
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
			RegisterHandler(handler)
		} else {
			log.Fatal(err)
		}
	}
}
