package handler

import "testing"

// leonardoGPTImageAllowedDimensions is the dimension whitelist Leonardo's
// generate API enforces for the GPT Image 2 family (from its VALIDATION_ERROR
// message: "parameters.width must be one of: ...").
var leonardoGPTImageAllowedDimensions = map[int]bool{
	768: true, 832: true, 848: true, 864: true, 896: true, 928: true,
	1024: true, 1088: true, 1136: true, 1152: true, 1184: true, 1200: true,
	1248: true, 1264: true, 1344: true, 1376: true, 1536: true, 1584: true,
	1648: true, 1696: true, 1792: true, 1856: true, 2016: true, 2048: true,
	2336: true, 2448: true, 2560: true, 2880: true, 3200: true, 3264: true,
	3504: true, 3584: true, 3808: true,
}

// Every preset dimension sent upstream must be in Leonardo's whitelist,
// otherwise the request dies with VALIDATION_ERROR.
func TestGPTImage2PresetsUseAllowedDimensions(t *testing.T) {
	for scale, presets := range gptImage2OfficialPresetsByScale {
		for _, preset := range presets {
			if !leonardoGPTImageAllowedDimensions[preset.Width] {
				t.Errorf("scale %d preset %dx%d: width %d not in Leonardo allowed set", scale, preset.Width, preset.Height, preset.Width)
			}
			if !leonardoGPTImageAllowedDimensions[preset.Height] {
				t.Errorf("scale %d preset %dx%d: height %d not in Leonardo allowed set", scale, preset.Width, preset.Height, preset.Height)
			}
		}
	}
}

// The previously-invalid 21:9 entries must now hit exact-21:9 allowed pairs.
func TestGPTImage2UltraWidePresets(t *testing.T) {
	wide1k := nearestImagePresetForModel("gpt-image-2", 21.0/9.0, 1)
	if wide1k.Width != 1792 || wide1k.Height != 768 {
		t.Fatalf("1k 21:9 resolved %dx%d, want 1792x768", wide1k.Width, wide1k.Height)
	}
	wide4k := nearestImagePresetForModel("gpt-image-2", 21.0/9.0, 4)
	if wide4k.Width != 3584 || wide4k.Height != 1536 {
		t.Fatalf("4k 21:9 resolved %dx%d, want 3584x1536", wide4k.Width, wide4k.Height)
	}
	// Nearby ratios must still snap to their canonical presets (no drift).
	square := nearestImagePresetForModel("gpt-image-2", 1, 1)
	if square.Width != 1024 || square.Height != 1024 {
		t.Fatalf("1k 1:1 resolved %dx%d, want 1024x1024", square.Width, square.Height)
	}
	landscape := nearestImagePresetForModel("gpt-image-2", 16.0/9.0, 2)
	if landscape.Width != 2048 || landscape.Height != 1136 {
		t.Fatalf("2k 16:9 resolved %dx%d, want 2048x1136", landscape.Width, landscape.Height)
	}
}
