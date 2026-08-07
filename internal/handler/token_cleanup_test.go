package handler

import (
	"testing"
	"time"

	"leo2api/internal/reqlog"
	"leo2api/internal/token"
)

func TestCleanupTokensByStatusDeletesOnlyExhaustedTokens(t *testing.T) {
	mgr := token.NewManager(nil)
	exhaustedInfo, _, err := mgr.Add("exhausted-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add exhausted token: %v", err)
	}
	activeInfo, _, err := mgr.Add("active-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add active token: %v", err)
	}
	exhaustedID := toString(exhaustedInfo["id"])
	activeID := toString(activeInfo["id"])
	if err := mgr.SetStatus(exhaustedID, "exhausted"); err != nil {
		t.Fatalf("set exhausted status: %v", err)
	}

	srv := &Server{TokenMgr: mgr}
	result := srv.cleanupTokensByStatus("exhausted")
	if result.MatchedCount != 1 || result.DeletedCount != 1 || result.FailedCount != 0 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if mgr.GetByID(exhaustedID) != nil {
		t.Fatalf("expected exhausted token to be deleted")
	}
	if mgr.GetByID(activeID) == nil {
		t.Fatalf("expected active token to remain")
	}
}

func TestCleanupTokensByStatusTreatsExpiredAsInvalid(t *testing.T) {
	mgr := token.NewManager(nil)
	expiredInfo, _, err := mgr.Add("expired-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add expired token: %v", err)
	}
	activeInfo, _, err := mgr.Add("not-expired-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add active token: %v", err)
	}
	expiredID := toString(expiredInfo["id"])
	activeID := toString(activeInfo["id"])
	if err := mgr.UpdateExpiry(expiredID, float64(time.Now().Add(-time.Hour).Unix())); err != nil {
		t.Fatalf("set expired expiry: %v", err)
	}
	if err := mgr.UpdateExpiry(activeID, float64(time.Now().Add(time.Hour).Unix())); err != nil {
		t.Fatalf("set active expiry: %v", err)
	}

	srv := &Server{TokenMgr: mgr}
	result := srv.cleanupTokensByStatus("invalid")
	if result.MatchedCount != 1 || result.DeletedCount != 1 || result.FailedCount != 0 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if mgr.GetByID(expiredID) != nil {
		t.Fatalf("expected expired token to be deleted")
	}
	if mgr.GetByID(activeID) == nil {
		t.Fatalf("expected non-expired token to remain")
	}
}

func TestCleanupExhaustedTokensSkipsRunningGeneration(t *testing.T) {
	mgr := token.NewManager(nil)
	info, _, err := mgr.Add("running-exhausted-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	tokenID := toString(info["id"])
	if err := mgr.SetStatus(tokenID, "exhausted"); err != nil {
		t.Fatalf("set exhausted status: %v", err)
	}

	logs := reqlog.NewStore("")
	logs.Add(reqlog.Entry{
		ID:           "running-generation",
		TaskStatus:   "IN_PROGRESS",
		TokenID:      tokenID,
		GenerationID: "generation-1",
	})
	srv := &Server{TokenMgr: mgr, ReqLog: logs}

	result := srv.cleanupTokensByStatus("exhausted")
	if result.MatchedCount != 1 || result.DeletedCount != 0 || result.SkippedRunningCount != 1 || result.FailedCount != 0 {
		t.Fatalf("unexpected cleanup result while running: %+v", result)
	}
	if mgr.GetByID(tokenID) == nil {
		t.Fatal("running token must not be deleted")
	}

	logs.UpdateByGenerationID("generation-1", "COMPLETE", 200, "", "", "")
	result = srv.cleanupTokensByStatus("exhausted")
	if result.MatchedCount != 1 || result.DeletedCount != 1 || result.SkippedRunningCount != 0 || result.FailedCount != 0 {
		t.Fatalf("unexpected cleanup result after completion: %+v", result)
	}
	if mgr.GetByID(tokenID) != nil {
		t.Fatal("completed exhausted token should be deleted")
	}
}

func TestCleanupExhaustedTokensSkipsPreparationLease(t *testing.T) {
	mgr := token.NewManager(nil)
	info, _, err := mgr.Add("preparing-exhausted-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	tokenID := toString(info["id"])
	if err := mgr.SetStatus(tokenID, "exhausted"); err != nil {
		t.Fatalf("set exhausted status: %v", err)
	}

	srv := &Server{TokenMgr: mgr}
	if !srv.reserveTokenPreparation(tokenID) {
		t.Fatal("expected preparation lease")
	}
	result := srv.cleanupTokensByStatus("exhausted")
	if result.MatchedCount != 1 || result.DeletedCount != 0 || result.SkippedRunningCount != 1 || result.FailedCount != 0 {
		t.Fatalf("unexpected cleanup result while preparing: %+v", result)
	}
	if mgr.GetByID(tokenID) == nil {
		t.Fatal("preparing token must not be deleted")
	}

	srv.releaseTokenPreparation(tokenID)
	result = srv.cleanupTokensByStatus("exhausted")
	if result.DeletedCount != 1 || result.SkippedRunningCount != 0 {
		t.Fatalf("unexpected cleanup result after lease release: %+v", result)
	}
}

func TestCleanupExhaustedTokensSkipsPendingCreditSettlement(t *testing.T) {
	mgr := token.NewManager(nil)
	info, _, err := mgr.Add("settling-exhausted-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	tokenID := toString(info["id"])
	if err := mgr.SetStatus(tokenID, "exhausted"); err != nil {
		t.Fatalf("set exhausted status: %v", err)
	}

	srv := &Server{TokenMgr: mgr}
	srv.beginTokenSettlement(tokenID)
	result := srv.cleanupTokensByStatus("exhausted")
	if result.DeletedCount != 0 || result.SkippedRunningCount != 1 {
		t.Fatalf("unexpected cleanup result while settling: %+v", result)
	}
	if mgr.GetByID(tokenID) == nil {
		t.Fatal("settling token must not be deleted")
	}

	srv.endTokenSettlement(tokenID)
	result = srv.cleanupTokensByStatus("exhausted")
	if result.DeletedCount != 1 || result.SkippedRunningCount != 0 {
		t.Fatalf("unexpected cleanup result after settlement: %+v", result)
	}
}
