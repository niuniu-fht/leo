package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultAdobe2APITimeout = 300 * time.Second
	adobe2APIClarityModel   = "gpt-image-2-clarity-free"
)

type adobe2APIImageResponse struct {
	Data []struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
		Base64  string `json:"base64"`
	} `json:"data"`
	Error interface{} `json:"error"`
}

func (s *Server) convertGeneratedImagesToTransparentWithAdobe(ctx context.Context, imageURLs []string, generationID, sizeLabel string) ([]string, error) {
	if len(imageURLs) == 0 {
		return nil, fmt.Errorf("source image url is required")
	}
	baseURL := ""
	apiKey := ""
	timeout := defaultAdobe2APITimeout
	if s != nil && s.Config != nil {
		baseURL = strings.TrimSpace(s.Config.GetString("adobe2api_base_url", ""))
		apiKey = strings.TrimSpace(s.Config.GetString("adobe2api_api_key", ""))
		if seconds := s.Config.GetInt("adobe2api_timeout_seconds", int(defaultAdobe2APITimeout/time.Second)); seconds > 0 {
			timeout = time.Duration(seconds) * time.Second
		}
	}
	if baseURL == "" {
		return nil, fmt.Errorf("adobe2api_base_url is required")
	}
	endpoint, err := adobe2APIImageEditsEndpoint(baseURL)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: timeout}
	out := make([]string, 0, len(imageURLs))
	for idx, sourceURL := range imageURLs {
		sourceURL = strings.TrimSpace(sourceURL)
		if sourceURL == "" {
			return nil, fmt.Errorf("source image url %d is empty", idx+1)
		}
		resultURL, err := s.callAdobe2APITransparentEdit(ctx, client, endpoint, baseURL, apiKey, sourceURL, generationID, sizeLabel, idx)
		if err != nil {
			return nil, err
		}
		out = append(out, resultURL)
	}
	return out, nil
}

func (s *Server) callAdobe2APITransparentEdit(ctx context.Context, client *http.Client, endpoint, baseURL, apiKey, sourceURL, generationID, sizeLabel string, index int) (string, error) {
	payload := map[string]interface{}{
		"model":           adobe2APIClarityModel,
		"prompt":          "transparent background",
		"size":            strings.TrimSpace(sizeLabel),
		"n":               1,
		"response_format": "url",
		"images": []map[string]string{
			{"image_url": sourceURL},
		},
	}
	if strings.TrimSpace(sizeLabel) == "" {
		delete(payload, "size")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("adobe2api request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if readErr != nil {
		return "", fmt.Errorf("read adobe2api response failed: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("adobe2api HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed adobe2APIImageResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("adobe2api invalid JSON response: %w", err)
	}
	if len(parsed.Data) == 0 {
		return "", fmt.Errorf("adobe2api returned empty data")
	}
	item := parsed.Data[0]
	if rawURL := strings.TrimSpace(item.URL); rawURL != "" {
		return resolveAdobe2APIResultURL(baseURL, rawURL), nil
	}
	rawB64 := strings.TrimSpace(item.B64JSON)
	if rawB64 == "" {
		rawB64 = strings.TrimSpace(item.Base64)
	}
	if rawB64 == "" {
		return "", fmt.Errorf("adobe2api response has no url or b64_json")
	}
	imageBytes, err := decodeAdobe2APIBase64Image(rawB64)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(generationID)
	if len(parsed.Data) > 1 || index > 0 {
		name = fmt.Sprintf("%s-clarity-%d", name, index+1)
	} else {
		name = fmt.Sprintf("%s-clarity", name)
	}
	return s.saveGeneratedMediaToLocal(&generatedMediaPayload{
		Data:        imageBytes,
		ContentType: "image/png",
		FileName:    sanitizeGeneratedFilename(name) + ".png",
	})
}

func adobe2APIImageEditsEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("adobe2api_base_url is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid adobe2api_base_url")
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/v1/images/edits"):
	case strings.HasSuffix(path, "/images/edits"):
	case strings.HasSuffix(path, "/v1"):
		parsed.Path = path + "/images/edits"
	default:
		parsed.Path = path + "/v1/images/edits"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func resolveAdobe2APIResultURL(baseURL string, rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if parsed, err := url.Parse(rawURL); err == nil && parsed.IsAbs() {
		return rawURL
	}
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return rawURL
	}
	base.Path = "/"
	base.RawQuery = ""
	base.Fragment = ""
	ref, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return base.ResolveReference(ref).String()
}

func decodeAdobe2APIBase64Image(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if comma := strings.Index(raw, ","); strings.HasPrefix(strings.ToLower(raw[:minInt(comma+1, len(raw))]), "data:") && comma >= 0 {
		raw = raw[comma+1:]
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode adobe2api b64_json failed: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("adobe2api b64_json is empty")
	}
	return data, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
