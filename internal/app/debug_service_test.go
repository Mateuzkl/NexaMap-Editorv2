package app

import "testing"

func TestDebugServiceRingBufferAndStats(t *testing.T) {
	emitted := 0
	ds := NewDebugService(func(name string, data any) {
		if name == EventDebugLog {
			emitted++
		}
	})

	for i := 0; i < maxDebugLogs+150; i++ {
		ds.Log("info", "test", "entry")
	}
	logs := ds.Logs()
	if len(logs) > maxDebugLogs {
		t.Fatalf("ring buffer exceeded maxDebugLogs: got %d", len(logs))
	}
	if emitted != maxDebugLogs+150 {
		t.Fatalf("emitted = %d, want %d", emitted, maxDebugLogs+150)
	}

	stats := ds.ForceGC()
	if stats.Goroutines <= 0 || stats.SysMB <= 0 {
		t.Fatalf("invalid runtime stats: %+v", stats)
	}

	ds.Clear()
	if len(ds.Logs()) != 0 {
		t.Fatalf("expected empty logs after Clear")
	}
}
