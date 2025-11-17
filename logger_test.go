// FILE: lixenwraith/log/logger_test.go
package log

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestLogger creates logger in temp directory
func createTestLogger(t *testing.T) (*Logger, string) {
	tmpDir := t.TempDir()
	logger := NewLogger()

	cfg := DefaultConfig()
	cfg.EnableConsole = false
	cfg.EnableFile = true
	cfg.Directory = tmpDir
	cfg.BufferSize = 1000
	cfg.FlushIntervalMs = 10

	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)

	// Start the logger
	err = logger.Start()
	require.NoError(t, err)

	return logger, tmpDir
}

// TestNewLogger verifies that a new logger is created with the correct initial state
func TestNewLogger(t *testing.T) {
	logger := NewLogger()

	assert.NotNil(t, logger)
	assert.False(t, logger.state.IsInitialized.Load())
	assert.False(t, logger.state.LoggerDisabled.Load())
}

// TestApplyConfig verifies that applying a valid configuration initializes the logger correctly
func TestApplyConfig(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	// Verify initialization
	assert.True(t, logger.state.IsInitialized.Load())

	// Verify log file creation
	// The file now contains "Logger started"
	logPath := filepath.Join(tmpDir, "log.log")
	_, err := os.Stat(logPath)
	assert.NoError(t, err)
}

// TestApplyConfigString tests applying configuration overrides from key-value strings
func TestApplyConfigString(t *testing.T) {
	logger, _ := createTestLogger(t)
	defer logger.Shutdown()

	tests := []struct {
		name         string
		configString []string
		verify       func(t *testing.T, cfg *Config)
		wantError    bool
	}{
		{
			name: "basic config string",
			configString: []string{
				"level=-4",
				"directory=/tmp/log",
				"format=json",
			},
			verify: func(t *testing.T, cfg *Config) {
				assert.Equal(t, LevelDebug, cfg.Level)
				assert.Equal(t, "/tmp/log", cfg.Directory)
				assert.Equal(t, "json", cfg.Format)
			},
		},
		{
			name:         "level by name",
			configString: []string{"level=debug"},
			verify: func(t *testing.T, cfg *Config) {
				assert.Equal(t, LevelDebug, cfg.Level)
			},
		},
		{
			name: "boolean values",
			configString: []string{
				"enable_console=true",
				"enable_file=true",
				"show_timestamp=false",
			},
			verify: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.EnableConsole)
				assert.True(t, cfg.EnableFile)
				assert.False(t, cfg.ShowTimestamp)
			},
		},
		{
			name:         "invalid format",
			configString: []string{"invalid"},
			wantError:    true,
		},
		{
			name:         "unknown key",
			configString: []string{"unknown_key=value"},
			wantError:    true,
		},
		{
			name:         "invalid value type",
			configString: []string{"buffer_size=not_a_number"},
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := logger.ApplyConfigString(tt.configString...)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				cfg := logger.GetConfig()
				tt.verify(t, cfg)
			}
		})
	}
}

// TestLoggerLoggingLevels checks that messages are correctly filtered based on the configured log level
func TestLoggerLoggingLevels(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	// Log at different levels
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	// Flush and verify
	err := logger.Flush(time.Second)
	require.NoError(t, err)

	// Read log file
	var content []byte
	var fileContent string
	// Poll for a short period to wait for all async writes to complete.
	// This makes the test robust against scheduling variations.
	success := false
	for i := 0; i < 20; i++ {
		content, err = os.ReadFile(filepath.Join(tmpDir, "log.log"))
		require.NoError(t, err)
		fileContent = string(content)
		if strings.Contains(fileContent, "info message") &&
			strings.Contains(fileContent, "warn message") &&
			strings.Contains(fileContent, "error message") {
			success = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, success, "timed out waiting for all log messages to be written")

	// Default level is INFO, so debug shouldn't appear
	assert.NotContains(t, string(content), "debug message")
	assert.Contains(t, string(content), "info message")
	assert.Contains(t, string(content), "warn message")
	assert.Contains(t, string(content), "error message")
}

// TestLoggerWithTrace ensures that logging with a stack trace does not cause a panic
func TestLoggerWithTrace(t *testing.T) {
	logger, _ := createTestLogger(t)
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.Level = LevelDebug
	logger.ApplyConfig(cfg)

	logger.DebugTrace(2, "trace test")
	logger.Flush(time.Second)

	// Just verify it doesn't panic - trace content varies by runtime
}

// TestLoggerFormats verifies that the logger produces the correct output for different formats
func TestLoggerFormats(t *testing.T) {
	tests := []struct {
		name   string
		format string
		check  func(t *testing.T, content string)
	}{
		{
			name:   "txt format",
			format: "txt",
			check: func(t *testing.T, content string) {
				assert.Contains(t, content, `INFO "test message"`)
			},
		},
		{
			name:   "json format",
			format: "json",
			check: func(t *testing.T, content string) {
				assert.Contains(t, content, `"level":"INFO"`)
				assert.Contains(t, content, `"fields":["test message"]`)
			},
		},
		{
			name:   "raw format",
			format: "raw",
			check: func(t *testing.T, content string) {
				assert.Contains(t, content, "test message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			logger := NewLogger()

			cfg := DefaultConfig()
			cfg.Directory = tmpDir
			cfg.Format = tt.format
			cfg.EnableFile = true
			cfg.ShowTimestamp = false // As in the original test
			cfg.ShowLevel = true      // As in the original test
			// Set a fast flush interval for test reliability
			cfg.FlushIntervalMs = 10

			err := logger.ApplyConfig(cfg)
			require.NoError(t, err)

			// Start the logger after configuring it
			err = logger.Start()
			require.NoError(t, err)

			defer logger.Shutdown()

			logger.Info("test message")

			err = logger.Flush(time.Second)
			require.NoError(t, err)

			// Small delay for flush
			time.Sleep(50 * time.Millisecond)

			content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
			require.NoError(t, err)

			tt.check(t, string(content))
		})
	}
}

// TestLoggerConcurrency ensures the logger is safe for concurrent use from multiple goroutines
func TestLoggerConcurrency(t *testing.T) {
	logger, _ := createTestLogger(t)
	defer logger.Shutdown()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				logger.Info("goroutine", i, "log", j)
			}
		}(i)
	}

	wg.Wait()
	err := logger.Flush(time.Second)
	assert.NoError(t, err)
}

// TestLoggerStdoutMirroring confirms that console output can be enabled without causing panics
func TestLoggerStdoutMirroring(t *testing.T) {
	logger := NewLogger()

	cfg := DefaultConfig()
	cfg.Directory = t.TempDir()
	cfg.EnableConsole = true
	cfg.EnableFile = false

	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)
	err = logger.Start()
	require.NoError(t, err)
	defer logger.Shutdown()

	// Just verify it doesn't panic - actual stdout capture is complex
	logger.Info("stdout test")
}

// TestLoggerWrite verifies that the Write method outputs raw, unformatted data
func TestLoggerWrite(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	logger.Write("raw", "output", 123)

	logger.Flush(time.Second)

	// Small delay for flush
	time.Sleep(50 * time.Millisecond)

	content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
	require.NoError(t, err)

	assert.Contains(t, string(content), "raw output 123")
	assert.True(t, strings.HasSuffix(string(content), "raw output 123"))
}