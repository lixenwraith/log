# Testing and performance

Use Go 1.27.1 or later. The module has no external dependencies.

```sh
go vet ./...
go test -race -count=1 -timeout=3m ./...
go test -race -run 'TestStress' -count=10 -timeout=3m .
```

`stress_test.go` uses fixed record counts and synchronized starts. It verifies exact queue overflow, unique IDs, processed-plus-dropped accounting across lifecycle changes, and byte preservation across rotations. It has no throughput pass threshold and no timed soak loop. Heartbeat and adaptive-timer tests use `testing/synctest` or explicit state transitions instead of wall-clock sleeps. Run benchmarks separately when comparing performance:

```sh
go test ./... -run '^$' -bench . -benchmem -count=5
```

Fuzz targets test malformed UTF-8, control characters, keys, custom timestamp layouts, numeric edge cases, structured marshal failures, and printf argument association. Each target runs independently:

```sh
go test ./sanitizer -run '^$' -fuzz '^FuzzSanitize$' -fuzztime=30s -parallel=2
go test ./sanitizer -run '^$' -fuzz '^FuzzSerializerJSON$' -fuzztime=30s -parallel=2
go test ./formatter -run '^$' -fuzz '^FuzzFormatJSON$' -fuzztime=30s -parallel=2
go test ./formatter -run '^$' -fuzz '^FuzzFormatTextPolicy$' -fuzztime=30s -parallel=2
go test ./compat -run '^$' -fuzz '^FuzzParseFormat$' -fuzztime=30s -parallel=2
```

Fuzzing is bounded by time in CI; execution counts depend on the runner. All committed seeds run during ordinary `go test`, including minimized failures retained under `testdata/fuzz`.

The GitHub workflow runs on pushes and pull requests. It checks formatting, vet, race detection, repeated lifecycle/stress workloads, bounded fuzzing, and compilation for FreeBSD, Windows, and browser WASM. Runtime tests execute on Linux; cross-compilation does not validate other operating systems' runtime behavior.
