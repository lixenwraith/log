package log

import (
	"testing"
)

// These benchmarks measure the producer path: format selection, channel send,
// and drop accounting. File writes complete asynchronously in the processor and
// are not attributed to the measured iterations.

// BenchmarkLoggerInfo measures the default raw-format path.
func BenchmarkLoggerInfo(b *testing.B) {
	logger, _ := newTestLogger(b)

	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		logger.Info("benchmark message", i)
	}
}

// BenchmarkLoggerTxt measures the txt path, which includes quote analysis.
func BenchmarkLoggerTxt(b *testing.B) {
	logger, _ := newTestLogger(b)

	cfg := logger.GetConfig()
	cfg.Format = "txt"
	mustNoErr(b, logger.ApplyConfig(cfg), "ApplyConfig")

	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		logger.Info("benchmark message", i)
	}
}

// BenchmarkLoggerJSON measures the json path with key/value arguments.
func BenchmarkLoggerJSON(b *testing.B) {
	logger, _ := newTestLogger(b)

	cfg := logger.GetConfig()
	cfg.Format = "json"
	mustNoErr(b, logger.ApplyConfig(cfg), "ApplyConfig")

	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		logger.Info("benchmark message", i, "key", "value")
	}
}

// BenchmarkLoggerStructured measures the json.Marshal path for field maps.
func BenchmarkLoggerStructured(b *testing.B) {
	logger, _ := newTestLogger(b)

	cfg := logger.GetConfig()
	cfg.Format = "json"
	mustNoErr(b, logger.ApplyConfig(cfg), "ApplyConfig")

	fields := map[string]any{
		"user_id": 123,
		"action":  "benchmark",
		"value":   42.5,
	}

	b.ReportAllocs()
	for b.Loop() {
		logger.LogStructured(LevelInfo, "benchmark", fields)
	}
}

// BenchmarkLoggerSanitized measures PolicyTxt overhead on control-free input,
// where the sanitizer takes its no-allocation fast path.
func BenchmarkLoggerSanitized(b *testing.B) {
	logger, _ := newTestLogger(b)

	cfg := logger.GetConfig()
	cfg.Format = "txt"
	cfg.Sanitization = PolicyTxt
	mustNoErr(b, logger.ApplyConfig(cfg), "ApplyConfig")

	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		logger.Info("benchmark message", i)
	}
}

// BenchmarkConcurrentLogging measures contention on the shared channel.
func BenchmarkConcurrentLogging(b *testing.B) {
	logger, _ := newTestLogger(b)

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			logger.Info("concurrent", i)
			i++
		}
	})
}
