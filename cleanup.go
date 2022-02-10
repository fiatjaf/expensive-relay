package main

import (
	"time"
)

// every hour, delete all very old events
func cleanupRoutine() {
	for {
		// TODO: cleanup
		time.Sleep(60 * time.Minute)
	}
}
