package reqlog

import "testing"

func TestUpdateRetryKeepsPublicGenerationIDAndGuardsStaleUpdates(t *testing.T) {
	logs := NewStore("")
	logs.Add(Entry{
		GenerationID:         "public-generation",
		UpstreamGenerationID: "upstream-attempt-1",
		TaskStatus:           "FAILED",
		StatusCode:           502,
		TokenAttempt:         1,
		ErrorCode:            "502",
		ErrorMessage:         "PROVIDER_MODERATION_ERROR",
	})

	if !logs.UpdateRetryByGenerationID("public-generation", "upstream-attempt-2", 2) {
		t.Fatal("UpdateRetryByGenerationID returned false")
	}
	entry, ok := logs.FindByGenerationID("public-generation")
	if !ok {
		t.Fatal("public generation entry not found")
	}
	if entry.GenerationID != "public-generation" {
		t.Fatalf("public generation ID changed to %q", entry.GenerationID)
	}
	if entry.UpstreamGenerationID != "upstream-attempt-2" {
		t.Fatalf("upstream generation ID = %q, want upstream-attempt-2", entry.UpstreamGenerationID)
	}
	if entry.TaskStatus != "IN_PROGRESS" || entry.StatusCode != 200 || entry.TokenAttempt != 2 {
		t.Fatalf("retry state = status %q, code %d, attempt %d", entry.TaskStatus, entry.StatusCode, entry.TokenAttempt)
	}
	if entry.ErrorCode != "" || entry.ErrorMessage != "" {
		t.Fatalf("retry retained stale error: code %q, message %q", entry.ErrorCode, entry.ErrorMessage)
	}
	if !logs.UpdateAttemptByGenerationID("public-generation", 3) {
		t.Fatal("UpdateAttemptByGenerationID returned false")
	}
	entry, _ = logs.FindByGenerationID("public-generation")
	if entry.TokenAttempt != 3 || entry.UpstreamGenerationID != "upstream-attempt-2" {
		t.Fatalf("submission attempt update changed retry mapping: %+v", entry)
	}

	if logs.UpdateByGenerationIDIfUpstream("public-generation", "upstream-attempt-1", "FAILED", 502, "", "", "stale failure") {
		t.Fatal("stale upstream update unexpectedly succeeded")
	}
	entry, _ = logs.FindByGenerationID("public-generation")
	if entry.TaskStatus != "IN_PROGRESS" || entry.UpstreamGenerationID != "upstream-attempt-2" {
		t.Fatalf("stale update changed current retry state: %+v", entry)
	}

	if !logs.UpdateByGenerationIDIfUpstream("public-generation", "upstream-attempt-2", "COMPLETE", 200, "/video.mp4", "video", "") {
		t.Fatal("current upstream update returned false")
	}
	entry, _ = logs.FindByGenerationID("public-generation")
	if entry.TaskStatus != "COMPLETE" || entry.PreviewURL != "/video.mp4" {
		t.Fatalf("current upstream completion was not stored: %+v", entry)
	}
}
