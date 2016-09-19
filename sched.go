package main

import (
	"time"
	"log"
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
				for t := range ticker.C {
					_, attrs := handler.Stat()
					log.Printf("%s, %s\n", t, attrs)
				}
			}()
		}
	}
}
