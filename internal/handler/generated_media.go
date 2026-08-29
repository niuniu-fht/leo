package handler

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"
)

const (
	generatedMediaFetchTimeout = 5 * time.Minute
	maxGeneratedMediaBytes     = 500 << 20
	localFetchRetryAttempts    = 3
)

type generatedMediaPayload struct {
	Data        []byte
	ContentType string
	FileName    string
}

func (s *Server) materializeGeneratedMedia(sourceURL, generationID, mediaKind string) (string, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return "", fmt.Errorf("source url is required")
	}

	useUpstream := s != nil && s.Config != nil && s.Config.GetBool("use_upstream_result_url", false)
	if useUpstream {
		return sourceURL, nil
	}

	payload, err := s.fetchGeneratedMediaPayloadWithRetry(sourceURL, generationID, mediaKind)
	if err != nil {
		return "", err
	}

	localURL, err := s.saveGeneratedMediaToLocal(payload)
	if err != nil {
		return "", err
	}
	return localURL, nil
}

func (s *Server) fetchGeneratedMediaPayloadWithRetry(sourceURL, generationID, mediaKind string) (*generatedMediaPayload, error) {
	var lastErr error
	for attempt := 1; attempt <= localFetchRetryAttempts; attempt++ {
		payload, err := s.fetchGeneratedMediaPayload(sourceURL, generationID, mediaKind)
		if err == nil {
			return payload, nil
		}
		lastErr = err
		if attempt < localFetchRetryAttempts && isRetryableGeneratedMediaFetchError(err) {
			log.Printf("[generated] local fallback fetch attempt %d/%d failed for %s: %v; retrying", attempt, localFetchRetryAttempts, sourceURL, err)
			continue
		}
		return nil, err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("generated media fetch failed")
	}
	return nil, lastErr
}

func isRetryableGeneratedMediaFetchError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "broken pipe")
}

func (s *Server) fetchGeneratedMediaPayload(sourceURL, generationID, mediaKind string) (*generatedMediaPayload, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil {
		return nil, fmt.Errorf("invalid generated media url: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("generated media url must use http or https")
	}
	if parsedURL.RawPath == "" {
		parsedURL.RawPath = parsedURL.EscapedPath()
	}

	httpClient, err := s.newResourceHTTPClient(generatedMediaFetchTimeout)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "leo2api-generated-fetch/1.0")
	req.Header.Set("Accept", "*/*")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch generated media failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("generated media url returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGeneratedMediaBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read generated media failed: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("generated media url returned empty body")
	}
	if len(data) > maxGeneratedMediaBytes {
		return nil, fmt.Errorf("generated media exceeds %d MB limit", maxGeneratedMediaBytes>>20)
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])); mediaType != "" {
		contentType = mediaType
	}

	ext := ""
	switch mediaKind {
	case "image":
		ext = imageExtFromContentType(contentType)
		if ext == "" {
			ext = imageExtFromURL(parsedURL.Path)
		}
		if ext == "" {
			ext = "jpg"
		}
	default:
		ext = videoExtFromContentType(contentType)
		if ext == "" {
			ext = videoExtFromURL(parsedURL.Path)
		}
		if ext == "" {
			ext = "mp4"
		}
	}

	baseName := strings.TrimSpace(generationID)
	if baseName == "" {
		baseName = strings.TrimSuffix(pathpkg.Base(parsedURL.Path), pathpkg.Ext(parsedURL.Path))
	}
	baseName = sanitizeGeneratedFilename(baseName)
	if baseName == "" {
		baseName = fmt.Sprintf("generated-%d", time.Now().Unix())
	}

	return &generatedMediaPayload{
		Data:        data,
		ContentType: contentType,
		FileName:    fmt.Sprintf("%s.%s", baseName, ext),
	}, nil
}

func (s *Server) generatedMediaBytesForResponse(resultURL, generationID, mediaKind string) ([]byte, error) {
	resultURL = strings.TrimSpace(resultURL)
	if resultURL == "" {
		return nil, fmt.Errorf("generated media url is required")
	}
	if fileName, ok := s.generatedLocalFileNameFromURL(resultURL); ok {
		if s == nil || strings.TrimSpace(s.GeneratedDir) == "" {
			return nil, fmt.Errorf("generated dir is not configured")
		}
		data, err := os.ReadFile(filepath.Join(s.GeneratedDir, fileName))
		if err != nil {
			return nil, fmt.Errorf("read generated media file: %w", err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("generated media file is empty")
		}
		return data, nil
	}
	payload, err := s.fetchGeneratedMediaPayloadWithRetry(resultURL, generationID, mediaKind)
	if err != nil {
		return nil, err
	}
	return payload.Data, nil
}

func (s *Server) generatedLocalFileNameFromURL(rawURL string) (string, bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", false
	}
	pathValue := ""
	if strings.HasPrefix(rawURL, "/generated/") {
		pathValue = rawURL
	} else if parsed, err := url.Parse(rawURL); err == nil && parsed.Path != "" {
		if s != nil && s.Config != nil {
			baseURL := strings.TrimSpace(s.Config.GetString("public_base_url", ""))
			if baseURL != "" {
				if baseParsed, baseErr := url.Parse(baseURL); baseErr == nil && strings.EqualFold(parsed.Scheme, baseParsed.Scheme) && strings.EqualFold(parsed.Host, baseParsed.Host) {
					pathValue = parsed.Path
				}
			}
		}
		if pathValue == "" && strings.HasPrefix(parsed.Path, "/generated/") {
			pathValue = parsed.Path
		}
	}
	if !strings.HasPrefix(pathValue, "/generated/") {
		return "", false
	}
	fileName := strings.TrimPrefix(pathValue, "/generated/")
	fileName = strings.TrimLeft(strings.ReplaceAll(fileName, "\\", "/"), "/")
	if fileName == "" || strings.Contains(fileName, "../") || strings.Contains(fileName, "/..") || strings.Contains(fileName, "/") {
		return "", false
	}
	return fileName, true
}

func (s *Server) saveGeneratedMediaToLocal(payload *generatedMediaPayload) (string, error) {
	if payload == nil {
		return "", fmt.Errorf("generated media payload is required")
	}
	if s == nil || strings.TrimSpace(s.GeneratedDir) == "" {
		return "", fmt.Errorf("generated dir is not configured")
	}

	fileName := strings.TrimSpace(payload.FileName)
	if fileName == "" {
		return "", fmt.Errorf("generated media file name is required")
	}
	filePath := filepath.Join(s.GeneratedDir, fileName)

	s.generatedStorageMu.Lock()
	defer s.generatedStorageMu.Unlock()

	if err := os.MkdirAll(s.GeneratedDir, 0o755); err != nil {
		return "", fmt.Errorf("ensure generated dir: %w", err)
	}
	if err := os.WriteFile(filePath, payload.Data, 0o644); err != nil {
		return "", fmt.Errorf("save generated media: %w", err)
	}
	if _, pruneErr := s.enforceGeneratedStorageLimitLocked(); pruneErr != nil {
		log.Printf("[generated] failed to prune generated storage after saving %s: %v", fileName, pruneErr)
	}

	return s.buildGeneratedPublicURL(fileName), nil
}

func (s *Server) buildGeneratedPublicURL(fileName string) string {
	fileName = strings.TrimLeft(strings.ReplaceAll(fileName, "\\", "/"), "/")
	if s.Config != nil {
		baseURL := strings.TrimSpace(s.Config.GetString("public_base_url", ""))
		if baseURL != "" {
			return strings.TrimRight(baseURL, "/") + "/generated/" + fileName
		}
	}
	return "/generated/" + fileName
}

func sanitizeGeneratedFilename(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return ""
	}
	return out
}
