package handler

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

// DailyCounter persists a per-local-day counter (e.g. successful image
// generations) in its own small JSON file, so the number survives request-log
// rotation and process restarts. Increment holds the mutex for a few
// instructions only; disk writes are debounced to a 1s background flusher so
// the generation hot path never touches the filesystem.
type DailyCounter struct {
	mu    sync.Mutex
	path  string
	date  string
	count int64
	dirty bool
}

// NewDailyCounter loads the counter file; counts from a previous day are
// discarded (the counter is per-day by definition).
func NewDailyCounter(path string) *DailyCounter {
	dc := &DailyCounter{path: path, date: time.Now().Format("2006-01-02")}
	if raw, err := os.ReadFile(path); err == nil {
		var saved struct {
			Date  string `json:"date"`
			Count int64  `json:"count"`
		}
		if json.Unmarshal(raw, &saved) == nil && saved.Date == dc.date && saved.Count > 0 {
			dc.count = saved.Count
		}
	}
	return dc
}

// Increment adds n to today's count and returns the new value. Crossing
// midnight resets the counter to zero first.
func (dc *DailyCounter) Increment(n int64) int64 {
	if dc == nil || n <= 0 {
		return 0
	}
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.rolloverLocked()
	dc.count += n
	dc.dirty = true
	return dc.count
}

// Snapshot returns the current date and count.
func (dc *DailyCounter) Snapshot() (string, int64) {
	if dc == nil {
		return "", 0
	}
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.rolloverLocked()
	return dc.date, dc.count
}

func (dc *DailyCounter) rolloverLocked() {
	today := time.Now().Format("2006-01-02")
	if today != dc.date {
		dc.date = today
		dc.count = 0
		dc.dirty = true
	}
}

// StartFlusher persists dirty state once per second for the process lifetime.
func (dc *DailyCounter) StartFlusher() {
	if dc == nil {
		return
	}
	go func() {
		for {
			time.Sleep(time.Second)
			dc.flushOnce()
		}
	}()
}

func (dc *DailyCounter) flushOnce() {
	dc.mu.Lock()
	if !dc.dirty {
		dc.mu.Unlock()
		return
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"date":       dc.date,
		"count":      dc.count,
		"updated_at": time.Now().Unix(),
	})
	dc.dirty = false
	dc.mu.Unlock()
	tmp := dc.path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		log.Printf("[daily_counter] write %s failed: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, dc.path); err != nil {
		log.Printf("[daily_counter] rename to %s failed: %v", dc.path, err)
	}
}
