package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const (
	EventDebugLog = "debug:log"
	maxDebugLogs  = 1000
)

type DebugLogEntry struct {
	ID      int64  `json:"id"`
	Time    string `json:"time"`
	Level   string `json:"level"`  // "info" | "warn" | "error"
	Source  string `json:"source"` // "backend" | "frontend" | "gc" | ...
	Message string `json:"message"`
}

type RuntimeStats struct {
	HeapAllocMB   float64 `json:"heapAllocMb"`
	HeapInuseMB   float64 `json:"heapInuseMb"`
	SysMB         float64 `json:"sysMb"`
	HeapObjects   uint64  `json:"heapObjects"`
	NumGC         uint32  `json:"numGc"`
	Goroutines    int     `json:"goroutines"`
	NativeConsole bool    `json:"nativeConsole"`
}

type DebugService struct {
	mu      sync.Mutex
	nextID  int64
	entries []DebugLogEntry
	logFile *os.File
	logPath string
	emit    func(name string, data any)
}

func NewDebugService(emit func(name string, data any)) *DebugService {
	return &DebugService{
		entries: make([]DebugLogEntry, 0, 128),
		emit:    emit,
	}
}

// SetLogFile enables or disables writing diagnostic logs to nexamap.log.
func (ds *DebugService) SetLogFile(enabled bool) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if !enabled {
		if ds.logFile != nil {
			_ = ds.logFile.Close()
			ds.logFile = nil
		}
		return nil
	}
	if ds.logFile != nil {
		return nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	logDir := filepath.Join(dir, "OTS Creator")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(logDir, "nexamap.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	ds.logFile = f
	ds.logPath = path
	return nil
}

// Write implements io.Writer so Go's standard log package streams straight into
// the bounded ring buffer, native console (when open), and the UI console.
func (ds *DebugService) Write(p []byte) (int, error) {
	text := strings.TrimSpace(string(p))
	if text == "" {
		return len(p), nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		level := "info"
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "panic") {
			level = "error"
		} else if strings.Contains(lower, "warn") || strings.Contains(lower, "damaged") {
			level = "warn"
		}
		ds.Log(level, "backend", line)
	}
	return len(p), nil
}

func (ds *DebugService) Log(level, source, message string) DebugLogEntry {
	switch level {
	case "info", "warn", "error":
	default:
		level = "info"
	}
	if source == "" {
		source = "backend"
	}
	message = strings.TrimSpace(message)

	ds.mu.Lock()
	ds.nextID++
	entry := DebugLogEntry{
		ID:      ds.nextID,
		Time:    time.Now().Format("15:04:05.000"),
		Level:   level,
		Source:  source,
		Message: message,
	}
	if len(ds.entries) >= maxDebugLogs {
		// Drop oldest 100 entries in one copy to keep amortized O(1) and bounded memory.
		drop := 100
		copy(ds.entries, ds.entries[drop:])
		ds.entries = ds.entries[:len(ds.entries)-drop]
	}
	ds.entries = append(ds.entries, entry)
	logFile := ds.logFile
	emit := ds.emit
	ds.mu.Unlock()

	formatted := fmt.Sprintf("[%s] [%s] [%s] %s", entry.Time, strings.ToUpper(entry.Level), entry.Source, entry.Message)
	writeNativeConsole(formatted)
	if logFile != nil {
		_, _ = logFile.WriteString(formatted + "\n")
	}
	if emit != nil {
		emit(EventDebugLog, entry)
	}
	return entry
}

func (ds *DebugService) Logs() []DebugLogEntry {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	out := make([]DebugLogEntry, len(ds.entries))
	copy(out, ds.entries)
	return out
}

func (ds *DebugService) Clear() {
	ds.mu.Lock()
	ds.entries = ds.entries[:0]
	ds.mu.Unlock()
}

func roundMB(bytes uint64) float64 {
	return math.Round((float64(bytes)/(1024*1024))*10) / 10
}

func (ds *DebugService) Stats() RuntimeStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return RuntimeStats{
		HeapAllocMB:   roundMB(m.HeapAlloc),
		HeapInuseMB:   roundMB(m.HeapInuse),
		SysMB:         roundMB(m.Sys),
		HeapObjects:   m.HeapObjects,
		NumGC:         m.NumGC,
		Goroutines:    runtime.NumGoroutine(),
		NativeConsole: isNativeConsoleOpen(),
	}
}

func (ds *DebugService) ForceGC() RuntimeStats {
	before := ds.Stats()
	runtime.GC()
	debug.FreeOSMemory()
	after := ds.Stats()
	ds.Log("info", "gc", fmt.Sprintf("Forced GC: Heap %.1f MB -> %.1f MB, Sys %.1f MB -> %.1f MB, Goroutines=%d",
		before.HeapAllocMB, after.HeapAllocMB, before.SysMB, after.SysMB, after.Goroutines))
	return after
}

func (ds *DebugService) ToggleNativeConsole(open bool) (bool, error) {
	if open {
		ok, err := openNativeConsole()
		if err != nil {
			return false, err
		}
		ds.Log("info", "debug", "Native Windows debug console attached.")
		return ok, nil
	}
	ok, err := closeNativeConsole()
	if err != nil {
		return isNativeConsoleOpen(), err
	}
	ds.Log("info", "debug", "Native Windows debug console detached.")
	return ok, nil
}
