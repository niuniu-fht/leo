package leonardo

import (
	"net/http"
	"testing"
)

func TestNewClientWithUploadProxyConfigDirectOverridesBasicProxy(t *testing.T) {
	client, err := NewClientWithUploadProxyConfig(
		"http://127.0.0.1:7890",
		"direct",
		"",
	)
	if err != nil {
		t.Fatalf("NewClientWithUploadProxyConfig() error = %v", err)
	}

	basicTransport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("basic transport type = %T, want *http.Transport", client.httpClient.Transport)
	}
	if basicTransport.Proxy == nil {
		t.Fatal("basic Leonardo requests should still use the configured basic proxy")
	}

	uploadTransport, ok := client.uploadHTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("upload transport type = %T, want *http.Transport", client.uploadHTTPClient.Transport)
	}
	if uploadTransport.Proxy != nil {
		t.Fatal("direct upload policy must not configure a proxy")
	}
	if client.uploadProxyMode != "direct" {
		t.Fatalf("uploadProxyMode = %q, want direct", client.uploadProxyMode)
	}
}
