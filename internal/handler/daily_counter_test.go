package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDailyCounterIncrementAndRollover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daily_success.json")
	dc := NewDailyCounter(path)
	if got := dc.Increment(1); got != 1 {
		t.Fatalf("first increment = %d, want 1", got)
	}
	if got := dc.Increment(2); got != 3 {
		t.Fatalf("second increment = %d, want 3", got)
	}
	dc.flushOnce()

	// Reload from disk in the same day keeps the count.
	dc2 := NewDailyCounter(path)
	date, count := dc2.Snapshot()
	if count != 3 {
		t.Fatalf("reloaded count = %d, want 3", count)
	}
	if date != time.Now().Format("2006-01-02") {
		t.Fatalf("date = %q, want today", date)
	}

	// A stale file from "yesterday" resets to zero.
	stale := filepath.Join(dir, "stale.json")
	_ = os.WriteFile(stale, []byte(fmt.Sprintf(`{"date":"%s","count":99}`, time.Now().AddDate(0, 0, -1).Format("2006-01-02"))), 0o644)
	dc3 := NewDailyCounter(stale)
	if _, count := dc3.Snapshot(); count != 0 {
		t.Fatalf("stale count = %d, want 0", count)
	}
}

func TestDailyCounterNilSafe(t *testing.T) {
	var dc *DailyCounter
	if got := dc.Increment(1); got != 0 {
		t.Fatalf("nil increment = %d, want 0", got)
	}
	if _, count := dc.Snapshot(); count != 0 {
		t.Fatalf("nil snapshot = %d, want 0", count)
	}
	dc.StartFlusher() // must not panic
}

func TestDailyCounterFileContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	dc := NewDailyCounter(path)
	dc.Increment(7)
	dc.flushOnce()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("counter file missing: %v", err)
	}
	if !strings.Contains(string(raw), `"count":7`) {
		t.Fatalf("unexpected file content: %s", raw)
	}
}
