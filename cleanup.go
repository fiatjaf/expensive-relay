package main

import (
	"time"
)

// every hour, delete all very old events
func cleanupRoutine() {
	for {
		// TODO: query board app for list of abusive users and clean up their old events?
		time.Sleep(60 * time.Minute)
	}
}
