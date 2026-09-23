package handler

import (
	"testing"
	"time"
)

func TestShouldRunTokenRenewalRecoveryWaitsForNextDay(t *testing.T) {
	s := &Server{}
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.Local)
	item := map[string]interface{}{"exhausted_at": float64(now.Add(-time.Hour).Unix())}

	if s.shouldRunTokenRenewalRecovery(item, "tok-a", now) {
		t.Fatal("token exhausted earlier today must not recover before the next day")
	}

	nextDay := time.Date(2026, 9, 24, 0, 5, 0, 0, time.Local)
	if !s.shouldRunTokenRenewalRecovery(item, "tok-a", nextDay) {
		t.Fatal("token exhausted yesterday should be eligible after midnight")
	}

	if !s.shouldRunTokenRenewalRecovery(map[string]interface{}{}, "tok-b", now) {
		t.Fatal("legacy token without exhausted_at should be eligible once")
	}

	s.autoRefreshRun = map[string]time.Time{"tok-a": nextDay.Add(-10 * time.Minute)}
	if s.shouldRunTokenRenewalRecovery(item, "tok-a", nextDay) {
		t.Fatal("probe cooldown should still apply within the same window")
	}
}
