// FILE: lixenwraith/log/constant.go
package log

import "time"

// Log level constants
const (
	LevelDebug int64 = -4
	LevelInfo  int64 = 0
	LevelWarn  int64 = 4
	LevelError int64 = 8
)

// Heartbeat log levels
const (
	LevelProc int64 = 12
	LevelDisk int64 = 16
	LevelSys  int64 = 20
)

// Record flags for controlling output structure
const (
	FlagShowTimestamp  int64 = 0b0001
	FlagShowLevel      int64 = 0b0010
	FlagRaw            int64 = 0b0100
	FlagStructuredJSON int64 = 0b1000
	FlagDefault              = FlagShowTimestamp | FlagShowLevel
)

const (
	// Threshold for triggering reactive disk check
	reactiveCheckThresholdBytes int64 = 10 * 1024 * 1024
	// Factors to adjust check interval
	adaptiveIntervalFactor float64 = 1.5 // Slow down
	adaptiveSpeedUpFactor  float64 = 0.8 // Speed up
	// Minimum wait time used throughout the package
	minWaitTime = 10 * time.Millisecond
)

const hexChars = "0123456789abcdef"

const sizeMultiplier = 1000