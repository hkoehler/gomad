// Copyright (C) 2016, Heiko Koehler

package main

import (
	"log"
	"time"
)

// schedules timers for executing commands
func StartScheduler() {
	for path, handler := range Registry {
		if handler.PollInterval() > 0 {
			log.Printf("Start ticker for %s\n", path)
			ticker := time.NewTicker(handler.PollInterval())
			// create own copy of handler for go routine
			handler := handler
			go func() {
				for range ticker.C {
					handler.Execute()
				}
			}()
		}
	}
}
