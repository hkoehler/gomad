package main

import (
	"encoding/json"
	"log"
	"os"
)

type Config struct {
	Port     int // TCP port for HTTP service
	Handlers []*HandlerConfig
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
