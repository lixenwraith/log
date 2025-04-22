package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/LixenWraith/log"
)

// Simulate rapid reconfiguration
func main() {
	var count atomic.Int64

	// Initialize the logger with defaults first
	err := log.InitWithDefaults()
	if err != nil {
		fmt.Printf("Initial Init error: %v\n", err)
		return
	}

	// Log something constantly
	go func() {
		for i := 0; ; i++ {
			log.Info("Test log", i)
			count.Add(1)
			time.Sleep(time.Millisecond)
		}
	}()

	// Trigger multiple reconfigurations rapidly
	for i := 0; i < 10; i++ {
		// Use different buffer sizes to trigger channel recreation
		bufSize := fmt.Sprintf("buffer_size=%d", 100*(i+1))
		err := log.InitWithDefaults(bufSize)
		if err != nil {
			fmt.Printf("Init error: %v\n", err)
		}
		// Minimal delay between reconfigurations
		time.Sleep(10 * time.Millisecond)
	}

	// Check if we see any inconsistency
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("Total logs attempted: %d\n", count.Load())

	// Gracefully shut down the logger
	err = log.Shutdown(time.Second)
	if err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
	}

	// Check for any error messages in the log files
	// or dropped log count
}