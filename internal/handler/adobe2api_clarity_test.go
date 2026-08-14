package handler

import "testing"

func TestAdobe2APIImageEditsEndpoint(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{base: "http://adobe2api:6001/v1", want: "http://adobe2api:6001/v1/images/edits"},
		{base: "https://example.com", want: "https://example.com/v1/images/edits"},
		{base: "https://example.com/api", want: "https://example.com/api/v1/images/edits"},
		{base: "https://example.com/v1/images/edits", want: "https://example.com/v1/images/edits"},
	}
	for _, tt := range tests {
		got, err := adobe2APIImageEditsEndpoint(tt.base)
		if err != nil {
			t.Fatalf("adobe2APIImageEditsEndpoint(%q) error = %v", tt.base, err)
		}
		if got != tt.want {
			t.Fatalf("adobe2APIImageEditsEndpoint(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
}

func TestResolveAdobe2APIResultURL(t *testing.T) {
	got := resolveAdobe2APIResultURL("http://adobe2api:6001/v1", "/generated/a.png")
	if got != "http://adobe2api:6001/generated/a.png" {
		t.Fatalf("resolved url = %q", got)
	}
	got = resolveAdobe2APIResultURL("http://adobe2api:6001/v1", "https://cdn.example.com/a.png")
	if got != "https://cdn.example.com/a.png" {
		t.Fatalf("absolute url = %q", got)
	}
}
