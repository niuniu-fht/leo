package handler

import (
	"errors"
	"testing"
)

func TestNormalizeVideoModelIDSupportsSora2(t *testing.T) {
	modelID, ok := normalizeVideoModelID("sora2")
	if !ok {
		t.Fatal("normalizeVideoModelID did not accept sora2")
	}
	if modelID != "sora-2" {
		t.Fatalf("modelID = %q, want sora-2", modelID)
	}
	if publicVideoModelID(modelID) != "sora2" {
		t.Fatalf("publicVideoModelID(%q) = %q, want sora2", modelID, publicVideoModelID(modelID))
	}
	if aliasModelID, ok := normalizeVideoModelID("sora-2"); !ok || aliasModelID != "sora-2" {
		t.Fatalf("normalizeVideoModelID(sora-2) = %q, %v; want sora-2, true", aliasModelID, ok)
	}
}

func TestNormalizeVideoModelIDSupportsKlingO3(t *testing.T) {
	modelID, ok := normalizeVideoModelID("ko3")
	if !ok {
		t.Fatal("normalizeVideoModelID did not accept ko3")
	}
	if modelID != "kling-video-o-3" {
		t.Fatalf("modelID = %q, want kling-video-o-3", modelID)
	}
	if publicVideoModelID(modelID) != "ko3" {
		t.Fatalf("publicVideoModelID(%q) = %q, want ko3", modelID, publicVideoModelID(modelID))
	}
	if aliasModelID, ok := normalizeVideoModelID("kling-o3"); !ok || aliasModelID != "kling-video-o-3" {
		t.Fatalf("normalizeVideoModelID(kling-o3) = %q, %v; want kling-video-o-3, true", aliasModelID, ok)
	}
}

func TestNormalizeVideoModelIDSupportsMinimaxH3(t *testing.T) {
	modelID, ok := normalizeVideoModelID("minimax-h3")
	if !ok || modelID != "minimax-h3" {
		t.Fatalf("normalizeVideoModelID(minimax-h3) = %q, %v; want minimax-h3, true", modelID, ok)
	}
	for _, input := range []string{"h3", "hailuo-03"} {
		if modelID, ok := normalizeVideoModelID(input); ok {
			t.Fatalf("normalizeVideoModelID(%q) = %q, true; alias should be rejected", input, modelID)
		}
	}
	if got := publicVideoModelID("hailuo-03"); got != "minimax-h3" {
		t.Fatalf("publicVideoModelID(hailuo-03) = %q, want minimax-h3", got)
	}
}

func TestPublicRequestLogModelUsesMinimaxH3(t *testing.T) {
	tests := map[string]string{
		"minimax-h3":               "minimax-h3",
		"hailuo-03":                "minimax-h3",
		"hailuo-03 (1440x2560 5s)": "minimax-h3 (1440x2560 5s)",
	}
	for input, want := range tests {
		if got := publicRequestLogModel(input); got != want {
			t.Fatalf("publicRequestLogModel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeVideoModelIDSupportsSeedanceMini(t *testing.T) {
	modelID, ok := normalizeVideoModelID("seedance-2.0-mini")
	if !ok {
		t.Fatal("normalizeVideoModelID did not accept seedance-2.0-mini")
	}
	if modelID != "seedance-2.0-mini" {
		t.Fatalf("modelID = %q, want seedance-2.0-mini", modelID)
	}
	if publicVideoModelID(modelID) != "video-2.0-mini" {
		t.Fatalf("publicVideoModelID(%q) = %q, want video-2.0-mini", modelID, publicVideoModelID(modelID))
	}
	if aliasModelID, ok := normalizeVideoModelID("video-2.0-mini"); !ok || aliasModelID != "seedance-2.0-mini" {
		t.Fatalf("normalizeVideoModelID(video-2.0-mini) = %q, %v; want seedance-2.0-mini, true", aliasModelID, ok)
	}
	if !isSeedanceModelID(modelID) {
		t.Fatalf("isSeedanceModelID(%q) = false, want true", modelID)
	}
}

func TestNormalizeVideoModelIDSupportsSeedance480pVariants(t *testing.T) {
	tests := []struct {
		input string
		want  string
		pub   string
	}{
		{input: "seedance-2.0-480p", want: "seedance-2.0-480p", pub: "video-2.0-480p"},
		{input: "video-2.0-480p", want: "seedance-2.0-480p", pub: "video-2.0-480p"},
		{input: "seedance-2.0-fast-480p", want: "seedance-2.0-fast-480p", pub: "video-2.0-fast-480p"},
		{input: "video-2.0-fast-480p", want: "seedance-2.0-fast-480p", pub: "video-2.0-fast-480p"},
		{input: "seedance-2.0-mini-480p", want: "seedance-2.0-mini-480p", pub: "video-2.0-mini-480p"},
		{input: "video-2.0-mini-480p", want: "seedance-2.0-mini-480p", pub: "video-2.0-mini-480p"},
	}
	for _, tt := range tests {
		modelID, ok := normalizeVideoModelID(tt.input)
		if !ok || modelID != tt.want {
			t.Fatalf("normalizeVideoModelID(%q) = %q, %v; want %q, true", tt.input, modelID, ok, tt.want)
		}
		if got := publicVideoModelID(modelID); got != tt.pub {
			t.Fatalf("publicVideoModelID(%q) = %q, want %q", modelID, got, tt.pub)
		}
		if !isSeedanceModelID(modelID) || !isSeedance480pModelID(modelID) {
			t.Fatalf("%q should be detected as Seedance 480p", modelID)
		}
	}
}

func TestSora2DefaultsMatchTextToVideoCapture(t *testing.T) {
	if got := defaultVideoDuration("sora2"); got != 8 {
		t.Fatalf("defaultVideoDuration(sora2) = %d, want 8", got)
	}
	width, height := defaultVideoSize("sora2")
	if width != 720 || height != 1280 {
		t.Fatalf("defaultVideoSize(sora2) = %dx%d, want 720x1280", width, height)
	}
}

func TestKlingO3DefaultsMatchTextToVideoCapture(t *testing.T) {
	if got := defaultVideoDuration("kling-video-o-3"); got != 3 {
		t.Fatalf("defaultVideoDuration(kling-video-o-3) = %d, want 3", got)
	}
	width, height := defaultVideoSize("kling-video-o-3")
	if width != 1080 || height != 1920 {
		t.Fatalf("defaultVideoSize(kling-video-o-3) = %dx%d, want 1080x1920", width, height)
	}
	if got := leonardoVideoResolutionMode("kling-video-o-3", width, height); got != "RESOLUTION_1080" {
		t.Fatalf("leonardoVideoResolutionMode(kling-video-o-3) = %q, want RESOLUTION_1080", got)
	}
}

func TestMinimaxH3DefaultsAndAllowedValues(t *testing.T) {
	if got := defaultVideoDuration("minimax-h3"); got != 5 {
		t.Fatalf("defaultVideoDuration(minimax-h3) = %d, want 5", got)
	}
	width, height := defaultVideoSize("minimax-h3")
	if width != 2560 || height != 1440 {
		t.Fatalf("defaultVideoSize(minimax-h3) = %dx%d, want 2560x1440", width, height)
	}
	for duration := 5; duration <= 15; duration++ {
		if !isAllowedMinimaxH3Duration(duration) {
			t.Fatalf("duration %d should be allowed for minimax-h3", duration)
		}
	}
	if isAllowedMinimaxH3Duration(4) || isAllowedMinimaxH3Duration(16) {
		t.Fatal("minimax-h3 should reject durations outside 5-15 seconds")
	}
	for _, size := range [][2]int{{2560, 1440}, {1440, 2560}, {1440, 1440}, {1920, 1440}, {1440, 1920}, {3360, 1440}} {
		if !isAllowedMinimaxH3Size(size[0], size[1]) {
			t.Fatalf("size %dx%d should be allowed for minimax-h3", size[0], size[1])
		}
	}
	if isAllowedMinimaxH3Size(1920, 1080) {
		t.Fatal("1920x1080 should not be allowed for minimax-h3")
	}
}

func TestMinimaxH3GuidanceRules(t *testing.T) {
	if err := validateMinimaxH3GuidanceInput(map[string]interface{}{"image_url": "https://example.com/one.png"}); err != nil {
		t.Fatalf("single image should use image-reference mode: %v", err)
	}
	if err := validateMinimaxH3GuidanceInput(map[string]interface{}{
		"image_url": "https://example.com/one.png",
		"audio_url": "https://example.com/reference.mp3",
	}); err != nil {
		t.Fatalf("image + audio should be allowed: %v", err)
	}
	if err := validateMinimaxH3GuidanceInput(map[string]interface{}{
		"start_frame": []interface{}{map[string]interface{}{"id": "start"}},
		"end_frame":   []interface{}{map[string]interface{}{"id": "end"}},
	}); err != nil {
		t.Fatalf("explicit start/end frames should be allowed: %v", err)
	}

	sixImages := make([]interface{}, 6)
	for i := range sixImages {
		sixImages[i] = map[string]interface{}{"id": "image"}
	}
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{name: "six images", data: map[string]interface{}{"image_guidance": sixImages}},
		{name: "audio without image", data: map[string]interface{}{"audio_url": "https://example.com/reference.mp3"}},
		{name: "audio with frames", data: map[string]interface{}{
			"start_frame": []interface{}{map[string]interface{}{"id": "start"}},
			"audio_url":   "https://example.com/reference.mp3",
		}},
		{name: "mixed image and frames", data: map[string]interface{}{
			"image_url":   "https://example.com/one.png",
			"start_frame": []interface{}{map[string]interface{}{"id": "start"}},
		}},
		{name: "video reference", data: map[string]interface{}{
			"video_reference": []interface{}{map[string]interface{}{"id": "video"}},
		}},
	}
	for _, tt := range tests {
		if err := validateMinimaxH3GuidanceInput(tt.data); err == nil {
			t.Fatalf("%s should be rejected", tt.name)
		}
	}
}

func TestSora2AllowedDurationsAndSizes(t *testing.T) {
	for _, duration := range []int{4, 8, 12} {
		if !isAllowedSora2Duration(duration) {
			t.Fatalf("duration %d should be allowed for sora2", duration)
		}
	}
	for _, duration := range []int{5, 10, 15} {
		if isAllowedSora2Duration(duration) {
			t.Fatalf("duration %d should not be allowed for sora2", duration)
		}
	}
	if !isAllowedSora2Size(720, 1280) {
		t.Fatal("720x1280 should be allowed for sora2")
	}
	if !isAllowedSora2Size(1280, 720) {
		t.Fatal("1280x720 should be allowed for sora2")
	}
	if isAllowedSora2Size(960, 960) {
		t.Fatal("960x960 should not be allowed for sora2")
	}
}

func TestKlingO3AllowedDurationsSizesAndGuidance(t *testing.T) {
	if !isAllowedKlingO3Duration(3, false) {
		t.Fatal("duration 3 should be allowed for kling-o3")
	}
	if !isAllowedKlingO3Duration(4, false) {
		t.Fatal("duration 4 should be allowed for kling-o3")
	}
	if !isAllowedKlingO3Duration(15, false) {
		t.Fatal("duration 15 should be allowed for kling-o3")
	}
	if isAllowedKlingO3Duration(16, false) {
		t.Fatal("duration 16 should not be allowed for kling-o3")
	}
	if !isAllowedKlingO3Size(1080, 1920, false) {
		t.Fatal("1080x1920 should be allowed for kling-o3")
	}
	if !isAllowedKlingO3Size(1920, 1080, false) {
		t.Fatal("1920x1080 should be allowed for kling-o3")
	}
	if !isAllowedKlingO3Size(1440, 1440, false) {
		t.Fatal("1440x1440 should be allowed for kling-o3")
	}
	if isAllowedKlingO3Size(1280, 720, false) {
		t.Fatal("1280x720 should not be allowed for kling-o3")
	}
	if !isAllowedKlingO3Duration(5, true) {
		t.Fatal("duration 5 should be allowed for kling-o3 video reference")
	}
	if !isAllowedKlingO3Duration(3, true) {
		t.Fatal("duration 3 should be allowed for kling-o3 video reference")
	}
	if isAllowedKlingO3Duration(2, true) {
		t.Fatal("duration 2 should not be allowed for kling-o3 video reference")
	}
	if !isAllowedKlingO3Size(0, 0, true) {
		t.Fatal("0x0 should be allowed for kling-o3 video reference")
	}
	if hasUnsupportedKlingO3GuidanceInput(map[string]interface{}{"prompt": "text only"}) {
		t.Fatal("text-only request should not have unsupported kling-o3 guidance input")
	}
	if hasUnsupportedKlingO3GuidanceInput(map[string]interface{}{"image_url": "https://example.com/a.png"}) {
		t.Fatal("image_url should be allowed as Kling O3 image-reference guidance")
	}
	if hasUnsupportedKlingO3GuidanceInput(map[string]interface{}{"image_guidance": []interface{}{map[string]interface{}{"id": "img"}}}) {
		t.Fatal("image_guidance should be allowed as Kling O3 image-reference guidance")
	}
	if hasUnsupportedKlingO3GuidanceInput(map[string]interface{}{"start_frame": []interface{}{map[string]interface{}{"id": "img"}}}) {
		t.Fatal("start_frame should be allowed as Kling O3 start-frame guidance")
	}
	if hasUnsupportedKlingO3GuidanceInput(map[string]interface{}{"end_frame": []interface{}{map[string]interface{}{"id": "img"}}}) {
		t.Fatal("end_frame should be allowed as Kling O3 end-frame guidance")
	}
	if hasUnsupportedKlingO3GuidanceInput(map[string]interface{}{
		"image_guidance": []interface{}{
			map[string]interface{}{"id": "img-1"},
			map[string]interface{}{"id": "img-2"},
			map[string]interface{}{"id": "img-3"},
			map[string]interface{}{"id": "img-4"},
		},
	}) {
		t.Fatal("multiple image_guidance entries should be allowed as Kling O3 image-reference guidance")
	}
	if hasUnsupportedKlingO3GuidanceInput(map[string]interface{}{"video_reference": []interface{}{map[string]interface{}{"id": "vid"}}}) {
		t.Fatal("video_reference should be allowed as Kling O3 video-reference guidance")
	}
}

func TestVideoSizeForAspectRatio(t *testing.T) {
	minimaxSizes := map[string][2]int{
		"16:9": {2560, 1440},
		"9:16": {1440, 2560},
		"1:1":  {1440, 1440},
		"4:3":  {1920, 1440},
		"3:4":  {1440, 1920},
		"21:9": {3360, 1440},
	}
	for ratio, want := range minimaxSizes {
		width, height, ok := videoSizeForAspectRatio("minimax-h3", ratio)
		if !ok || width != want[0] || height != want[1] {
			t.Fatalf("minimax-h3 %s = %dx%d, %v; want %dx%d, true", ratio, width, height, ok, want[0], want[1])
		}
	}

	width, height, ok := videoSizeForAspectRatio("sora2", "16:9")
	if !ok || width != 1280 || height != 720 {
		t.Fatalf("sora2 16:9 = %dx%d, %v; want 1280x720, true", width, height, ok)
	}
	width, height, ok = videoSizeForAspectRatio("sora2", "9:16")
	if !ok || width != 720 || height != 1280 {
		t.Fatalf("sora2 9:16 = %dx%d, %v; want 720x1280, true", width, height, ok)
	}
	if _, _, ok := videoSizeForAspectRatio("sora2", "1:1"); ok {
		t.Fatal("sora2 1:1 should not be allowed")
	}
	width, height, ok = videoSizeForAspectRatio("ko3", "16:9")
	if !ok || width != 1920 || height != 1080 {
		t.Fatalf("ko3 16:9 = %dx%d, %v; want 1920x1080, true", width, height, ok)
	}
	width, height, ok = videoSizeForAspectRatio("video-2.0-fast", "1:1")
	if !ok || width != 960 || height != 960 {
		t.Fatalf("video-2.0-fast 1:1 = %dx%d, %v; want 960x960, true", width, height, ok)
	}
	width, height, ok = videoSizeForAspectRatio("video-2.0-fast-480p", "9:16")
	if !ok || width != 496 || height != 864 {
		t.Fatalf("video-2.0-fast-480p 9:16 = %dx%d, %v; want 496x864, true", width, height, ok)
	}
	width, height, ok = videoSizeForAspectRatio("video-2.0-mini-480p", "16:9")
	if !ok || width != 864 || height != 496 {
		t.Fatalf("video-2.0-mini-480p 16:9 = %dx%d, %v; want 864x496, true", width, height, ok)
	}
	width, height, ok = videoSizeForAspectRatio("video-2.0-480p", "1:1")
	if !ok || width != 640 || height != 640 {
		t.Fatalf("video-2.0-480p 1:1 = %dx%d, %v; want 640x640, true", width, height, ok)
	}
	if !isAllowedSeedance480pSize(496, 864) || !isAllowedSeedance480pSize(864, 496) || !isAllowedSeedance480pSize(640, 640) {
		t.Fatal("expected all Seedance 480p sizes to be allowed")
	}
	if isAllowedSeedance480pSize(720, 1280) {
		t.Fatal("720x1280 should not be allowed for Seedance 480p")
	}
}

func TestMinimaxH3ModelParamsKeepWidthAndHeightOrder(t *testing.T) {
	if got := videoModelParams("minimax-h3", 3360, 1440, 15); got != "3360x1440 15s" {
		t.Fatalf("videoModelParams(minimax-h3) = %q; want %q", got, "3360x1440 15s")
	}
}

func TestVideoGenerationAsyncPathCompatibility(t *testing.T) {
	const generationID = "gen-123"
	if got := videoGenerationPollURL("/v1/video/generations", generationID); got != "/v1/video/generations/gen-123" {
		t.Fatalf("poll url = %q", got)
	}
	if got := videoGenerationPollURL("/v1/video/async-generations", generationID); got != "/v1/video/async-generations/gen-123" {
		t.Fatalf("async poll url = %q", got)
	}
	if got := videoGenerationIDFromPath("/v1/video/generations/gen-123"); got != generationID {
		t.Fatalf("generation id = %q", got)
	}
	if got := videoGenerationIDFromPath("/v1/video/async-generations/gen-123"); got != generationID {
		t.Fatalf("async generation id = %q", got)
	}
}

func TestSora2UnsupportedGuidanceDetection(t *testing.T) {
	if hasUnsupportedSora2GuidanceInput(map[string]interface{}{"prompt": "text only"}) {
		t.Fatal("text-only request should not have unsupported guidance input")
	}
	if hasUnsupportedSora2GuidanceInput(map[string]interface{}{"image_url": "https://example.com/a.png"}) {
		t.Fatal("image_url should be allowed as Sora 2 start-frame guidance")
	}
	if hasUnsupportedSora2GuidanceInput(map[string]interface{}{"start_image_url": "https://example.com/a.png"}) {
		t.Fatal("start_image_url should be allowed as Sora 2 start-frame guidance")
	}
	if hasUnsupportedSora2GuidanceInput(map[string]interface{}{"start_frame": []interface{}{map[string]interface{}{"id": "img"}}}) {
		t.Fatal("start_frame should be allowed as Sora 2 start-frame guidance")
	}
	if !hasUnsupportedSora2GuidanceInput(map[string]interface{}{"video_reference": []interface{}{map[string]interface{}{"id": "vid"}}}) {
		t.Fatal("video_reference should be detected as unsupported Sora 2 guidance input")
	}
	if !hasUnsupportedSora2GuidanceInput(map[string]interface{}{"end_image_url": "https://example.com/end.png"}) {
		t.Fatal("end_image_url should be detected as unsupported Sora 2 guidance input")
	}
}

func TestCountSora2StartFrameInputs(t *testing.T) {
	if got := countSora2StartFrameInputs(map[string]interface{}{"prompt": "text only"}); got != 0 {
		t.Fatalf("count = %d, want 0", got)
	}
	if got := countSora2StartFrameInputs(map[string]interface{}{"image_url": "https://example.com/a.png"}); got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}
	got := countSora2StartFrameInputs(map[string]interface{}{
		"image_url":       "https://example.com/a.png",
		"start_image_url": "https://example.com/b.png",
		"start_frame":     []interface{}{map[string]interface{}{"id": "img"}},
	})
	if got != 3 {
		t.Fatalf("count = %d, want 3", got)
	}
}

func TestRetryableGuidancePreparationError(t *testing.T) {
	retryable := []string{
		`invalid image_urls[0]: upload init failed: graphql request failed: Post "https://api.leonardo.ai/v1/graphql": EOF`,
		`invalid image_urls[2]: s3 upload failed: s3 upload returned 403: Policy expired`,
		`invalid start_frame[0]: s3 upload failed: s3 upload failed after 3 attempt(s): read: connection reset by peer`,
		`invalid video_reference[0]: wait for staged video asset failed: context deadline exceeded`,
		`invalid image_urls[0]: wait for init image failed: moderation polling returned unknown error`,
	}
	for _, msg := range retryable {
		if !isRetryableGuidancePreparationError(errors.New(msg)) {
			t.Fatalf("expected retryable guidance preparation error for %q", msg)
		}
	}

	nonRetryable := []string{
		`invalid image_url: image url returned 400`,
		`invalid image_urls[0]: image url returned 404`,
		`invalid image_urls[0]: image url did not return an image content type`,
		`invalid image_urls[0]: either id or url is required`,
	}
	for _, msg := range nonRetryable {
		if isRetryableGuidancePreparationError(errors.New(msg)) {
			t.Fatalf("expected non-retryable guidance preparation error for %q", msg)
		}
	}
}

func TestRequiredCreditsForVideoModel(t *testing.T) {
	tests := []struct {
		modelID string
		want    float64
		ok      bool
	}{
		{modelID: "video-2.0", want: video2RequiredCredits, ok: true},
		{modelID: "seedance-2.0", want: video2RequiredCredits, ok: true},
		{modelID: "video-2.0-480p", want: video2Required480pCredits, ok: true},
		{modelID: "seedance-2.0-480p", want: video2Required480pCredits, ok: true},
		{modelID: "video-2.0-fast", want: video2FastRequiredCredits, ok: true},
		{modelID: "seedance-2.0-fast", want: video2FastRequiredCredits, ok: true},
		{modelID: "video-2.0-fast-480p", want: video2FastRequired480pCredits, ok: true},
		{modelID: "seedance-2.0-fast-480p", want: video2FastRequired480pCredits, ok: true},
		{modelID: "video-2.0-mini", want: video2MiniRequiredCredits, ok: true},
		{modelID: "seedance-2.0-mini", want: video2MiniRequiredCredits, ok: true},
		{modelID: "video-2.0-mini-480p", want: video2MiniRequired480pCredits, ok: true},
		{modelID: "seedance-2.0-mini-480p", want: video2MiniRequired480pCredits, ok: true},
		{modelID: "sora2", want: sora2RequiredCredits, ok: true},
		{modelID: "sora-2", want: sora2RequiredCredits, ok: true},
		{modelID: "ko3", want: klingO3RequiredCredits, ok: true},
		{modelID: "kling-o3", want: klingO3RequiredCredits, ok: true},
		{modelID: "minimax-h3", want: minimaxH3RequiredCredits, ok: true},
	}
	for _, tt := range tests {
		got, ok := requiredCreditsForVideoModel(tt.modelID)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("requiredCreditsForVideoModel(%q) = %.0f, %v; want %.0f, %v", tt.modelID, got, ok, tt.want, tt.ok)
		}
	}
}

func TestRequiredCreditsForKlingO3VideoReference(t *testing.T) {
	for _, modelID := range []string{"ko3", "kling-o3", "kling-video-o-3"} {
		got, ok := requiredCreditsForVideoRequest(modelID, true)
		if !ok || got != klingO3VideoRefRequiredCredits {
			t.Fatalf("requiredCreditsForVideoRequest(%q, true) = %.0f, %v; want %.0f, true", modelID, got, ok, float64(klingO3VideoRefRequiredCredits))
		}
	}
}

func TestTokenCreditsAvailable(t *testing.T) {
	if got, ok := tokenCreditsAvailable(map[string]interface{}{"credits_available": 3950}); !ok || got != 3950 {
		t.Fatalf("credits_available = %.0f, %v; want 3950, true", got, ok)
	}
	if got, ok := tokenCreditsAvailable(map[string]interface{}{"credits": 8500}); !ok || got != 8500 {
		t.Fatalf("credits = %.0f, %v; want 8500, true", got, ok)
	}
	if got, ok := tokenCreditsAvailable(map[string]interface{}{"max_credits": 8500}); !ok || got != 0 {
		t.Fatalf("max_credits-only = %.0f, %v; want 0, true", got, ok)
	}
	if _, ok := tokenCreditsAvailable(map[string]interface{}{}); ok {
		t.Fatal("empty token info should not have known credits")
	}
}
