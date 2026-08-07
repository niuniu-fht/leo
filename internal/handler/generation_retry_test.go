package handler

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGenerationRetryActionSeparatesSubmitAndAsyncFailures(t *testing.T) {
	policy := generationRetryPolicy{
		Enabled:                true,
		MaxAttempts:            3,
		BackoffBase:            time.Second,
		StatusCodes:            map[int]struct{}{http.StatusBadGateway: {}},
		ErrorMatchers:          []string{"timeout", "connection"},
		SameTokenErrorMatchers: []string{"provider_moderation_error"},
	}

	moderationFailure := &videoGenerationAttemptFailure{
		StatusCode:      http.StatusBadGateway,
		Message:         "PROVIDER_MODERATION_ERROR",
		RetryCodeSource: "provider-moderation-error",
	}
	if got := policy.retryAction(moderationFailure, generationRetryPhaseAsyncTask); got != generationRetryActionSameToken {
		t.Fatalf("async moderation failure action = %q, want %q", got, generationRetryActionSameToken)
	}
	if got := policy.retryAction(moderationFailure, generationRetryPhaseSubmit); got != generationRetryActionNextToken {
		t.Fatalf("submit moderation failure action = %q, want %q", got, generationRetryActionNextToken)
	}

	asyncGeneric502 := &videoGenerationAttemptFailure{
		StatusCode: http.StatusBadGateway,
		Message:    "upstream connection reset",
	}
	if got := policy.retryAction(asyncGeneric502, generationRetryPhaseAsyncTask); got != generationRetryActionNone {
		t.Fatalf("generic async 502 action = %q, want %q", got, generationRetryActionNone)
	}
	if got := policy.retryAction(asyncGeneric502, generationRetryPhaseSubmit); got != generationRetryActionNextToken {
		t.Fatalf("generic submit 502 action = %q, want %q", got, generationRetryActionNextToken)
	}
}

func TestAsyncSubmissionRetryUsesRemainingAttemptsForTransientErrors(t *testing.T) {
	policy := generationRetryPolicy{
		Enabled:       true,
		StatusCodes:   map[int]struct{}{http.StatusServiceUnavailable: {}},
		ErrorMatchers: []string{"connection"},
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "unexpected EOF", err: errors.New("graphql request failed: unexpected EOF"), want: true},
		{name: "connection matcher", err: errors.New("connection closed by upstream"), want: true},
		{name: "configured status", err: errors.New("graphql returned 503"), want: true},
		{name: "deterministic request error", err: errors.New("duration must be at least 4 seconds"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := policy.shouldRetryAsyncSubmission(tt.err); got != tt.want {
				t.Fatalf("shouldRetryAsyncSubmission(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestAsyncRetryPublicFailureMessageDoesNotExposeTransportDetails(t *testing.T) {
	got := asyncRetryPublicFailureMessage("PROVIDER_MODERATION_ERROR")
	want := "PROVIDER_MODERATION_ERROR"
	if got != want {
		t.Fatalf("public failure message = %q, want %q", got, want)
	}
	for _, hidden := range []string{"graphql", "api.leonardo.ai", "unexpected EOF", "async retry submission"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("public failure message exposed %q: %q", hidden, got)
		}
	}
}

func TestGenerationRetryActionDisabled(t *testing.T) {
	policy := generationRetryPolicy{
		Enabled:                false,
		StatusCodes:            map[int]struct{}{http.StatusBadGateway: {}},
		SameTokenErrorMatchers: []string{"provider_moderation_error"},
	}
	failure := &videoGenerationAttemptFailure{
		StatusCode: http.StatusBadGateway,
		Message:    "PROVIDER_MODERATION_ERROR",
	}

	if got := policy.retryAction(failure, generationRetryPhaseSubmit); got != generationRetryActionNone {
		t.Fatalf("disabled submit action = %q, want %q", got, generationRetryActionNone)
	}
	if got := policy.retryAction(failure, generationRetryPhaseAsyncTask); got != generationRetryActionNone {
		t.Fatalf("disabled async action = %q, want %q", got, generationRetryActionNone)
	}
	if policy.shouldRetryAsyncSubmission(errors.New("unexpected EOF")) {
		t.Fatal("disabled policy retried an async submission error")
	}
}
