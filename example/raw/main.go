// FILE: example/raw/main.go
package main

import (
	"fmt"
	"time"

	"github.com/lixenwraith/log"
)

// TestPayload defines a struct for testing complex type serialization.
type TestPayload struct {
	RequestID uint64
	User      string
	Metrics   map[string]float64
}

func main() {
	fmt.Println("--- Logger Raw Format Test ---")

	// --- 1. Define the records to be tested ---
	// Record 1: A byte slice with special characters (newline, tab, null).
	byteRecord := []byte("binary\ndata\twith\x00null")

	// Record 2: A struct containing a uint64, a string, and a map.
	structRecord := TestPayload{
		RequestID: 9223372036854775807, // A large uint64
		User:      "test_user",
		Metrics: map[string]float64{
			"latency_ms": 15.7,
			"cpu_percent": 88.2,
		},
	}

	// --- 2. Test on-demand raw logging using Logger.Write() ---
	// This method produces raw output regardless of the global format setting.
	fmt.Println("\n[1] Testing on-demand raw output via Logger.Write()")
	logger1 := log.NewLogger()
	// Use default config, but enable stdout and disable file output for this test.
	err := logger1.InitWithDefaults("enable_stdout=true", "disable_file=false")
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		return
	}

	logger1.Write("Byte Record ->", byteRecord)
	logger1.Write("Struct Record ->", structRecord)
	// Wait briefly for the async processor to handle the logs.
	time.Sleep(100 * time.Millisecond)
	logger1.Shutdown()

	// --- 3. Test instance-wide raw logging using format="raw" ---
	// Here, standard methods like Info() will produce raw output.
	fmt.Println("\n[2] Testing instance-wide raw output via format=\"raw\"")
	logger2 := log.NewLogger()
	err = logger2.InitWithDefaults(
		"enable_stdout=true",
		"disable_file=false",
		"format=raw",
	)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		return
	}

	logger2.Info("Byte Record ->", byteRecord)
	logger2.Info("Struct Record ->", structRecord)
	time.Sleep(100 * time.Millisecond)
	logger2.Shutdown()

	fmt.Println("\n--- Test Complete ---")
}
