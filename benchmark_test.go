package log

import (
	"testing"
	"time"
)

// The benchmarks up to BenchmarkProcessLogRecord measure the producer path:
// format selection, channel send, and drop accounting. File writes complete
// asynchronously in the processor and are not attributed to their iterations.

// BenchmarkLoggerInfo measures the default raw-format path.
func BenchmarkLoggerInfo(b *testing.B) {
	logger, _ := newTestLogger(b)

	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		logger.Info("benchmark message", i)
	}
}

// BenchmarkLoggerTxt measures enqueueing with txt output selected.
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

// BenchmarkLoggerStructured measures enqueueing and snapshotting structured maps.
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

// BenchmarkLoggerSanitized measures enqueueing with PolicyTxt selected;
// sanitizer CPU work runs asynchronously and is benchmarked in its own package.
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

// BenchmarkProcessLogRecord measures the processor's write path, which the
// producer benchmarks above do not see. Record stamps advance by step to stand
// for a logging rate: at 100/s every record is due a log path check.
func BenchmarkProcessLogRecord(b *testing.B) {
	for _, bc := range []struct {
		name string
		step time.Duration // 0: time.Now per record, as emit stamps it
	}{{"ts=now", 0}, {"rate=1k/s", time.Millisecond}, {"rate=100/s", 10 * time.Millisecond}} {
		b.Run(bc.name, func(b *testing.B) {
			l := NewLogger()
			c := DefaultConfig()
			c.EnableConsole = false
			c.EnableFile = true
			c.Directory = b.TempDir()
			c.Format = "txt"
			c.MaxSizeKB = 0
			mustNoErr(b, l.ApplyConfig(c), "ApplyConfig") // not started: the benchmark is the processor
			b.Cleanup(func() { _ = l.Shutdown() })
			rec := logRecord{Flags: l.getFlags(), Level: LevelInfo, Args: []any{"benchmark message", 42}}
			ts := time.Now()
			b.ReportAllocs()
			for b.Loop() {
				if ts = ts.Add(bc.step); bc.step == 0 {
					ts = time.Now()
				}
				rec.TimeStamp = ts
				if l.processLogRecord(rec) == 0 {
					b.Fatal("record not written")
				}
			}
		})
	}
}
