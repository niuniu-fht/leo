package reqlog

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"leo2api/internal/store"
)

// Entry represents a single generation request log.
type Entry struct {
	ID                   string  `json:"id"`
	Timestamp            float64 `json:"ts"`
	StatusCode           int     `json:"status_code"`
	TaskStatus           string  `json:"task_status"` // "IN_PROGRESS", "COMPLETE", "FAILED"
	Type                 string  `json:"type"`        // "image", "video"
	InputMode            string  `json:"input_mode,omitempty"`
	ReferenceCount       int     `json:"reference_count,omitempty"`
	DurationSec          float64 `json:"duration_sec"`
	TokenID              string  `json:"token_id,omitempty"`
	TokenAttempt         int     `json:"token_attempt,omitempty"`
	AccountName          string  `json:"token_account_name"`
	AccountEmail         string  `json:"token_account_email"`
	Model                string  `json:"model"`
	ModelParams          string  `json:"model_params,omitempty"`
	SizeTier             string  `json:"size_tier,omitempty"`
	SizeRatio            string  `json:"size_ratio,omitempty"`
	SizeTransform        string  `json:"size_transform,omitempty"`
	Prompt               string  `json:"prompt_preview"`
	ErrorCode            string  `json:"error_code,omitempty"`
	ErrorMessage         string  `json:"error_message,omitempty"`
	GenerationID         string  `json:"generation_id,omitempty"`
	UpstreamGenerationID string  `json:"upstream_generation_id,omitempty"`
	PreviewURL           string  `json:"preview_url,omitempty"`
	PreviewKind          string  `json:"preview_kind,omitempty"`
	CreditCost           int     `json:"credit_cost"`
	Operation            string  `json:"operation,omitempty"`
}

// Store is a thread-safe log store with JSON file persistence.
type Store struct {
	mu         sync.Mutex
	entries    []Entry
	filePath   string
	jsonStore  store.JSONStore
	jsonKey    string
	stats      map[string]statsCacheEntry
	maxEntries int
	// dirty-tracking persistence: save() only flags; a background flusher
	// snapshots and writes at most once per interval. Every Add/Update used to
	// marshal the whole entry list and rewrite the multi-megabyte file under
	// mu, which serialized all request logging (and stalled generations under
	// high concurrency).
	dirty     bool
	stopOnce  sync.Once
	stopFlush chan struct{}
	// running is an O(1) per-token IN_PROGRESS counter. Token scheduling
	// checks running counts for every candidate on every selection; scanning
	// the whole entry list under mu for each check serialized submissions
	// under load.
	running      map[string]int
	lastExpireAt time.Time
}

type statsCacheEntry struct {
	expiresAt time.Time
	value     map[string]interface{}
}

// reqlogFlushInterval bounds how long a log mutation can stay unpersisted.
const reqlogFlushInterval = time.Second

// NewStore creates a new log store. If filePath is non-empty, loads existing
// logs from disk and persists all changes automatically.
func NewStore(filePath string) *Store {
	return NewStoreWithJSON(filePath, nil, "")
}

// NewStoreWithJSON creates a new log store with optional JSON blob persistence.
func NewStoreWithJSON(filePath string, jsonStore store.JSONStore, jsonKey string) *Store {
	s := &Store{
		entries:    make([]Entry, 0),
		filePath:   filePath,
		jsonStore:  jsonStore,
		jsonKey:    strings.TrimSpace(jsonKey),
		maxEntries: 5000,
		stopFlush:  make(chan struct{}),
	}
	if jsonStore != nil && strings.TrimSpace(jsonKey) != "" {
		s.loadFromJSONStore()
	}
	if len(s.entries) == 0 && filePath != "" {
		s.loadFromDisk()
		if len(s.entries) > 0 && s.jsonStore != nil && s.jsonKey != "" {
			if err := s.jsonStore.SaveJSON(s.jsonKey, s.entries); err != nil {
				log.Printf("[reqlog] failed to seed %s from %s: %v", s.jsonKey, s.filePath, err)
			} else {
				log.Printf("[reqlog] seeded %d log entries into %s", len(s.entries), s.jsonKey)
			}
		}
	}
	go s.runFlusher()
	return s
}

// runFlusher persists dirty state at most once per reqlogFlushInterval.
func (s *Store) runFlusher() {
	ticker := time.NewTicker(reqlogFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.Flush()
		case <-s.stopFlush:
			s.Flush()
			return
		}
	}
}

// adjustRunningLocked maintains the per-token IN_PROGRESS counter; callers
// must hold s.mu.
func (s *Store) adjustRunningLocked(tokenID string, delta int) {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" || delta == 0 {
		return
	}
	if s.running == nil {
		s.running = make(map[string]int)
	}
	v := s.running[tokenID] + delta
	if v <= 0 {
		delete(s.running, tokenID)
		return
	}
	s.running[tokenID] = v
}

// rebuildRunningLocked recomputes the per-token running counters from the
// entry list.
func (s *Store) rebuildRunningLocked() {
	s.running = make(map[string]int)
	for _, e := range s.entries {
		if normalizeTaskStatus(e.TaskStatus) == "IN_PROGRESS" {
			s.adjustRunningLocked(e.TokenID, 1)
		}
	}
}

// Flush persists the entries when dirty. The snapshot is taken under the
// lock; the file/redis writes happen outside it.
func (s *Store) Flush() {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	s.dirty = false
	snapshot := s.entries
	var jsonStore store.JSONStore
	var jsonKey string
	if s.jsonStore != nil && s.jsonKey != "" {
		jsonStore, jsonKey = s.jsonStore, s.jsonKey
	}
	var data []byte
	if s.filePath != "" && snapshot != nil {
		var err error
		data, err = json.Marshal(snapshot)
		if err != nil {
			log.Printf("[reqlog] failed to marshal logs: %v", err)
			data = nil
		}
	}
	filePath := s.filePath
	s.mu.Unlock()

	if jsonStore != nil {
		if err := jsonStore.SaveJSON(jsonKey, snapshot); err != nil {
			log.Printf("[reqlog] failed to save %s: %v", jsonKey, err)
		}
	}
	if filePath != "" && data != nil {
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			log.Printf("[reqlog] failed to write %s: %v", filePath, err)
		}
	}
}

// loadFromDisk reads saved log entries from the JSON file.
func (s *Store) loadFromDisk() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[reqlog] failed to read %s: %v", s.filePath, err)
		}
		return
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("[reqlog] failed to parse %s: %v", s.filePath, err)
		return
	}
	for i := range entries {
		entries[i].TaskStatus = normalizeTaskStatus(entries[i].TaskStatus)
		entries[i].DurationSec = normalizeDuration(entries[i].DurationSec)
		if entries[i].ErrorMessage == "" && entries[i].ErrorCode != "" && !isNumericErrorCode(entries[i].ErrorCode) {
			entries[i].ErrorMessage = entries[i].ErrorCode
		}
	}
	s.entries = entries
	s.rebuildRunningLocked()
	log.Printf("[reqlog] loaded %d log entries from %s", len(entries), s.filePath)
}

func (s *Store) loadFromJSONStore() {
	if s.jsonStore == nil || s.jsonKey == "" {
		return
	}
	var entries []Entry
	if err := s.jsonStore.LoadJSON(s.jsonKey, &entries); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[reqlog] failed to load %s: %v", s.jsonKey, err)
		}
		return
	}
	for i := range entries {
		entries[i].TaskStatus = normalizeTaskStatus(entries[i].TaskStatus)
		entries[i].DurationSec = normalizeDuration(entries[i].DurationSec)
		if entries[i].ErrorMessage == "" && entries[i].ErrorCode != "" && !isNumericErrorCode(entries[i].ErrorCode) {
			entries[i].ErrorMessage = entries[i].ErrorCode
		}
	}
	s.entries = entries
	s.rebuildRunningLocked()
	log.Printf("[reqlog] loaded %d log entries from %s", len(entries), s.jsonKey)
}

// save invalidates caches, bounds in-memory growth, and marks the store
// dirty; the background flusher persists within ~1s. Previously every
// mutation also rewrote the whole multi-megabyte log file here under the
// store mutex, which serialized request logging under load.
func (s *Store) save() {
	s.stats = nil
	s.pruneLocked()
	s.dirty = true
}

// Add inserts a new log entry at the beginning (newest first).
func (s *Store) Add(entry Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.Timestamp == 0 {
		entry.Timestamp = float64(time.Now().Unix())
	}
	entry.TaskStatus = normalizeTaskStatus(entry.TaskStatus)
	entry.DurationSec = normalizeDuration(entry.DurationSec)
	if entry.ErrorMessage == "" && entry.ErrorCode != "" && !isNumericErrorCode(entry.ErrorCode) {
		entry.ErrorMessage = entry.ErrorCode
	}

	if entry.TaskStatus == "IN_PROGRESS" {
		s.adjustRunningLocked(entry.TokenID, 1)
	}

	// Prepend
	s.entries = append([]Entry{entry}, s.entries...)

	s.save()
}

// SetMaxEntries caps persisted logs to the newest maxEntries rows.
func (s *Store) SetMaxEntries(maxEntries int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.maxEntries = normalizeMaxEntries(maxEntries)
	before := len(s.entries)
	s.pruneLocked()
	if len(s.entries) != before {
		s.save()
	}
}

// UpdateByGenerationID updates an entry matching the generation ID.
func (s *Store) UpdateByGenerationID(genID string, taskStatus string, statusCode int, previewURL string, previewKind string, errorMessage string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.entries {
		if s.entries[i].GenerationID == genID {
			prevStatus := normalizeTaskStatus(s.entries[i].TaskStatus)
			s.entries[i].TaskStatus = normalizeTaskStatus(taskStatus)
			if prevStatus == "IN_PROGRESS" && s.entries[i].TaskStatus != "IN_PROGRESS" {
				s.adjustRunningLocked(s.entries[i].TokenID, -1)
			}
			if statusCode > 0 {
				s.entries[i].StatusCode = statusCode
				if statusCode >= 400 {
					s.entries[i].ErrorCode = strconv.Itoa(statusCode)
				}
			}
			if previewURL != "" {
				s.entries[i].PreviewURL = previewURL
			}
			if previewKind != "" {
				s.entries[i].PreviewKind = previewKind
			}
			if errorMessage != "" {
				s.entries[i].ErrorMessage = errorMessage
			}
			if s.entries[i].DurationSec <= 0 && s.entries[i].Timestamp > 0 && s.entries[i].TaskStatus != "IN_PROGRESS" {
				elapsed := time.Since(time.Unix(int64(s.entries[i].Timestamp), 0)).Seconds()
				s.entries[i].DurationSec = normalizeDuration(elapsed)
			}
			s.save()
			return true
		}
	}
	return false
}

// UpdateDuration updates the duration of an entry by generation ID.
func (s *Store) UpdateDuration(genID string, durationSec float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.entries {
		if s.entries[i].GenerationID == genID {
			s.entries[i].DurationSec = normalizeDuration(durationSec)
			s.save()
			return
		}
	}
}

// UpdateByGenerationIDIfUpstream updates a client-facing generation only when
// the observed upstream generation is still current. This prevents a stale
// status request from overwriting a replacement task submitted by a retry.
func (s *Store) UpdateByGenerationIDIfUpstream(genID string, expectedUpstreamGenerationID string, taskStatus string, statusCode int, previewURL string, previewKind string, errorMessage string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	expectedUpstreamGenerationID = strings.TrimSpace(expectedUpstreamGenerationID)
	for i := range s.entries {
		if s.entries[i].GenerationID != genID {
			continue
		}
		currentUpstreamGenerationID := strings.TrimSpace(s.entries[i].UpstreamGenerationID)
		if currentUpstreamGenerationID == "" {
			currentUpstreamGenerationID = strings.TrimSpace(s.entries[i].GenerationID)
		}
		if currentUpstreamGenerationID != expectedUpstreamGenerationID {
			return false
		}

		prevStatus := normalizeTaskStatus(s.entries[i].TaskStatus)
		s.entries[i].TaskStatus = normalizeTaskStatus(taskStatus)
		if prevStatus == "IN_PROGRESS" && s.entries[i].TaskStatus != "IN_PROGRESS" {
			s.adjustRunningLocked(s.entries[i].TokenID, -1)
		}
		if statusCode > 0 {
			s.entries[i].StatusCode = statusCode
			if statusCode >= 400 {
				s.entries[i].ErrorCode = strconv.Itoa(statusCode)
			}
		}
		if previewURL != "" {
			s.entries[i].PreviewURL = previewURL
		}
		if previewKind != "" {
			s.entries[i].PreviewKind = previewKind
		}
		if errorMessage != "" {
			s.entries[i].ErrorMessage = errorMessage
		}
		if s.entries[i].DurationSec <= 0 && s.entries[i].Timestamp > 0 && s.entries[i].TaskStatus != "IN_PROGRESS" {
			elapsed := time.Since(time.Unix(int64(s.entries[i].Timestamp), 0)).Seconds()
			s.entries[i].DurationSec = normalizeDuration(elapsed)
		}
		s.save()
		return true
	}
	return false
}

// UpdateRetryByGenerationID keeps the client-facing generation ID stable while
// recording the replacement upstream generation created by an async retry.
func (s *Store) UpdateRetryByGenerationID(genID string, upstreamGenerationID string, tokenAttempt int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.entries {
		if s.entries[i].GenerationID != genID {
			continue
		}
		s.entries[i].UpstreamGenerationID = strings.TrimSpace(upstreamGenerationID)
		if tokenAttempt > 0 {
			s.entries[i].TokenAttempt = tokenAttempt
		}
		if normalizeTaskStatus(s.entries[i].TaskStatus) != "IN_PROGRESS" {
			s.adjustRunningLocked(s.entries[i].TokenID, 1)
		}
		s.entries[i].TaskStatus = "IN_PROGRESS"
		s.entries[i].StatusCode = 200
		s.entries[i].ErrorCode = ""
		s.entries[i].ErrorMessage = ""
		s.entries[i].DurationSec = 0
		s.save()
		return true
	}
	return false
}

// UpdateAttemptByGenerationID records a retry submission attempt even when the
// upstream request fails before a replacement generation ID is returned.
func (s *Store) UpdateAttemptByGenerationID(genID string, tokenAttempt int) bool {
	if tokenAttempt <= 0 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].GenerationID != genID {
			continue
		}
		s.entries[i].TokenAttempt = tokenAttempt
		s.save()
		return true
	}
	return false
}

// FindByGenerationID returns a copy of the log entry for the given generation ID.
func (s *Store) FindByGenerationID(genID string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, entry := range s.entries {
		if entry.GenerationID == genID {
			return entry, true
		}
	}
	return Entry{}, false
}

// List returns paginated log entries with optional filtering.
func (s *Store) List(page, pageSize int, failedOnly bool) ([]Entry, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pageSize <= 0 {
		pageSize = 50
	}
	if page < 1 {
		page = 1
	}

	collectPage := func(targetPage int) ([]Entry, int) {
		start := (targetPage - 1) * pageSize
		result := make([]Entry, 0, pageSize)
		total := 0
		for _, e := range s.entries {
			taskStatus := normalizeTaskStatus(e.TaskStatus)
			if taskStatus == "IN_PROGRESS" {
				continue
			}
			if failedOnly && taskStatus != "FAILED" {
				continue
			}
			if total >= start && len(result) < pageSize {
				result = append(result, e)
			}
			total++
		}
		return result, total
	}

	result, total := collectPage(page)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
		result, _ = collectPage(page)
	}

	return result, page, totalPages
}

// Running returns entries that are still in progress.
func (s *Store) Running() []Entry {
	return s.RunningLimit(0)
}

// RunningLimit returns entries that are still in progress, capped by limit when positive.
func (s *Store) RunningLimit(limit int) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []Entry
	for _, e := range s.entries {
		if normalizeTaskStatus(e.TaskStatus) == "IN_PROGRESS" {
			result = append(result, e)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result
}

// RunningCountByToken returns the number of IN_PROGRESS entries for a token.
func (s *Store) RunningCountByToken(tokenID string) int {
	tokenID = strings.TrimSpace(tokenID)
	if s == nil || tokenID == "" {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running == nil {
		return 0
	}
	return s.running[strings.TrimSpace(tokenID)]
}


// ExpireStaleRunning marks long-running IN_PROGRESS entries as FAILED/504.
// The scan is throttled: token scheduling calls this on every availability
// check, so an unthrottled full-list scan per candidate serialized
// submissions under load.
func (s *Store) ExpireStaleRunning(timeout time.Duration, now time.Time) int {
	if timeout <= 0 {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	if !s.lastExpireAt.IsZero() && now.Sub(s.lastExpireAt) < 5*time.Second {
		s.mu.Unlock()
		return 0
	}
	s.lastExpireAt = now
	defer s.mu.Unlock()

	updated := 0
	for i := range s.entries {
		if normalizeTaskStatus(s.entries[i].TaskStatus) != "IN_PROGRESS" {
			continue
		}
		if s.entries[i].Timestamp <= 0 {
			continue
		}

		startedAt := time.Unix(int64(s.entries[i].Timestamp), 0)
		elapsed := now.Sub(startedAt)
		if elapsed < timeout {
			continue
		}

		s.adjustRunningLocked(s.entries[i].TokenID, -1)
		s.entries[i].TaskStatus = "FAILED"
		s.entries[i].StatusCode = httpStatusGatewayTimeout
		s.entries[i].ErrorCode = strconv.Itoa(httpStatusGatewayTimeout)
		s.entries[i].ErrorMessage = "Generation polling timed out"
		s.entries[i].DurationSec = normalizeDuration(elapsed.Seconds())
		updated++
	}

	if updated > 0 {
		s.save()
	}
	return updated
}

// Stats computes statistics for log entries within a time range.
func (s *Store) Stats(rangeStr string) map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if s.stats != nil {
		if cached, ok := s.stats[rangeStr]; ok && now.Before(cached.expiresAt) {
			return cloneStatsMap(cached.value)
		}
	}

	var startTime time.Time

	switch rangeStr {
	case "today":
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "7d":
		startTime = now.AddDate(0, 0, -7)
	case "30d":
		startTime = now.AddDate(0, 0, -30)
	default:
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}

	startTs := float64(startTime.Unix())
	var images, videos, total, failed, running int

	for _, e := range s.entries {
		if e.Timestamp < startTs {
			continue
		}
		total++
		taskStatus := normalizeTaskStatus(e.TaskStatus)
		if taskStatus == "IN_PROGRESS" {
			running++
		}
		if taskStatus == "FAILED" {
			failed++
		}
		switch e.Type {
		case "image":
			if taskStatus == "COMPLETE" {
				images++
			}
		case "video":
			if taskStatus == "COMPLETE" {
				videos++
			}
		}
	}

	result := map[string]interface{}{
		"generated_images": images,
		"generated_videos": videos,
		"running_requests": running,
		"total_requests":   total,
		"failed_requests":  failed,
		"start_ts":         startTs,
		"end_ts":           float64(now.Unix()),
	}
	if s.stats == nil {
		s.stats = make(map[string]statsCacheEntry)
	}
	s.stats[rangeStr] = statsCacheEntry{
		expiresAt: now.Add(15 * time.Second),
		value:     cloneStatsMap(result),
	}
	return result
}

// Clear removes all entries. Returns the count cleared.
func (s *Store) Clear() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := len(s.entries)
	s.entries = s.entries[:0]
	s.running = make(map[string]int)
	s.save()
	return n
}

func cloneStatsMap(src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func normalizeMaxEntries(maxEntries int) int {
	if maxEntries < 100 {
		return 5000
	}
	if maxEntries > 100000 {
		return 100000
	}
	return maxEntries
}

func (s *Store) pruneLocked() {
	limit := normalizeMaxEntries(s.maxEntries)
	s.maxEntries = limit
	if len(s.entries) <= limit {
		return
	}
	for _, e := range s.entries[limit:] {
		if normalizeTaskStatus(e.TaskStatus) == "IN_PROGRESS" {
			s.adjustRunningLocked(e.TokenID, -1)
		}
	}
	s.entries = s.entries[:limit]
}

func normalizeTaskStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "COMPLETED":
		return "COMPLETE"
	case "":
		return "IN_PROGRESS"
	default:
		return strings.ToUpper(strings.TrimSpace(status))
	}
}

func normalizeDuration(durationSec float64) float64 {
	if durationSec <= 0 {
		return 0
	}
	if durationSec < 0.1 {
		return 0.1
	}
	return float64(int(durationSec*10+0.5)) / 10
}

func isNumericErrorCode(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	_, err := strconv.Atoi(value)
	return err == nil
}

const httpStatusGatewayTimeout = 504
