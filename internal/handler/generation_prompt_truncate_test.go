package handler

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateImagePromptForUpstream(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		want   int // expected rune count of result
	}{
		{"short ascii", "a cute cat", 10},
		{"at limit", strings.Repeat("a", 9500), 9500},
		{"ascii over limit", strings.Repeat("a", 12000), 9500},
		{"chinese under limit", strings.Repeat("好", 5000), 5000},
		{"chinese over limit (bytes far above)", strings.Repeat("好", 11000), 9500},
		{"mixed", strings.Repeat("图", 6000) + strings.Repeat("x", 6000), 9500},
	}
	for _, tc := range cases {
		got := truncateImagePromptForUpstream(tc.prompt)
		if n := utf8.RuneCountInString(got); n != tc.want {
			t.Fatalf("%s: rune count = %d, want %d", tc.name, n, tc.want)
		}
	}
	// truncation must not split a multi-byte character: result stays valid UTF-8
	got := truncateImagePromptForUpstream(strings.Repeat("好", 9600))
	if !utf8.ValidString(got) {
		t.Fatal("truncated prompt is not valid UTF-8")
	}
}

func TestCCNSFWTotalFailureMapsToSafetyReview(t *testing.T) {
	const reason = "CC_NSFW_TOTAL_FAILURE"
	if !isGenerationSafetyReviewError(reason) {
		t.Fatal("CC_NSFW_TOTAL_FAILURE should be classified as a safety review error")
	}
	if code, ok := explicitStatusCodeFromGenerationError(errString(reason)); !ok || code != 400 {
		t.Fatalf("expected explicit 400, got %d ok=%v", code, ok)
	}
	if publicGenerationErrorMessage(reason, 400) != generationSafetyReviewPublicMessage {
		t.Fatalf("expected public safety message %q, got %q", generationSafetyReviewPublicMessage, publicGenerationErrorMessage(reason, 400))
	}
}

type errString string

func (e errString) Error() string { return string(e) }
