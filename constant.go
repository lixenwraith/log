// FILE: lixenwraith/log/constant.go
package log

import (
	"time"
)

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
	FlagRaw            int64 = 0b0001
	FlagShowTimestamp  int64 = 0b0010
	FlagShowLevel      int64 = 0b0100
	FlagStructuredJSON int64 = 0b1000
	FlagDefault              = FlagShowTimestamp | FlagShowLevel
)

// Storage
const (
	// Threshold for triggering reactive disk check
	reactiveCheckThresholdBytes int64 = 10 * 1024 * 1024
	// Size multiplier for KB, MB
	sizeMultiplier = 1000
)

// Timers
const (
	// Minimum wait time used throughout the package
	minWaitTime = 10 * time.Millisecond
	// Factors to adjust check interval
	adaptiveIntervalFactor float64 = 1.5 // Slow down
	adaptiveSpeedUpFactor  float64 = 0.8 // Speed up
)