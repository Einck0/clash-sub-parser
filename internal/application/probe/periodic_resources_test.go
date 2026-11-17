package probe

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// scaleProcessResources describes this Go test process, not the entire CI host.
// CPU ticks are reported in the kernel's clock-tick unit, not guessed seconds.
// RSS/heap/goroutines are endpoint samples; RSS high-water is process lifetime,
// so neither is claimed to be the peak attributable to a particular stage.
type scaleProcessResources struct {
	at                   time.Time
	mem                  runtime.MemStats
	goroutines           int
	linux                bool
	userTicks, sysTicks  uint64
	rssKiB, highWaterKiB uint64
}

func readScaleProcessResources(t *testing.T) scaleProcessResources {
	t.Helper()
	s := scaleProcessResources{at: time.Now(), goroutines: runtime.NumGoroutine(), linux: runtime.GOOS == "linux"}
	runtime.ReadMemStats(&s.mem)
	if !s.linux {
		return s
	}

	// Linux /proc/self is this test process; no host-wide process enumeration.
	stat, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		t.Fatalf("read process CPU diagnostics: %v", err)
	}
	// comm (field 2) can contain spaces and parentheses. Fields after its final
	// ')' begin at state (3), hence utime/stime (14/15) are offsets 11/12.
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 {
		t.Fatal("invalid process CPU diagnostics")
	}
	fields := strings.Fields(string(stat[end+1:]))
	if len(fields) <= 12 {
		t.Fatal("incomplete process CPU diagnostics")
	}
	s.userTicks, err = strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		t.Fatalf("parse user CPU ticks: %v", err)
	}
	s.sysTicks, err = strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		t.Fatalf("parse system CPU ticks: %v", err)
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatalf("read process memory diagnostics: %v", err)
	}
	seen := 0
	for _, line := range strings.Split(string(status), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 0 || (parts[0] != "VmRSS:" && parts[0] != "VmHWM:") {
			continue
		}
		if len(parts) != 3 || parts[2] != "kB" {
			t.Fatal("unexpected process memory diagnostic unit")
		}
		value, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			t.Fatalf("parse process memory diagnostics: %v", err)
		}
		if parts[0] == "VmRSS:" {
			s.rssKiB = value
		} else {
			s.highWaterKiB = value
		}
		seen++
	}
	if seen != 2 {
		t.Fatal("missing process RSS/high-water diagnostics")
	}
	return s
}

// observeScaleProcessResources records bounded stage deltas through the existing
// go test JSON artifact. Concurrent stages overlap: CPU/allocation deltas include
// their siblings and must not be added or treated as a coordinator-only profile.
// No machine-dependent threshold is imposed and no forced GC distorts the load.
func observeScaleProcessResources(t *testing.T, stage string) func() {
	t.Helper()
	before := readScaleProcessResources(t)
	return func() {
		t.Helper()
		after := readScaleProcessResources(t)
		t.Logf("resource_stage=%s scope=test_process elapsed=%s heap_alloc_bytes_before=%d heap_alloc_bytes_after=%d allocated_bytes_delta=%d heap_sys_bytes_after=%d gc_cycles_delta=%d goroutines_before=%d goroutines_after=%d", stage, after.at.Sub(before.at), before.mem.HeapAlloc, after.mem.HeapAlloc, after.mem.TotalAlloc-before.mem.TotalAlloc, after.mem.HeapSys, after.mem.NumGC-before.mem.NumGC, before.goroutines, after.goroutines)
		if after.linux {
			t.Logf("resource_stage=%s scope=test_process cpu_user_ticks_delta=%d cpu_system_ticks_delta=%d rss_kib_before=%d rss_kib_after=%d rss_high_water_kib_lifetime=%d", stage, after.userTicks-before.userTicks, after.sysTicks-before.sysTicks, before.rssKiB, after.rssKiB, after.highWaterKiB)
		} else {
			t.Logf("resource_stage=%s cpu_rss_unavailable_os=%s", stage, runtime.GOOS)
		}
	}
}
