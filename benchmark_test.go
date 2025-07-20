// FILE: lixenwraith/log/benchmark_test.go
package log

import (
	"testing"
)

func BenchmarkLoggerInfo(b *testing.B) {
	logger, _ := createTestLogger(&testing.T{})
	defer logger.Shutdown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", i)
	}
}

func BenchmarkLoggerJSON(b *testing.B) {
	logger, _ := createTestLogger(&testing.T{})
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.Format = "json"
	logger.ApplyConfig(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", i, "key", "value")
	}
}

func BenchmarkLoggerStructured(b *testing.B) {
	logger, _ := createTestLogger(&testing.T{})
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.Format = "json"
	logger.ApplyConfig(cfg)

	fields := map[string]any{
		"user_id": 123,
		"action":  "benchmark",
		"value":   42.5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.LogStructured(LevelInfo, "benchmark", fields)
	}
}

func BenchmarkConcurrentLogging(b *testing.B) {
	logger, _ := createTestLogger(&testing.T{})
	defer logger.Shutdown()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			logger.Info("concurrent", i)
			i++
		}
	})
}