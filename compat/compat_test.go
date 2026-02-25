package compat

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lixenwraith/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestCompatBuilder creates a standard setup for compatibility adapter tests
func createTestCompatBuilder(t *testing.T) (*Builder, *log.Logger, string) {
	t.Helper()
	tmpDir := t.TempDir()
	appLogger, err := log.NewBuilder().
		Directory(tmpDir).
		Format("json").
		LevelString("debug").
		EnableFile(true).
		Build()
	require.NoError(t, err)

	// Start the logger before using it
	err = appLogger.Start()
	require.NoError(t, err)

	builder := NewBuilder().WithLogger(appLogger)
	return builder, appLogger, tmpDir
}

// readLogFile reads a log file, retrying briefly to await async writes
func readLogFile(t *testing.T, dir string, expectedLines int) []string {
	t.Helper()
	var err error

	// Retry for a short period to handle logging delays
	for i := 0; i < 20; i++ {
		var files []os.DirEntry
		files, err = os.ReadDir(dir)
		if err == nil && len(files) > 0 {
			var logFile *os.File
			logFilePath := filepath.Join(dir, files[0].Name())
			logFile, err = os.Open(logFilePath)
			if err == nil {
				scanner := bufio.NewScanner(logFile)
				var readLines []string
				for scanner.Scan() {
					readLines = append(readLines, scanner.Text())
				}
				logFile.Close()
				if len(readLines) >= expectedLines {
					return readLines
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Failed to read %d log lines from directory %s. Last error: %v", expectedLines, dir, err)
	return nil
}

// TestCompatBuilder verifies the compatibility builder can be initialized correctly
func TestCompatBuilder(t *testing.T) {
	t.Run("with existing logger", func(t *testing.T) {
		builder, logger, _ := createTestCompatBuilder(t)
		defer logger.Shutdown()

		gnetAdapter, err := builder.BuildGnet()
		require.NoError(t, err)
		assert.NotNil(t, gnetAdapter)
		assert.Equal(t, logger, gnetAdapter.logger)
	})

	t.Run("with config", func(t *testing.T) {
		logCfg := log.DefaultConfig()
		logCfg.Directory = t.TempDir()

		builder := NewBuilder().WithConfig(logCfg)
		fasthttpAdapter, err := builder.BuildFastHTTP()
		require.NoError(t, err)
		assert.NotNil(t, fasthttpAdapter)

		logger1, _ := builder.GetLogger()
		// The builder now creates AND starts the logger internally if needed
		// We need to defer shutdown to clean up resources
		defer logger1.Shutdown()
	})
}

// TestGnetAdapter tests the gnet adapter's logging output and format
func TestGnetAdapter(t *testing.T) {
	builder, logger, tmpDir := createTestCompatBuilder(t)
	defer logger.Shutdown()

	var fatalCalled bool
	adapter, err := builder.BuildGnet(WithFatalHandler(func(msg string) {
		fatalCalled = true
	}))
	require.NoError(t, err)

	adapter.Debugf("gnet debug id=%d", 1)
	adapter.Infof("gnet info id=%d", 2)
	adapter.Warnf("gnet warn id=%d", 3)
	adapter.Errorf("gnet error id=%d", 4)
	adapter.Fatalf("gnet fatal id=%d", 5)

	err = logger.Flush(time.Second)
	require.NoError(t, err)

	lines := readLogFile(t, tmpDir, 5)

	// Define expected log data. The order in the "fields" array is fixed by the adapter call
	expected := []struct{ level, msg string }{
		{"DEBUG", "gnet debug id=1"},
		{"INFO", "gnet info id=2"},
		{"WARN", "gnet warn id=3"},
		{"ERROR", "gnet error id=4"},
		{"ERROR", "gnet fatal id=5"},
	}

	// Filter out the "Logger started" line
	var logLines []string
	for _, line := range lines {
		logLines = append(logLines, line)
	}
	require.Len(t, logLines, 5, "Should have 5 gnet log lines after filtering")

	for i, line := range logLines {
		var entry map[string]any
		err := json.Unmarshal([]byte(line), &entry)
		require.NoError(t, err, "Failed to parse log line: %s", line)

		assert.Equal(t, expected[i].level, entry["level"])

		// The logger puts all arguments into a "fields" array
		// The adapter's calls look like: logger.Info("msg", msg, "source", "gnet")
		fields := entry["fields"].([]any)
		assert.Equal(t, "msg", fields[0])
		assert.Equal(t, expected[i].msg, fields[1])
		assert.Equal(t, "source", fields[2])
		assert.Equal(t, "gnet", fields[3])
	}
	assert.True(t, fatalCalled, "Custom fatal handler should have been called")
}

// TestStructuredGnetAdapter tests the gnet adapter with structured field extraction
func TestStructuredGnetAdapter(t *testing.T) {
	builder, logger, tmpDir := createTestCompatBuilder(t)
	defer logger.Shutdown()

	adapter, err := builder.BuildStructuredGnet()
	require.NoError(t, err)

	adapter.Infof("request served status=%d client_ip=%s", 200, "127.0.0.1")

	err = logger.Flush(time.Second)
	require.NoError(t, err)

	lines := readLogFile(t, tmpDir, 1)

	// Find our specific log line
	require.Len(t, lines, 1, "Should be exactly one log line")
	logLine := lines[0]
	require.NotEmpty(t, logLine, "Did not find the structured gnet log line")

	var entry map[string]any
	err = json.Unmarshal([]byte(logLine), &entry)
	require.NoError(t, err)

	// The structured adapter parses keys and values, so we check them directly
	fields := entry["fields"].([]any)
	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "msg", fields[0])
	assert.Equal(t, "request served", fields[1])
	assert.Equal(t, "status", fields[2])
	assert.Equal(t, 200.0, fields[3]) // JSON numbers are float64
	assert.Equal(t, "client_ip", fields[4])
	assert.Equal(t, "127.0.0.1", fields[5])
	assert.Equal(t, "source", fields[6])
	assert.Equal(t, "gnet", fields[7])
}

// TestFastHTTPAdapter tests the fasthttp adapter's logging output and level detection
func TestFastHTTPAdapter(t *testing.T) {
	builder, logger, tmpDir := createTestCompatBuilder(t)
	defer logger.Shutdown()

	adapter, err := builder.BuildFastHTTP()
	require.NoError(t, err)

	testMessages := []string{
		"this is some informational message",
		"a debug message for the developers",
		"warning: something might be wrong",
		"an error occurred while processing",
	}
	for _, msg := range testMessages {
		adapter.Printf("%s", msg)
	}

	err = logger.Flush(time.Second)
	require.NoError(t, err)

	// Expect 4 test messages
	lines := readLogFile(t, tmpDir, 4)
	expectedLevels := []string{"INFO", "DEBUG", "WARN", "ERROR"}

	require.Len(t, lines, 4, "Should have 4 fasthttp log lines")

	for i, line := range lines {
		var entry map[string]any
		err := json.Unmarshal([]byte(line), &entry)
		require.NoError(t, err, "Failed to parse log line: %s", line)

		assert.Equal(t, expectedLevels[i], entry["level"])
		fields := entry["fields"].([]any)
		assert.Equal(t, "msg", fields[0])
		assert.Equal(t, testMessages[i], fields[1])
		assert.Equal(t, "source", fields[2])
		assert.Equal(t, "fasthttp", fields[3])
	}
}

// TestFiberAdapter tests the Fiber adapter's logging output across all log levels
func TestFiberAdapter(t *testing.T) {
	builder, logger, tmpDir := createTestCompatBuilder(t)
	defer logger.Shutdown()

	var fatalCalled bool
	var panicCalled bool
	adapter, err := builder.BuildFiber(
		WithFiberFatalHandler(func(msg string) {
			fatalCalled = true
		}),
		WithFiberPanicHandler(func(msg string) {
			panicCalled = true
		}),
	)
	require.NoError(t, err)

	// Test formatted logging (Tracef, Debugf, Infof, Warnf, Errorf, Fatalf, Panicf)
	adapter.Tracef("fiber trace id=%d", 1)
	adapter.Debugf("fiber debug id=%d", 2)
	adapter.Infof("fiber info id=%d", 3)
	adapter.Warnf("fiber warn id=%d", 4)
	adapter.Errorf("fiber error id=%d", 5)
	adapter.Fatalf("fiber fatal id=%d", 6)
	adapter.Panicf("fiber panic id=%d", 7)

	err = logger.Flush(time.Second)
	require.NoError(t, err)

	lines := readLogFile(t, tmpDir, 7)

	expected := []struct {
		level string
		msg   string
	}{
		{"DEBUG", "fiber trace id=1"},
		{"DEBUG", "fiber debug id=2"},
		{"INFO", "fiber info id=3"},
		{"WARN", "fiber warn id=4"},
		{"ERROR", "fiber error id=5"},
		{"ERROR", "fiber fatal id=6"},
		{"ERROR", "fiber panic id=7"},
	}

	require.Len(t, lines, 7, "Should have 7 fiber log lines")

	for i, line := range lines {
		var entry map[string]any
		err := json.Unmarshal([]byte(line), &entry)
		require.NoError(t, err, "Failed to parse log line: %s", line)

		assert.Equal(t, expected[i].level, entry["level"])
		fields := entry["fields"].([]any)
		assert.Equal(t, "msg", fields[0])
		assert.Equal(t, expected[i].msg, fields[1])
		assert.Equal(t, "source", fields[2])
		assert.Equal(t, "fiber", fields[3])
	}
	assert.True(t, fatalCalled, "Custom fatal handler should have been called")
	assert.True(t, panicCalled, "Custom panic handler should have been called")
}

// TestFiberAdapterStructuredLogging tests Fiber's structured logging (WithLogger methods)
func TestFiberAdapterStructuredLogging(t *testing.T) {
	builder, logger, tmpDir := createTestCompatBuilder(t)
	defer logger.Shutdown()

	adapter, err := builder.BuildFiber()
	require.NoError(t, err)

	// Test structured logging with key-value pairs
	adapter.Infow("request served", "status", 200, "client_ip", "127.0.0.1", "method", "GET")
	adapter.Debugw("query executed", "duration_ms", 42, "query", "SELECT * FROM users")

	err = logger.Flush(time.Second)
	require.NoError(t, err)

	lines := readLogFile(t, tmpDir, 2)
	require.Len(t, lines, 2, "Should have 2 fiber structured log lines")

	// Check first structured log (Infow)
	var entry1 map[string]any
	err = json.Unmarshal([]byte(lines[0]), &entry1)
	require.NoError(t, err)

	assert.Equal(t, "INFO", entry1["level"])
	fields1 := entry1["fields"].([]any)
	assert.Equal(t, "msg", fields1[0])
	assert.Equal(t, "request served", fields1[1])
	assert.Equal(t, "source", fields1[2])
	assert.Equal(t, "fiber", fields1[3])
	assert.Equal(t, "status", fields1[4])
	assert.Equal(t, 200.0, fields1[5]) // JSON numbers are float64
	assert.Equal(t, "client_ip", fields1[6])
	assert.Equal(t, "127.0.0.1", fields1[7])

	// Check second structured log (Debugw)
	var entry2 map[string]any
	err = json.Unmarshal([]byte(lines[1]), &entry2)
	require.NoError(t, err)

	assert.Equal(t, "DEBUG", entry2["level"])
	fields2 := entry2["fields"].([]any)
	assert.Equal(t, "msg", fields2[0])
	assert.Equal(t, "query executed", fields2[1])
	assert.Equal(t, "source", fields2[2])
	assert.Equal(t, "fiber", fields2[3])
	assert.Equal(t, "duration_ms", fields2[4])
	assert.Equal(t, 42.0, fields2[5]) // JSON numbers are float64
}

// TestFiberBuilderIntegration ensures Fiber adapter can be built from builder
func TestFiberBuilderIntegration(t *testing.T) {
	builder, logger, _ := createTestCompatBuilder(t)
	defer logger.Shutdown()

	fiberAdapter, err := builder.BuildFiber()
	require.NoError(t, err)
	assert.NotNil(t, fiberAdapter)
	assert.Equal(t, logger, fiberAdapter.logger)
}