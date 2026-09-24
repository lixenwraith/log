package log

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Fixed workloads assert accounting and uniqueness; they do not depend on the
// scheduler producing a particular drop rate or on a wall-clock soak duration.
func TestStressQueueAccounting(t *testing.T) {
	for _, capacity := range []int64{8, 16384} {
		t.Run(strconv.FormatInt(capacity, 10), func(t *testing.T) {
			const workers, perWorker = 32, 512
			var output bytes.Buffer
			l := NewLogger()
			c := DefaultConfig()
			c.BufferSize = capacity
			c.EnableConsole = true
			c.Format = "raw"
			mustNoErr(t, l.ApplyConfig(c), "apply")
			l.state.StdoutWriter.Store(&sink{w: &output})
			var run func()
			l.SetSpawn(func(fn func()) { run = fn })
			mustNoErr(t, l.Start(), "start deferred processor")
			var wg sync.WaitGroup
			for worker := range workers {
				wg.Go(func() {
					for i := range perWorker {
						l.Info(strconv.Itoa(worker*perWorker+i) + "\n")
					}
				})
			}
			wg.Wait()
			total := uint64(workers * perWorker)
			equal(t, l.state.TotalDroppedLogs.Load(), total-uint64(capacity), "exact overflow count")
			go run()
			mustNoErr(t, l.Shutdown(5*time.Second), "drain")
			seen := assertUniqueRecords(t, output.String(), int(total))
			equal(t, len(seen), int(capacity), "accepted records all written")
			equal(t, l.state.TotalLogsProcessed.Load()+l.state.TotalDroppedLogs.Load(), total, "no unaccounted records")
		})
	}
}

func TestStressLifecycleAccounting(t *testing.T) {
	const workers, perWorker = 8, 1000
	l := NewLogger()
	c := DefaultConfig()
	c.EnableConsole = false
	c.EnableFile = true
	c.Directory = t.TempDir()
	c.Format = "raw"
	c.BufferSize = 256
	c.MaxSizeKB = 0
	c.MaxTotalSizeKB = 0
	c.MinDiskFreeKB = 0
	mustNoErr(t, l.ApplyConfig(c), "apply")
	mustNoErr(t, l.Start(), "start")
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() {
			<-start
			for i := range perWorker {
				// Exercise the enqueue boundary directly: public level-gated calls made
				// while stopped intentionally never become attempted records.
				l.sendLogRecord(logRecord{Args: []any{strconv.Itoa(worker*perWorker+i) + "\n"}})
			}
		})
	}
	wg.Go(func() {
		<-start
		for i := range 24 {
			noErr(t, l.Stop(time.Second), "stop")
			noErr(t, l.Start(), "start")
			noErr(t, l.ApplyConfigString(fmt.Sprintf("buffer_size=%d", 128+128*(i%2))), "reconfigure")
		}
	})
	close(start)
	wg.Wait()
	mustNoErr(t, l.Shutdown(5*time.Second), "final shutdown")
	seen := assertUniqueRecords(t, readLog(t, c.Directory), workers*perWorker)
	equal(t, uint64(len(seen)), l.state.TotalLogsProcessed.Load(), "written equals processed")
	equal(t, l.state.TotalLogsProcessed.Load()+l.state.TotalDroppedLogs.Load(), uint64(workers*perWorker), "every attempted enqueue processed or dropped")
}

func TestStressRotationPreservesRecords(t *testing.T) {
	const total = 4096
	l := NewLogger()
	c := DefaultConfig()
	c.EnableConsole = false
	c.EnableFile = true
	c.Directory = t.TempDir()
	c.BufferSize = total
	c.Format = "raw"
	c.MaxSizeKB = 1
	c.MaxTotalSizeKB = 0
	c.MinDiskFreeKB = 0
	mustNoErr(t, l.ApplyConfig(c), "apply")
	mustNoErr(t, l.Start(), "start")
	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Go(func() {
			for i := range total / 16 {
				l.Info(strconv.Itoa(worker*(total/16)+i) + "\n")
			}
		})
	}
	wg.Wait()
	mustNoErr(t, l.Shutdown(5*time.Second), "shutdown")
	equal(t, len(assertUniqueRecords(t, readAllLogs(t, c.Directory), total)), total, "records survive rotation")
	isTrue(t, l.state.TotalRotations.Load() > 1, "multiple rotations")
	var actualSize int64
	files, err := os.ReadDir(c.Directory)
	mustNoErr(t, err, "list")
	for _, file := range files {
		info, err := os.Stat(filepath.Join(c.Directory, file.Name()))
		mustNoErr(t, err, "stat")
		actualSize += info.Size()
	}
	var expectedSize int64
	for i := range total {
		expectedSize += int64(len(strconv.Itoa(i)) + 1)
	}
	equal(t, actualSize, expectedSize, "exact bytes across archives")
	equal(t, l.state.TotalDroppedLogs.Load(), uint64(0), "no drops")
}

func assertUniqueRecords(t *testing.T, data string, total int) map[int]bool {
	t.Helper()
	seen := make(map[int]bool)
	for _, line := range strings.Split(strings.TrimSuffix(data, "\n"), "\n") {
		if line == "" && data == "" {
			continue
		}
		id, err := strconv.Atoi(line)
		if err != nil || id < 0 || id >= total {
			t.Fatalf("invalid record %q", line)
		}
		if seen[id] {
			t.Fatalf("duplicate record %d", id)
		}
		seen[id] = true
	}
	return seen
}
