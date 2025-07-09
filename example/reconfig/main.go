// FILE: example/reconfig/main.go
package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/lixenwraith/log"
)

// Simulate rapid reconfiguration
func main() {
	var count atomic.Int64

	logger := log.NewLogger()

	// Initialize the logger with defaults first
	err := logger.InitWithDefaults()
	if err != nil {
		fmt.Printf("Initial Init error: %v\n", err)
		return
	}

	// Log something constantly
	go func() {
		for i := 0; ; i++ {
			logger.Info("Test log", i)
			count.Add(1)
			time.Sleep(time.Millisecond)
		}
	}()

	// Trigger multiple reconfigurations rapidly
	for i := 0; i < 10; i++ {
		// Use different buffer sizes to trigger channel recreation
		bufSize := fmt.Sprintf("buffer_size=%d", 100*(i+1))
		err := logger.InitWithDefaults(bufSize)
		if err != nil {
			fmt.Printf("Init error: %v\n", err)
		}
		// Minimal delay between reconfigurations
		time.Sleep(10 * time.Millisecond)
	}

	// Check if we see any inconsistency
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("Total logger. attempted: %d\n", count.Load())

	// Gracefully shut down the logger.er
	err = logger.Shutdown(time.Second)
	if err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
	}

	// Check for any error messages in the logger.files
	// or dropped logger.count
}