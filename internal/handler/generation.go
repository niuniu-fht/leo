package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"leo2api/internal/config"
	"leo2api/internal/provider/leonardo"
	"leo2api/internal/reqlog"
	"leo2api/internal/token"
)

type imageModelAlias struct {
	PublicID        string
	UpstreamModelID string
	RequestModel    string
	Quality         string
	Description     string
	Aliases         []string
}

var imageModelAliases = map[string]imageModelAlias{
	"gpt-image-2": {
		PublicID:        "gpt-image-2",
		UpstreamModelID: "135b2740-a20b-48c8-8f86-6f68199e06c5",
		RequestModel:    "gpt-image-2",
		Quality:         "low",
		Description:     "Leonardo GPT Image-2 image generation",
	},
	"gpt-image-2-high": {
		PublicID:        "gpt-image-2-high",
		UpstreamModelID: "135b2740-a20b-48c8-8f86-6f68199e06c5",
		RequestModel:    "gpt-image-2",
		Quality:         "medium",
		Description:     "Leonardo GPT Image-2 image generation",
	},
	"gpt-image-2-higher": {
		PublicID:        "gpt-image-2-higher",
		UpstreamModelID: "135b2740-a20b-48c8-8f86-6f68199e06c5",
		RequestModel:    "gpt-image-2",
		Quality:         "high",
		Description:     "Leonardo GPT Image-2 image generation",
	},
	"gpt-image-2-clarity": {
		PublicID:        "gpt-image-2-clarity",
		UpstreamModelID: "135b2740-a20b-48c8-8f86-6f68199e06c5",
		RequestModel:    "gpt-image-2",
		Quality:         "low",
		Description:     "Leonardo GPT Image-2 low quality generation + Adobe2API transparent background",
	},
	"banana2": {
		PublicID:        "banana2",
		UpstreamModelID: "7418e71f-4133-4e1b-9895-bee19f48f2ce",
		RequestModel:    "nano-banana-2",
		Quality:         "medium",
		Description:     "Leonardo Nano Banana 2 image generation",
		Aliases:         []string{"nano-banana-2"},
	},
	"nano-banana-2": {
		PublicID:        "banana2",
		UpstreamModelID: "7418e71f-4133-4e1b-9895-bee19f48f2ce",
		RequestModel:    "nano-banana-2",
		Quality:         "medium",
		Description:     "Leonardo Nano Banana 2 image generation",
		Aliases:         []string{"nano-banana-2"},
	},
	"bananapro": {
		PublicID:        "bananapro",
		UpstreamModelID: "7c02ef35-3a6b-4df6-b78d-873e5032c3b4",
		RequestModel:    "gemini-image-2",
		Quality:         "high",
		Description:     "Leonardo Nano Banana Pro image generation",
		Aliases:         []string{"banana-pro", "nano-banana-pro"},
	},
	"banana-pro": {
		PublicID:        "bananapro",
		UpstreamModelID: "7c02ef35-3a6b-4df6-b78d-873e5032c3b4",
		RequestModel:    "gemini-image-2",
		Quality:         "high",
		Description:     "Leonardo Nano Banana Pro image generation",
		Aliases:         []string{"banana-pro", "nano-banana-pro"},
	},
	"nano-banana-pro": {
		PublicID:        "bananapro",
		UpstreamModelID: "7c02ef35-3a6b-4df6-b78d-873e5032c3b4",
		RequestModel:    "gemini-image-2",
		Quality:         "high",
		Description:     "Leonardo Nano Banana Pro image generation",
		Aliases:         []string{"banana-pro", "nano-banana-pro"},
	},
	"gpt-image-gemini-3.1-flash-image": {
		PublicID:        "banana2",
		UpstreamModelID: "7418e71f-4133-4e1b-9895-bee19f48f2ce",
		RequestModel:    "nano-banana-2",
		Quality:         "medium",
		Description:     "Leonardo Nano Banana 2 image generation",
		Aliases:         []string{"gpt-image-gemini-3.1-flash-image", "nano-banana-2"},
	},
	"gpt-image-gemini-3-pro-image": {
		PublicID:        "bananapro",
		UpstreamModelID: "7c02ef35-3a6b-4df6-b78d-873e5032c3b4",
		RequestModel:    "gemini-image-2",
		Quality:         "high",
		Description:     "Leonardo Nano Banana Pro image generation",
		Aliases:         []string{"gpt-image-gemini-3-pro-image", "banana-pro", "nano-banana-pro"},
	},
}

func imageCatalogEntry(id, description, quality string, aliases []string) map[string]interface{} {
	entry := map[string]interface{}{
		"id":          id,
		"object":      "model",
		"owned_by":    "leonardo",
		"description": fmt.Sprintf("%s (server quality: %s)", description, quality),
		"parameters": map[string]interface{}{
			"type":    "image",
			"quality": quality,
			"size":    []string{"1536x1536", "2752x1536", "1536x2752", "2048x1536"},
		},
	}
	if len(aliases) > 0 {
		entry["aliases"] = aliases
	}
	return entry
}

var openAIModelCatalog = []map[string]interface{}{
	imageCatalogEntry("gpt-image-2", "Leonardo GPT Image-2 image generation", "low", nil),
	imageCatalogEntry("gpt-image-2-high", "Leonardo GPT Image-2 image generation", "medium", nil),
	imageCatalogEntry("gpt-image-2-higher", "Leonardo GPT Image-2 image generation", "high", nil),
	imageCatalogEntry("gpt-image-2-clarity", "Leonardo GPT Image-2 low quality generation + Adobe2API transparent background", "low", nil),
	imageCatalogEntry("gpt-image-gemini-3.1-flash-image", "Leonardo Nano Banana 2 image generation", "medium", []string{"banana2", "nano-banana-2"}),
	imageCatalogEntry("gpt-image-gemini-3-pro-image", "Leonardo Nano Banana Pro image generation", "high", []string{"bananapro", "banana-pro", "nano-banana-pro"}),
	{
		"id":          "video-2.0",
		"object":      "model",
		"owned_by":    "leonardo",
		"description": "Video 2.0 standard video generation",
		"aliases":     []string{"seedance-2.0"},
		"parameters": map[string]interface{}{
			"duration": []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			"size":     []string{"1280x720", "720x1280", "960x960"},
		},
	},
	{
		"id":          "video-2.0-fast",
		"object":      "model",
		"owned_by":    "leonardo",
		"description": "Video 2.0 fast video generation",
		"aliases":     []string{"seedance-2.0-fast"},
		"parameters": map[string]interface{}{
			"duration": []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			"size":     []string{"1280x720", "720x1280", "960x960"},
		},
	},
	{
		"id":          "video-2.0-mini",
		"object":      "model",
		"owned_by":    "leonardo",
		"description": "Video 2.0 mini video generation",
		"aliases":     []string{"seedance-2.0-mini"},
		"parameters": map[string]interface{}{
			"duration": []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			"size":     []string{"1280x720", "720x1280", "960x960"},
		},
	},
	{
		"id":          "video-2.0-480p",
		"object":      "model",
		"owned_by":    "leonardo",
		"description": "Video 2.0 480p video generation",
		"aliases":     []string{"seedance-2.0-480p"},
		"parameters": map[string]interface{}{
			"duration": []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			"size":     []string{"864x496", "496x864", "640x640"},
		},
	},
	{
		"id":          "video-2.0-fast-480p",
		"object":      "model",
		"owned_by":    "leonardo",
		"description": "Video 2.0 fast 480p video generation",
		"aliases":     []string{"seedance-2.0-fast-480p"},
		"parameters": map[string]interface{}{
			"duration": []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			"size":     []string{"864x496", "496x864", "640x640"},
		},
	},
	{
		"id":          "video-2.0-mini-480p",
		"object":      "model",
		"owned_by":    "leonardo",
		"description": "Video 2.0 mini 480p video generation",
		"aliases":     []string{"seedance-2.0-mini-480p"},
		"parameters": map[string]interface{}{
			"duration": []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			"size":     []string{"864x496", "496x864", "640x640"},
		},
	},
	{
		"id":          "sora2",
		"object":      "model",
		"owned_by":    "leonardo",
		"description": "Sora 2 video generation",
		"parameters": map[string]interface{}{
			"duration": []int{4, 8, 12},
			"size":     []string{"1280x720", "720x1280"},
		},
	},
	{
		"id":          "ko3",
		"object":      "model",
		"owned_by":    "leonardo",
		"description": "Kling O3 video generation",
		"aliases":     []string{"kling-o3", "kling-video-o-3"},
		"parameters": map[string]interface{}{
			"duration": []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			"size":     []string{"1440x1440", "1080x1920", "1920x1080", "0x0"},
		},
	},
	{
		"id":          "minimax-h3",
		"object":      "model",
		"owned_by":    "leonardo",
		"description": "MiniMax Hailuo 03 video generation",
		"parameters": map[string]interface{}{
			"duration": []int{5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			"size":     []string{"2560x1440", "1440x2560", "1440x1440", "1920x1440", "1440x1920", "3360x1440"},
		},
	},
}

const (
	defaultSeedanceVideoDuration    = 10
	defaultSora2VideoDuration       = 8
	defaultKlingO3VideoDuration     = 3
	defaultKlingO3VideoRefDuration  = 5
	defaultMinimaxH3VideoDuration   = 5
	tokenPreparationLeaseTTL        = 5 * time.Minute
	generationJWTPreferredRemaining = 10 * time.Minute
	generationJWTMinimumRemaining   = 5 * time.Minute
	sora2RequiredCredits            = 1200
	video2RequiredCredits           = 4550
	video2FastRequiredCredits       = 3650
	video2MiniRequiredCredits       = 2400
	video2Required480pCredits       = 2150
	video2FastRequired480pCredits   = 1700
	video2MiniRequired480pCredits   = 1200
	klingO3RequiredCredits          = 4200
	klingO3VideoRefRequiredCredits  = 3400
	minimaxH3RequiredCredits        = 2100
	defaultTokenMaxRunningTasks     = 2
	videoKo3ExhaustionCredits       = video2MiniRequired480pCredits
	failedGenerationCreditsDelay    = 5 * time.Second
)

// Server holds all dependencies for HTTP handlers.
type Server struct {
	TokenMgr                *token.Manager
	Config                  *config.Manager
	GeneratedDir            string
	LeonardoClient          *leonardo.Client
	ReqLog                  *reqlog.Store
	generatedStorageMu      sync.Mutex
	cookieImportMu          sync.Mutex
	tokenSelectionMu        sync.Mutex
	cookieImportJobs        map[string]*cookieImportJob
	tokenRefreshJobMu       sync.Mutex
	tokenRefreshJobs        map[string]*tokenRefreshBatchJob
	leoSessionMu            sync.Mutex
	leoSessions             map[string]*leonardo.TokenSession
	autoRefreshMu           sync.Mutex
	autoRefreshRun          map[string]time.Time
	autoRefreshBusy         map[string]bool
	autoRefreshLoopStarted  bool
	autoRefreshSweepRunning bool
	tokenCleanupMu          sync.Mutex
	tokenCleanupLoopStarted bool
	tokenCleanupRunning     bool
	tokenLifecycleMu        sync.Mutex
	tokenPrepLeaseMu        sync.Mutex
	tokenPrepLeases         map[string]time.Time
	tokenSettlementMu       sync.Mutex
	tokenSettlements        map[string]int
}

type generationRetryPolicy struct {
	Enabled                bool
	MaxAttempts            int
	BackoffBase            time.Duration
	StatusCodes            map[int]struct{}
	ErrorMatchers          []string
	SameTokenErrorMatchers []string
}

type generationRetryPhase string

const (
	generationRetryPhaseSubmit    generationRetryPhase = "submit"
	generationRetryPhaseAsyncTask generationRetryPhase = "async_task"
)

type generationRetryAction string

const (
	generationRetryActionNone      generationRetryAction = "none"
	generationRetryActionNextToken generationRetryAction = "next_token"
	generationRetryActionSameToken generationRetryAction = "same_token"
)

type videoGenerationAttemptFailure struct {
	StatusCode      int
	Message         string
	ErrorType       string
	RetryCodeSource string
	MarkInvalid     bool
	Insufficient    bool
}

type videoGenerationSubmission struct {
	GenerationID string
	CreatedAt    time.Time
	Request      *leonardo.GenerateRequest
}

type asyncVideoGenerationContext struct {
	Session              *leonardo.TokenSession
	TokenID              string
	ModelID              string
	PublicGenerationID   string
	UpstreamGenerationID string
	Request              *leonardo.GenerateRequest
	Attempt              int
	StartedAt            time.Time
}

func (s *Server) expireStaleRunningLogs() int {
	if s == nil || s.ReqLog == nil {
		return 0
	}

	timeoutSec := 600
	if s.Config != nil {
		timeoutSec = s.Config.GetInt("generate_timeout", 600)
	}
	if timeoutSec < 1 {
		timeoutSec = 600
	}

	return s.ReqLog.ExpireStaleRunning(time.Duration(timeoutSec)*time.Second, time.Now())
}

// requireAPIKey validates the X-API-Key or Authorization header.
func (s *Server) requireAPIKey(r *http.Request) error {
	expected := s.Config.GetString("api_key")
	if expected == "" {
		return nil
	}
	key := r.Header.Get("X-API-Key")
	if key == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			key = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	if strings.TrimSpace(key) != expected {
		return fmt.Errorf("invalid api key")
	}
	return nil
}

// HandleListModels handles GET /v1/models.
func (s *Server) HandleListModels(w http.ResponseWriter, r *http.Request) {
	if err := s.requireAPIKey(r); err != nil {
		writeJSON(w, 401, map[string]interface{}{"error": map[string]string{"message": err.Error(), "type": "authentication_error"}})
		return
	}
	writeJSON(w, 200, map[string]interface{}{"object": "list", "data": openAIModelCatalog})
}

// HandleImageGeneration handles POST /v1/images/generations.
func (s *Server) HandleImageGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.requireAPIKey(r); err != nil {
		writeJSON(w, 401, errorResp("invalid api key", "authentication_error"))
		return
	}
	if s.LeonardoClient == nil {
		writeJSON(w, 500, errorResp("Leonardo client not initialized", "server_error"))
		return
	}

	var payload openAIImageGenerationRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, 400, errorResp("invalid request body", "invalid_request_error"))
		return
	}
	s.handleOpenAIImageRequest(w, r, payload)
}

// HandleImageEdits handles POST /v1/images/edits.
func (s *Server) HandleImageEdits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.requireAPIKey(r); err != nil {
		writeJSON(w, 401, errorResp("invalid api key", "authentication_error"))
		return
	}
	if s.LeonardoClient == nil {
		writeJSON(w, 500, errorResp("Leonardo client not initialized", "server_error"))
		return
	}

	payload, err := parseOpenAIImageEditRequest(r)
	if err != nil {
		writeJSON(w, 400, errorResp(err.Error(), "invalid_request_error"))
		return
	}
	if len(payload.referenceURLs()) == 0 {
		writeJSON(w, 400, errorResp("image is required", "invalid_request_error"))
		return
	}
	s.handleOpenAIImageRequest(w, r, payload)
}

func (s *Server) handleOpenAIImageRequest(w http.ResponseWriter, r *http.Request, payload openAIImageGenerationRequest) {
	payload.Prompt = strings.TrimSpace(payload.Prompt)
	if len(payload.Prompt) < 3 {
		writeJSON(w, 400, errorResp("prompt must contain at least 3 characters", "invalid_request_error"))
		return
	}
	if payload.N == 0 {
		payload.N = 1
	}
	if payload.N < 1 || payload.N > 4 {
		writeJSON(w, 400, errorResp("n must be between 1 and 4", "invalid_request_error"))
		return
	}

	publicModelID, upstreamModelID, requestModel, quality := s.resolveImageGenerationModel(payload.Model)
	sizeInfo, err := s.resolveImageRequestSizeDetails(publicModelID, payload.Size, payload.AspectRatio)
	if err != nil {
		writeJSON(w, 400, errorResp(err.Error(), "invalid_request_error"))
		return
	}
	width, height, sizeLabel := sizeInfo.Width, sizeInfo.Height, sizeInfo.Label

	session, usedTokenID, releaseTokenPreparation := s.getLeonardoSessionForModelExcludingWithPreparationLease("", nil, publicModelID, false)
	if session == nil {
		writeJSON(w, 503, errorResp("No tokens available", "server_error"))
		return
	}
	releaseTokenPreparationNow := func() {
		if releaseTokenPreparation == nil {
			return
		}
		releaseTokenPreparation()
		releaseTokenPreparation = nil
	}
	defer releaseTokenPreparationNow()

	startTime := time.Now()
	uploadCache := make(map[string]string)
	refURLs := payload.referenceURLs()
	imageReferenceCount := len(refURLs)
	imageInputMode := "text_to_image"
	if imageReferenceCount > 0 {
		imageInputMode = "image_to_image"
	}
	imageOperation := "images.generations"
	if strings.Contains(strings.ToLower(r.URL.Path), "/v1/images/edits") {
		imageOperation = "images.edits"
	}
	initImageIDs := make([]string, 0, len(refURLs))
	for idx, refURL := range refURLs {
		imageID, err := s.resolveLeonardoImageID(session, "", refURL, uploadCache)
		if err != nil {
			msg := fmt.Sprintf("invalid image_urls[%d]: %v", idx, err)
			if payload.ImageURL != "" && idx == 0 {
				msg = fmt.Sprintf("invalid image_url: %v", err)
			}
			s.logImageRequestFailure(payload.Prompt, publicModelID, quality, sizeLabel, sizeInfo.Transform, imageInputMode, imageOperation, imageReferenceCount, usedTokenID, session, time.Since(startTime).Seconds(), 400, msg)
			writeJSON(w, 400, errorResp(msg, "invalid_request_error"))
			return
		}
		initImageIDs = append(initImageIDs, imageID)
	}

	nativeImageRequest := imageUsesNativeRequest(publicModelID)
	promptEnhance := ""
	styleIDs := []string(nil)
	requestQuality := quality
	if nativeImageRequest {
		promptEnhance = "OFF"
		styleIDs = []string{"556c1ee5-ec38-42e8-955a-1e82dad0ffa1"}
	}
	if imageUsesGeminiImage2Request(publicModelID, requestModel) {
		nativeImageRequest = true
		promptEnhance = "AUTO"
		styleIDs = []string{"111dc692-d470-4eec-b791-3475abac4c46"}
		requestQuality = ""
	}

	result, err := s.LeonardoClient.GenerateImage(session, &leonardo.ImageGenerateRequest{
		RequestModel:       requestModel,
		ModelID:            upstreamModelID,
		Prompt:             payload.Prompt,
		Quality:            requestQuality,
		Width:              width,
		Height:             height,
		Quantity:           payload.N,
		InitImageIDs:       initImageIDs,
		Public:             true,
		NativeModelRequest: nativeImageRequest,
		PromptEnhance:      promptEnhance,
		StyleIDs:           styleIDs,
	})
	if err != nil {
		statusCode := statusCodeFromGenerationError(err)
		if isInsufficientTokensMessage(err.Error()) {
			s.markTokenExhausted(usedTokenID, err.Error())
		} else if !isRetryableGenerationError(err) && s.TokenMgr != nil && usedTokenID != "" {
			s.TokenMgr.ReportFail(usedTokenID)
		}
		msg := fmt.Sprintf("image generation failed: %v", err)
		s.logImageRequestFailure(payload.Prompt, publicModelID, quality, sizeLabel, sizeInfo.Transform, imageInputMode, imageOperation, imageReferenceCount, usedTokenID, session, time.Since(startTime).Seconds(), statusCode, msg)
		writeJSON(w, statusCode, errorResp(msg, "server_error"))
		return
	}
	s.applyTokenCreditCost(usedTokenID, result.APICreditCost)
	accountName, accountEmail := s.resolveReqLogAccount(usedTokenID, session)
	if s.ReqLog != nil {
		s.ReqLog.Add(reqlog.Entry{
			ID:                   fmt.Sprintf("img-%d", time.Now().UnixNano()),
			Timestamp:            float64(startTime.Unix()),
			StatusCode:           200,
			TaskStatus:           "IN_PROGRESS",
			Type:                 "image",
			InputMode:            imageInputMode,
			ReferenceCount:       imageReferenceCount,
			TokenID:              usedTokenID,
			TokenAttempt:         1,
			AccountName:          accountName,
			AccountEmail:         accountEmail,
			Model:                publicModelID,
			ModelParams:          fmt.Sprintf("%s quality=%s n=%d", sizeLabel, quality, payload.N),
			SizeTransform:        sizeInfo.Transform,
			Prompt:               payload.Prompt,
			GenerationID:         result.GenerationID,
			UpstreamGenerationID: result.GenerationID,
			CreditCost:           result.APICreditCost,
			Operation:            imageOperation,
		})
	}
	// After Leonardo accepts the image generation and the IN_PROGRESS log is
	// recorded, release the short preparation lock. From here on, per-token
	// concurrency is controlled by token_max_running_tasks via request logs.
	releaseTokenPreparationNow()

	timeout := 300 * time.Second
	if s.Config != nil {
		timeout = time.Duration(s.Config.GetInt("generate_timeout", 300)) * time.Second
	}
	imageURLs, err := s.waitForLeonardoImageGeneration(session, result.GenerationID, timeout, 4*time.Second)
	elapsedSec := time.Since(startTime).Seconds()
	if err != nil {
		failureMessage := err.Error()
		if strings.Contains(strings.ToLower(failureMessage), "failed") {
			if reason := s.waitForGenerationFailureReason(session, result.GenerationID, "image_generation"); reason != "" {
				failureMessage = reason
			}
			s.scheduleFailedGenerationCreditsReconciliation(usedTokenID, session, result.GenerationID)
		}
		if s.ReqLog != nil {
			s.ReqLog.UpdateByGenerationID(result.GenerationID, "FAILED", 502, "", "", failureMessage)
			s.ReqLog.UpdateDuration(result.GenerationID, elapsedSec)
		}
		s.refreshTokenCredits(usedTokenID, session)
		writeJSON(w, 502, errorResp(failureMessage, "server_error"))
		return
	}

	postprocess := ""
	finalURLs := make([]string, 0, len(imageURLs))
	if imageUsesAdobeClarity(publicModelID) {
		postprocess = "adobe2api.transparent"
		finalURLs, err = s.convertGeneratedImagesToTransparentWithAdobe(r.Context(), imageURLs, result.GenerationID, sizeLabel)
		if err != nil {
			msg := fmt.Sprintf("transparent background failed: %v", err)
			if s.ReqLog != nil {
				s.ReqLog.UpdateByGenerationID(result.GenerationID, "FAILED", 502, "", "", msg)
				s.ReqLog.UpdateDuration(result.GenerationID, elapsedSec)
			}
			s.refreshTokenCredits(usedTokenID, session)
			writeJSON(w, 502, errorResp(msg, "server_error"))
			return
		}
	} else {
		for idx, rawURL := range imageURLs {
			generationName := result.GenerationID
			if len(imageURLs) > 1 {
				generationName = fmt.Sprintf("%s-%d", result.GenerationID, idx+1)
			}
			finalURL, materializeErr := s.materializeGeneratedMedia(rawURL, generationName, "image")
			if materializeErr != nil {
				if s.ReqLog != nil {
					s.ReqLog.UpdateByGenerationID(result.GenerationID, "FAILED", 502, "", "", fmt.Sprintf("save generated image failed: %v", materializeErr))
					s.ReqLog.UpdateDuration(result.GenerationID, elapsedSec)
				}
				s.refreshTokenCredits(usedTokenID, session)
				writeJSON(w, 502, errorResp(fmt.Sprintf("save generated image failed: %v", materializeErr), "server_error"))
				return
			}
			finalURLs = append(finalURLs, finalURL)
		}
	}
	previewURL := ""
	if len(finalURLs) > 0 {
		previewURL = finalURLs[0]
	}
	if s.ReqLog != nil {
		s.ReqLog.UpdateByGenerationID(result.GenerationID, "COMPLETE", 200, previewURL, "image", "")
		s.ReqLog.UpdateDuration(result.GenerationID, elapsedSec)
	}
	s.refreshTokenCredits(usedTokenID, session)

	data := make([]map[string]string, 0, len(finalURLs))
	for _, imageURL := range finalURLs {
		data = append(data, map[string]string{"url": imageURL})
	}
	writeJSON(w, 200, map[string]interface{}{
		"created": time.Now().Unix(),
		"data":    data,
		"provider": map[string]interface{}{
			"generation_id":     result.GenerationID,
			"used_token_id":     usedTokenID,
			"model":             publicModelID,
			"upstream_model_id": upstreamModelID,
			"request_model":     requestModel,
			"quality":           quality,
			"size":              sizeLabel,
			"credit_cost":       result.APICreditCost,
			"postprocess":       postprocess,
		},
	})
}

// HandleChatCompletions handles POST /v1/chat/completions.
func (s *Server) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if err := s.requireAPIKey(r); err != nil {
		writeJSON(w, 401, errorResp("invalid api key", "authentication_error"))
		return
	}
	writeJSON(w, 400, errorResp("chat completions are not supported by this deployment; use /v1/video/generations with a supported video model", "invalid_request_error"))
}

// HandleVideoGeneration handles POST /v1/video/generations.
func (s *Server) HandleVideoGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.requireAPIKey(r); err != nil {
		writeJSON(w, 401, errorResp("invalid api key", "authentication_error"))
		return
	}
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeJSON(w, 400, errorResp("invalid request body", "invalid_request_error"))
		return
	}

	prompt := strings.TrimSpace(fmt.Sprintf("%v", data["prompt"]))
	if prompt == "" || prompt == "<nil>" {
		prompt = extractPromptFromMessages(data)
	}
	if prompt == "<nil>" {
		prompt = ""
	}
	if len(prompt) < 3 {
		writeJSON(w, 400, errorResp("prompt must contain at least 3 characters", "invalid_request_error"))
		return
	}

	requestedModelID, _ := data["model"].(string)
	if strings.TrimSpace(requestedModelID) == "" {
		requestedModelID = "video-2.0-fast"
	}
	modelID, ok := normalizeVideoModelID(requestedModelID)
	if !ok {
		writeJSON(w, 400, errorResp("unsupported model; available models are video-2.0, video-2.0-fast, video-2.0-mini, their 480p variants, sora2, ko3, and minimax-h3", "invalid_request_error"))
		return
	}
	responseModelID := publicVideoModelID(modelID)
	if hasAudioReferenceInput(data) && !isSeedanceModelID(modelID) && !isMinimaxH3ModelID(modelID) {
		writeJSON(w, 400, errorResp("audio_reference is only supported for video-2.0 Seedance models and minimax-h3 image-reference requests", "invalid_request_error"))
		return
	}
	if isMinimaxH3ModelID(modelID) {
		if err := validateMinimaxH3GuidanceInput(data); err != nil {
			writeJSON(w, 400, errorResp(err.Error(), "invalid_request_error"))
			return
		}
	}
	duration := defaultVideoDuration(modelID)
	klingO3VideoRefMode := isKlingO3ModelID(modelID) && hasVideoReferenceInput(data)
	if klingO3VideoRefMode {
		duration = defaultKlingO3VideoRefDuration
	}
	if d, ok := data["duration"].(float64); ok {
		duration = int(d)
	}
	if isSora2ModelID(modelID) && !isAllowedSora2Duration(duration) {
		writeJSON(w, 400, errorResp("sora2 duration must be 4, 8, or 12 seconds", "invalid_request_error"))
		return
	}
	if isKlingO3ModelID(modelID) && !isAllowedKlingO3Duration(duration, klingO3VideoRefMode) {
		writeJSON(w, 400, errorResp(klingO3DurationError(klingO3VideoRefMode), "invalid_request_error"))
		return
	}
	if isMinimaxH3ModelID(modelID) && !isAllowedMinimaxH3Duration(duration) {
		writeJSON(w, 400, errorResp("minimax-h3 duration must be between 5 and 15 seconds", "invalid_request_error"))
		return
	}
	if duration < 4 || duration > 15 {
		if !isKlingO3ModelID(modelID) {
			writeJSON(w, 400, errorResp("duration must be between 4 and 15 seconds", "invalid_request_error"))
			return
		}
	}

	// Parse size (e.g. "1280x720")
	width, height := defaultVideoSize(modelID)
	if klingO3VideoRefMode {
		width, height = 0, 0
	}
	if aspectRatio := strings.TrimSpace(toString(data["aspect_ratio"])); aspectRatio != "" {
		aspectWidth, aspectHeight, ok := videoSizeForAspectRatio(modelID, aspectRatio)
		if !ok {
			writeJSON(w, 400, errorResp("unsupported aspect_ratio for model", "invalid_request_error"))
			return
		}
		width, height = aspectWidth, aspectHeight
	}
	if size, ok := data["size"].(string); ok && size != "" {
		parts := strings.Split(size, "x")
		if len(parts) == 2 {
			if w, err := strconv.Atoi(parts[0]); err == nil {
				width = w
			}
			if h, err := strconv.Atoi(parts[1]); err == nil {
				height = h
			}
		}
	}
	if w, ok := data["width"].(float64); ok {
		width = int(w)
	}
	if h, ok := data["height"].(float64); ok {
		height = int(h)
	}
	if isKlingO3ModelID(modelID) && hasUnsupportedKlingO3GuidanceInput(data) {
		writeJSON(w, 400, errorResp("ko3 currently supports text-to-video, image-reference image-to-video, and start/end-frame requests only", "invalid_request_error"))
		return
	}
	if isKlingO3ModelID(modelID) && !isAllowedKlingO3Size(width, height, klingO3VideoRefMode) {
		writeJSON(w, 400, errorResp(klingO3SizeError(klingO3VideoRefMode), "invalid_request_error"))
		return
	}
	if isSora2ModelID(modelID) && hasUnsupportedSora2GuidanceInput(data) {
		writeJSON(w, 400, errorResp("sora2 currently supports text-to-video and start-frame image-to-video requests only", "invalid_request_error"))
		return
	}
	if isSora2ModelID(modelID) && !isAllowedSora2Size(width, height) {
		writeJSON(w, 400, errorResp("sora2 size must be 720x1280 or 1280x720", "invalid_request_error"))
		return
	}
	if isSeedance480pModelID(modelID) && !isAllowedSeedance480pSize(width, height) {
		writeJSON(w, 400, errorResp("seedance 480p size must be 496x864, 864x496, or 640x640", "invalid_request_error"))
		return
	}
	if isMinimaxH3ModelID(modelID) && !isAllowedMinimaxH3Size(width, height) {
		writeJSON(w, 400, errorResp("minimax-h3 size must be one of 2560x1440, 1440x2560, 1440x1440, 1920x1440, 1440x1920, or 3360x1440", "invalid_request_error"))
		return
	}
	if isSora2ModelID(modelID) && countSora2StartFrameInputs(data) > 1 {
		writeJSON(w, 400, errorResp("sora2 supports at most one uploaded image", "invalid_request_error"))
		return
	}

	retryPolicy := s.loadGenerationRetryPolicy()
	triedTokenIDs := make(map[string]bool)
	var lastFailure *videoGenerationAttemptFailure
	var lastTokenID string
	var lastSession *leonardo.TokenSession
	var lastAttempt int

	maxAttempts := retryPolicy.MaxAttempts
	if s != nil && s.TokenMgr != nil {
		if tokenCount := s.TokenMgr.Count(); tokenCount > maxAttempts {
			maxAttempts = tokenCount
		}
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		session, usedTokenID, releaseTokenPreparation := s.getLeonardoSessionForModelExcludingWithPreparationLease("", triedTokenIDs, modelID, klingO3VideoRefMode)
		if session == nil {
			if lastFailure != nil {
				break
			}
			s.logVideoRequestFailure("openai.video.generate", prompt, modelID, duration, width, height, usedTokenID, session, attempt, 503, "No tokens available")
			writeJSON(w, 503, errorResp("No tokens available", "server_error"))
			return
		}

		imageRefs, startFrames, endFrames, videoRefs, audioRefs, err := s.resolveOpenAIVideoGuidanceInputs(data, session, modelID)
		if err != nil {
			if isRetryableGuidancePreparationError(err) {
				failure := &videoGenerationAttemptFailure{
					StatusCode:      http.StatusBadRequest,
					Message:         err.Error(),
					ErrorType:       "invalid_request_error",
					RetryCodeSource: extractRetryCodeSource(err.Error()),
				}
				lastFailure = failure
				lastTokenID = usedTokenID
				lastSession = session
				lastAttempt = attempt

				if attempt < retryPolicy.MaxAttempts {
					triedTokenIDs[usedTokenID] = true
					if s.TokenMgr != nil && usedTokenID != "" {
						s.TokenMgr.ReportFail(usedTokenID)
					}
					releaseTokenPreparation()
					log.Printf("[generation] guidance upload failed for token %s on attempt %d/%d; switching token: %v", usedTokenID, attempt, retryPolicy.MaxAttempts, err)
					if delay := retryPolicy.backoffDelay(attempt); delay > 0 {
						time.Sleep(delay)
					}
					continue
				}
				if s.TokenMgr != nil && usedTokenID != "" {
					s.TokenMgr.ReportFail(usedTokenID)
				}
				releaseTokenPreparation()
				s.logVideoRequestFailure("openai.video.generate", prompt, modelID, duration, width, height, usedTokenID, session, attempt, failure.StatusCode, failure.Message)
				writeJSON(w, failure.StatusCode, errorResp(failure.Message, failure.ErrorType))
				return
			}
			releaseTokenPreparation()
			s.logVideoRequestFailure("openai.video.generate", prompt, modelID, duration, width, height, usedTokenID, session, attempt, 400, err.Error())
			writeJSON(w, 400, errorResp(err.Error(), "invalid_request_error"))
			return
		}

		submission, failure := s.submitLeonardoVideoGeneration(session, usedTokenID, attempt, prompt, modelID, duration, width, height, imageRefs, startFrames, endFrames, videoRefs, audioRefs)
		if failure == nil {
			releaseTokenPreparation()
			attemptTimeout := 10 * time.Minute
			if s.Config != nil {
				attemptTimeout = time.Duration(s.Config.GetInt("generate_timeout", 600)) * time.Second
			}
			go s.trackLeonardoVideoGeneration(&asyncVideoGenerationContext{
				Session:              session,
				TokenID:              usedTokenID,
				ModelID:              modelID,
				PublicGenerationID:   submission.GenerationID,
				UpstreamGenerationID: submission.GenerationID,
				Request:              submission.Request,
				Attempt:              attempt,
				StartedAt:            submission.CreatedAt,
			}, 5*time.Second, attemptTimeout)
			writeJSON(w, http.StatusAccepted, map[string]interface{}{
				"id":         submission.GenerationID,
				"object":     "video.generation",
				"created":    submission.CreatedAt.Unix(),
				"model":      responseModelID,
				"status":     "in_progress",
				"poll_url":   videoGenerationPollURL(r.URL.Path, submission.GenerationID),
				"request_id": submission.GenerationID,
			})
			return
		}

		lastFailure = failure
		lastTokenID = usedTokenID
		lastSession = session
		lastAttempt = attempt

		if failure.Insufficient || (retryPolicy.retryAction(failure, generationRetryPhaseSubmit) == generationRetryActionNextToken && attempt < retryPolicy.MaxAttempts) {
			triedTokenIDs[usedTokenID] = true
			if s.TokenMgr != nil && usedTokenID != "" {
				if failure.Insufficient {
					s.refreshTokenCredits(usedTokenID, session)
				} else {
					s.TokenMgr.ReportFail(usedTokenID)
				}
			}
			releaseTokenPreparation()
			delay := retryPolicy.backoffDelay(attempt)
			if !failure.Insufficient && delay > 0 {
				time.Sleep(delay)
			}
			continue
		}

		if s.TokenMgr != nil && usedTokenID != "" {
			if failure.MarkInvalid {
				s.TokenMgr.ReportInvalid(usedTokenID)
			} else if failure.Insufficient {
				s.refreshTokenCredits(usedTokenID, session)
			} else if retryPolicy.retryAction(failure, generationRetryPhaseSubmit) == generationRetryActionNextToken {
				s.TokenMgr.ReportFail(usedTokenID)
			}
		}
		releaseTokenPreparation()
		s.logVideoRequestFailure("openai.video.generate", prompt, modelID, duration, width, height, usedTokenID, session, attempt, failure.StatusCode, failure.Message)
		writeJSON(w, failure.StatusCode, errorResp(failure.Message, failure.ErrorType))
		return
	}

	if lastFailure != nil {
		if s.TokenMgr != nil && lastTokenID != "" {
			if lastFailure.MarkInvalid {
				s.TokenMgr.ReportInvalid(lastTokenID)
			} else if lastFailure.Insufficient {
				s.refreshTokenCredits(lastTokenID, lastSession)
			} else if retryPolicy.retryAction(lastFailure, generationRetryPhaseSubmit) == generationRetryActionNextToken {
				s.TokenMgr.ReportFail(lastTokenID)
			}
		}
		s.logVideoRequestFailure("openai.video.generate", prompt, modelID, duration, width, height, lastTokenID, lastSession, lastAttempt, lastFailure.StatusCode, lastFailure.Message)
		writeJSON(w, lastFailure.StatusCode, errorResp(lastFailure.Message, lastFailure.ErrorType))
		return
	}

	writeJSON(w, 503, errorResp("No tokens available", "server_error"))
}

// HandleVideoGenerationStatus handles GET /v1/video/generations/{id}.
func (s *Server) HandleVideoGenerationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.requireAPIKey(r); err != nil {
		writeJSON(w, 401, errorResp("invalid api key", "authentication_error"))
		return
	}
	generationID := videoGenerationIDFromPath(r.URL.Path)
	if generationID == "" {
		writeJSON(w, 400, errorResp("generation id is required", "invalid_request_error"))
		return
	}
	if s.ReqLog == nil {
		writeJSON(w, 404, errorResp("generation not found", "not_found_error"))
		return
	}

	entry, ok := s.ReqLog.FindByGenerationID(generationID)
	if !ok {
		writeJSON(w, 404, errorResp("generation not found", "not_found_error"))
		return
	}

	modelID := publicRequestLogModel(entry.Model)
	status := "in_progress"
	response := map[string]interface{}{
		"id":         generationID,
		"object":     "video.generation",
		"created":    int64(entry.Timestamp),
		"model":      modelID,
		"status":     status,
		"request_id": generationID,
	}

	switch strings.ToUpper(strings.TrimSpace(entry.TaskStatus)) {
	case "COMPLETE":
		status = "succeeded"
		response["status"] = status
		if entry.PreviewURL != "" {
			response["data"] = []map[string]interface{}{{"url": entry.PreviewURL}}
		} else {
			response["data"] = []map[string]interface{}{}
		}
	case "FAILED":
		status = "failed"
		response["status"] = status
		response["error"] = map[string]interface{}{
			"message": strings.TrimSpace(entry.ErrorMessage),
			"type":    "server_error",
		}
	default:
		response["status"] = status
	}

	returnedCode := http.StatusOK
	if status == "in_progress" {
		returnedCode = http.StatusAccepted
	}
	writeJSON(w, returnedCode, response)
}

// ---- Helpers ----

func extractPromptFromMessages(data map[string]interface{}) string {
	messages, _ := data["messages"].([]interface{})
	for _, msg := range messages {
		m, _ := msg.(map[string]interface{})
		content := m["content"]
		switch c := content.(type) {
		case string:
			if strings.TrimSpace(c) != "" {
				return strings.TrimSpace(c)
			}
		case []interface{}:
			for _, part := range c {
				p, _ := part.(map[string]interface{})
				if p["type"] == "text" {
					if text, ok := p["text"].(string); ok && strings.TrimSpace(text) != "" {
						return strings.TrimSpace(text)
					}
				}
			}
		}
	}
	return ""
}

func errorResp(message, errType string) map[string]interface{} {
	return map[string]interface{}{
		"error": map[string]interface{}{"message": message, "type": errType},
	}
}

type openAIImageReferenceInput struct {
	ImageURL    string `json:"image_url"`
	URL         string `json:"url"`
	ImageBase64 string `json:"image_base64"`
	Base64      string `json:"base64"`
	B64JSON     string `json:"b64_json"`
}

type openAIImageGenerationRequest struct {
	Prompt       string                      `json:"prompt"`
	Model        string                      `json:"model"`
	N            int                         `json:"n"`
	Size         string                      `json:"size"`
	AspectRatio  string                      `json:"aspect_ratio"`
	ImageURL     string                      `json:"image_url"`
	ImageURLs    []string                    `json:"image_urls"`
	ImageBase64  string                      `json:"image_base64"`
	ImageBase64s []string                    `json:"image_base64s"`
	Images       []openAIImageReferenceInput `json:"images"`
}

func parseOpenAIImageEditRequest(r *http.Request) (openAIImageGenerationRequest, error) {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "multipart/form-data") {
		var payload openAIImageGenerationRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return payload, fmt.Errorf("invalid request body")
		}
		return payload, nil
	}

	if err := r.ParseMultipartForm((maxRemoteImageBytes * 4) + (4 << 20)); err != nil {
		return openAIImageGenerationRequest{}, fmt.Errorf("invalid multipart form: %w", err)
	}

	payload := openAIImageGenerationRequest{
		Prompt:      firstFormValue(r, "prompt"),
		Model:       firstFormValue(r, "model"),
		Size:        firstFormValue(r, "size"),
		AspectRatio: firstFormValue(r, "aspect_ratio"),
	}
	if rawN := strings.TrimSpace(firstFormValue(r, "n")); rawN != "" {
		n, err := strconv.Atoi(rawN)
		if err != nil {
			return payload, fmt.Errorf("invalid n")
		}
		payload.N = n
	}

	for _, field := range []string{"image_url", "url"} {
		for _, raw := range formValues(r, field) {
			payload.ImageURLs = append(payload.ImageURLs, raw)
		}
	}
	for _, field := range []string{"image_urls", "image_url[]", "images[]"} {
		for _, raw := range formValues(r, field) {
			payload.ImageURLs = append(payload.ImageURLs, raw)
		}
	}
	for _, field := range []string{"image_base64", "base64", "b64_json", "image"} {
		for _, raw := range formValues(r, field) {
			payload.ImageBase64s = append(payload.ImageBase64s, raw)
		}
	}
	for _, field := range []string{"image_base64s", "base64s"} {
		for _, raw := range formValues(r, field) {
			payload.ImageBase64s = append(payload.ImageBase64s, raw)
		}
	}

	if r.MultipartForm != nil {
		for _, field := range []string{"image", "image[]", "images"} {
			for _, fh := range r.MultipartForm.File[field] {
				dataURL, err := multipartImageFileToDataURL(fh)
				if err != nil {
					return payload, err
				}
				payload.ImageBase64s = append(payload.ImageBase64s, dataURL)
			}
		}
	}
	return payload, nil
}

func firstFormValue(r *http.Request, key string) string {
	values := formValues(r, key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func formValues(r *http.Request, key string) []string {
	if r.MultipartForm == nil || r.MultipartForm.Value == nil {
		return nil
	}
	rawValues := r.MultipartForm.Value[key]
	out := make([]string, 0, len(rawValues))
	for _, raw := range rawValues {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			out = append(out, raw)
		}
	}
	return out
}

func multipartImageFileToDataURL(fh *multipart.FileHeader) (string, error) {
	file, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("open image failed: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxRemoteImageBytes+1))
	if err != nil {
		return "", fmt.Errorf("read image failed: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("image file is empty")
	}
	if len(data) > maxRemoteImageBytes {
		return "", fmt.Errorf("image file exceeds %d MB limit", maxRemoteImageBytes>>20)
	}
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return "", fmt.Errorf("image file is not an image")
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (r openAIImageGenerationRequest) referenceURLs() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 2+len(r.ImageURLs)+len(r.ImageBase64s))
	appendURL := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	appendURL(r.ImageURL)
	for _, raw := range r.ImageURLs {
		appendURL(raw)
	}
	appendURL(r.ImageBase64)
	for _, raw := range r.ImageBase64s {
		appendURL(raw)
	}
	for _, img := range r.Images {
		appendURL(img.ImageURL)
		appendURL(img.URL)
		appendURL(img.ImageBase64)
		appendURL(img.Base64)
		appendURL(img.B64JSON)
	}
	if len(out) > 4 {
		return out[:4]
	}
	return out
}

func resolveOpenAIImageSize(size string, aspectRatio string) (int, int, string, error) {
	size = strings.ToLower(strings.TrimSpace(size))
	aspectRatio = strings.TrimSpace(aspectRatio)

	if aspectRatio == "" {
		if size == "" {
			return 1536, 1536, "1536x1536", nil
		}
		if width, height, ok := parseOpenAIImageSizePair(size); ok {
			return width, height, fmt.Sprintf("%dx%d", width, height), nil
		}
		switch size {
		case "1:1", "square":
			aspectRatio = "1:1"
		case "16:9", "landscape":
			aspectRatio = "16:9"
		case "9:16", "portrait":
			aspectRatio = "9:16"
		case "4:3":
			aspectRatio = "4:3"
		default:
			return 0, 0, "", fmt.Errorf("unsupported image size %q", size)
		}
	}

	switch strings.ToLower(strings.ReplaceAll(aspectRatio, " ", "")) {
	case "1:1", "square":
		return 1536, 1536, "1536x1536", nil
	case "16:9", "landscape":
		return 2752, 1536, "2752x1536", nil
	case "9:16", "portrait":
		return 1536, 2752, "1536x2752", nil
	case "4:3":
		return 2048, 1536, "2048x1536", nil
	default:
		return 0, 0, "", fmt.Errorf("unsupported image aspect_ratio %q", aspectRatio)
	}
}

type imageSizeResolution struct {
	Width         int
	Height        int
	Label         string
	Mode          string
	OriginalLabel string
	FinalLabel    string
	Transform     string
}

func (s *Server) resolveImageRequestSize(publicModelID string, size string, aspectRatio string) (int, int, string, error) {
	info, err := s.resolveImageRequestSizeDetails(publicModelID, size, aspectRatio)
	if err != nil {
		return 0, 0, "", err
	}
	return info.Width, info.Height, info.Label, nil
}

func (s *Server) resolveImageRequestSizeDetails(publicModelID string, size string, aspectRatio string) (imageSizeResolution, error) {
	mode := s.imageSizeModeForModel(publicModelID)
	if !isGPTImage1KSizeMode(mode) {
		width, height, label, err := resolveOpenAIImageSize(size, aspectRatio)
		if err != nil {
			return imageSizeResolution{}, err
		}
		return imageSizeResolution{
			Width:         width,
			Height:        height,
			Label:         label,
			Mode:          "request",
			OriginalLabel: label,
			FinalLabel:    label,
		}, nil
	}

	originalWidth, originalHeight, originalLabel, originalErr := resolveOpenAIImageSize(size, aspectRatio)
	finalWidth, finalHeight, finalLabel, err := resolveOpenAIImageSize1K(size, aspectRatio)
	if err != nil {
		return imageSizeResolution{}, err
	}
	if originalErr != nil || originalWidth <= 0 || originalHeight <= 0 {
		originalLabel = rawImageSizeInputLabel(size, aspectRatio)
		if originalLabel == "" {
			originalLabel = "未识别"
		}
	}
	transform := buildImageSizeTransform(originalWidth, originalHeight, originalLabel, finalWidth, finalHeight, finalLabel)
	return imageSizeResolution{
		Width:         finalWidth,
		Height:        finalHeight,
		Label:         finalLabel,
		Mode:          "1k",
		OriginalLabel: originalLabel,
		FinalLabel:    finalLabel,
		Transform:     transform,
	}, nil
}

func imageSizeModeConfigKey(modelID string) string {
	return imageConfigKey("image_size_mode_", modelID)
}

func (s *Server) imageSizeModeForModel(publicModelID string) string {
	modelID := strings.ToLower(strings.TrimSpace(publicModelID))
	if s == nil || s.Config == nil {
		return "request"
	}
	if specific := strings.TrimSpace(s.Config.GetString(imageSizeModeConfigKey(modelID), "")); specific != "" {
		return specific
	}
	if isGPTImagePublicModel(modelID) {
		return s.Config.GetString("gpt_image_size_mode", "request")
	}
	return "request"
}

func isGPTImagePublicModel(publicModelID string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(publicModelID)), "gpt-image-2")
}

func isGPTImage1KSizeMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "1k", "1024", "1024x1024", "scale_1k", "scale-1k":
		return true
	default:
		return false
	}
}

func rawImageSizeInputLabel(size string, aspectRatio string) string {
	size = strings.TrimSpace(size)
	aspectRatio = strings.TrimSpace(aspectRatio)
	if size != "" {
		return size
	}
	if aspectRatio != "" {
		return aspectRatio
	}
	return ""
}

func buildImageSizeTransform(originalWidth int, originalHeight int, originalLabel string, finalWidth int, finalHeight int, finalLabel string) string {
	originalLabel = strings.TrimSpace(originalLabel)
	finalLabel = strings.TrimSpace(finalLabel)
	if finalLabel == "" && finalWidth > 0 && finalHeight > 0 {
		finalLabel = fmt.Sprintf("%dx%d", finalWidth, finalHeight)
	}
	originalRatio := "-"
	if originalWidth > 0 && originalHeight > 0 {
		originalRatio = imageRatioLabel(originalWidth, originalHeight)
		if originalLabel == "" {
			originalLabel = fmt.Sprintf("%dx%d", originalWidth, originalHeight)
		}
	}
	finalRatio := "-"
	if finalWidth > 0 && finalHeight > 0 {
		finalRatio = imageRatioLabel(finalWidth, finalHeight)
	}
	if originalLabel == "" {
		originalLabel = "未识别"
	}
	return fmt.Sprintf("%s（%s）→ %s（%s）", originalLabel, originalRatio, finalLabel, finalRatio)
}

func imageRatioLabel(width int, height int) string {
	if width <= 0 || height <= 0 {
		return "-"
	}
	divisor := gcdInt(width, height)
	return fmt.Sprintf("%d:%d", width/divisor, height/divisor)
}

func gcdInt(a int, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func resolveOpenAIImageSize1K(size string, aspectRatio string) (int, int, string, error) {
	if width, height, ok := parseOpenAIImageSizePair(size); ok {
		return nearestGPTImage1KSize(float64(width) / float64(height))
	}
	if ratio, ok := parseOpenAIAspectRatio(size); ok {
		return nearestGPTImage1KSize(ratio)
	}
	if strings.TrimSpace(size) == "" {
		if ratio, ok := parseOpenAIAspectRatio(aspectRatio); ok {
			return nearestGPTImage1KSize(ratio)
		}
		if width, height, _, err := resolveOpenAIImageSize("", aspectRatio); err == nil && width > 0 && height > 0 {
			return nearestGPTImage1KSize(float64(width) / float64(height))
		}
	}
	return 1024, 1024, "1024x1024", nil
}

func parseOpenAIImageSizePair(size string) (int, int, bool) {
	size = strings.ToLower(strings.TrimSpace(size))
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, wErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, hErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if wErr != nil || hErr != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func parseOpenAIAspectRatio(raw string) (float64, bool) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "square":
		return 1, true
	case "landscape":
		return 16.0 / 9.0, true
	case "portrait":
		return 9.0 / 16.0, true
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, false
	}
	left, lErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	right, rErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if lErr != nil || rErr != nil || left <= 0 || right <= 0 {
		return 0, false
	}
	return left / right, true
}

type gptImage1KPreset struct {
	Ratio  float64
	Width  int
	Height int
}

var gptImage1KPresets = []gptImage1KPreset{
	{Ratio: 1, Width: 1024, Height: 1024},
	{Ratio: 2.0 / 3.0, Width: 848, Height: 1264},
	{Ratio: 3.0 / 2.0, Width: 1264, Height: 848},
	{Ratio: 4.0 / 5.0, Width: 928, Height: 1152},
	{Ratio: 5.0 / 4.0, Width: 1152, Height: 928},
	{Ratio: 9.0 / 16.0, Width: 768, Height: 1376},
	{Ratio: 16.0 / 9.0, Width: 1376, Height: 768},
	{Ratio: 21.0 / 9.0, Width: 1584, Height: 672},
	{Ratio: 9.0 / 21.0, Width: 672, Height: 1584},
}

func nearestGPTImage1KSize(ratio float64) (int, int, string, error) {
	if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return 1024, 1024, "1024x1024", nil
	}
	best := gptImage1KPresets[0]
	bestDistance := math.Abs(math.Log(ratio / best.Ratio))
	for _, preset := range gptImage1KPresets[1:] {
		distance := math.Abs(math.Log(ratio / preset.Ratio))
		if distance < bestDistance {
			best = preset
			bestDistance = distance
		}
	}
	width, height := best.Width, best.Height
	return width, height, fmt.Sprintf("%dx%d", width, height), nil
}

func imageConfigKey(prefix, modelID string) string {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	replacer := strings.NewReplacer("-", "_", ".", "_", "/", "_", ":", "_")
	return prefix + replacer.Replace(modelID)
}

func imageModelConfigKey(modelID string) string {
	return imageConfigKey("image_model_id_", modelID)
}

func imageRequestModelConfigKey(modelID string) string {
	return imageConfigKey("image_request_model_", modelID)
}

func imageQualityConfigKey(modelID string) string {
	return imageConfigKey("image_quality_", modelID)
}

func imageAliasForModelID(modelID string) (imageModelAlias, bool) {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	if modelID == "" {
		modelID = "gpt-image-2"
	}
	alias, ok := imageModelAliases[modelID]
	return alias, ok
}

func imageUsesNativeRequest(modelID string) bool {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	return modelID == "gpt-image-2" || modelID == "gpt-image-2-high" || modelID == "gpt-image-2-higher" || modelID == "gpt-image-2-clarity" || modelID == "bananapro"
}

func imageUsesGeminiImage2Request(publicModelID string, requestModel string) bool {
	return strings.ToLower(strings.TrimSpace(publicModelID)) == "bananapro" || strings.ToLower(strings.TrimSpace(requestModel)) == "gemini-image-2"
}

func imageUsesAdobeClarity(modelID string) bool {
	return strings.ToLower(strings.TrimSpace(modelID)) == "gpt-image-2-clarity"
}

func imageQualityForModelID(modelID string) (string, bool) {
	alias, ok := imageAliasForModelID(modelID)
	if !ok {
		return "low", false
	}
	return alias.Quality, true
}

func (s *Server) resolveImageGenerationModel(model string) (publicModelID string, upstreamModelID string, requestModel string, quality string) {
	requestedModelID := strings.ToLower(strings.TrimSpace(model))
	if requestedModelID == "" {
		requestedModelID = "gpt-image-2"
	}
	alias, knownAlias := imageAliasForModelID(requestedModelID)
	if knownAlias {
		publicModelID = alias.PublicID
		upstreamModelID = alias.UpstreamModelID
		requestModel = alias.RequestModel
		quality = alias.Quality
	} else {
		publicModelID = requestedModelID
		upstreamModelID = requestedModelID
		requestModel = "nano-banana-2"
		quality = "low"
	}

	if s != nil && s.Config != nil {
		if specificQuality := strings.TrimSpace(s.Config.GetString(imageQualityConfigKey(requestedModelID))); specificQuality != "" {
			quality = specificQuality
		}
		if specificRequestModel := strings.TrimSpace(s.Config.GetString(imageRequestModelConfigKey(requestedModelID))); specificRequestModel != "" {
			requestModel = specificRequestModel
		} else if !knownAlias {
			if globalRequestModel := strings.TrimSpace(s.Config.GetString("image_request_model", "")); globalRequestModel != "" {
				requestModel = globalRequestModel
			}
		}
		if specificModelID := strings.TrimSpace(s.Config.GetString(imageModelConfigKey(requestedModelID))); specificModelID != "" {
			upstreamModelID = specificModelID
		} else if knownAlias {
			if publicSpecific := strings.TrimSpace(s.Config.GetString(imageModelConfigKey(publicModelID))); publicSpecific != "" {
				upstreamModelID = publicSpecific
			} else if publicModelID == "gpt-image-2" {
				if baseModelID := strings.TrimSpace(s.Config.GetString("image_model_id_gpt_image_2")); baseModelID != "" {
					upstreamModelID = baseModelID
				}
			}
		} else if genericModelID := strings.TrimSpace(s.Config.GetString("image_model_id")); genericModelID != "" {
			upstreamModelID = genericModelID
		}
	}
	if requestModel == "" {
		requestModel = "nano-banana-2"
	}
	if strings.TrimSpace(upstreamModelID) == "" {
		upstreamModelID = publicModelID
	}
	return publicModelID, upstreamModelID, requestModel, quality
}

func (s *Server) logImageRequestFailure(prompt, modelID, quality, sizeLabel, sizeTransform, inputMode, operation string, referenceCount int, tokenID string, session *leonardo.TokenSession, durationSec float64, statusCode int, errorMessage string) {
	if s == nil || s.ReqLog == nil {
		return
	}
	if strings.TrimSpace(inputMode) == "" {
		inputMode = "text_to_image"
	}
	if strings.TrimSpace(operation) == "" {
		operation = "images.generations"
	}
	if referenceCount < 0 {
		referenceCount = 0
	}
	accountName, accountEmail := s.resolveReqLogAccount(tokenID, session)
	s.ReqLog.Add(reqlog.Entry{
		ID:             fmt.Sprintf("img-fail-%d", time.Now().UnixNano()),
		Timestamp:      float64(time.Now().Unix()),
		StatusCode:     statusCode,
		TaskStatus:     "FAILED",
		Type:           "image",
		InputMode:      inputMode,
		ReferenceCount: referenceCount,
		TokenID:        tokenID,
		TokenAttempt:   1,
		AccountName:    accountName,
		AccountEmail:   accountEmail,
		Model:          modelID,
		ModelParams:    fmt.Sprintf("%s quality=%s", sizeLabel, quality),
		SizeTransform:  sizeTransform,
		Prompt:         prompt,
		ErrorCode:      strconv.Itoa(statusCode),
		ErrorMessage:   errorMessage,
		DurationSec:    durationSec,
		Operation:      operation,
	})
}

func (s *Server) waitForLeonardoImageGeneration(session *leonardo.TokenSession, generationID string, timeout time.Duration, interval time.Duration) ([]string, error) {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	if interval <= 0 {
		interval = 4 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		status, err := s.LeonardoClient.PollGenerationStatus(session, generationID)
		if err != nil {
			lastErr = err
			log.Printf("[image_generation] poll status failed for %s: %v", generationID, err)
		} else {
			switch strings.ToUpper(strings.TrimSpace(status.Status)) {
			case "COMPLETE":
				detail, err := s.LeonardoClient.GetGenerationDetail(session, generationID)
				if err != nil {
					lastErr = err
					log.Printf("[image_generation] fetch detail failed for %s: %v", generationID, err)
				} else {
					urls := make([]string, 0, len(detail.Images))
					for _, img := range detail.Images {
						if rawURL := strings.TrimSpace(img.URL); rawURL != "" {
							urls = append(urls, rawURL)
						}
					}
					if len(urls) > 0 {
						return urls, nil
					}
					lastErr = fmt.Errorf("generation complete but no image url returned")
				}
			case "FAILED":
				return nil, fmt.Errorf("image generation status FAILED")
			}
		}
		if time.Now().Add(interval).After(deadline) {
			break
		}
		time.Sleep(interval)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("image generation timed out after %s: %v", timeout, lastErr)
	}
	return nil, fmt.Errorf("image generation timed out after %s", timeout)
}

func (s *Server) resolveReqLogAccount(tokenID string, session *leonardo.TokenSession) (string, string) {
	accountName := ""
	accountEmail := ""

	if tokenID != "" && s.TokenMgr != nil {
		if info := s.TokenMgr.GetByID(tokenID); info != nil {
			accountName = strings.TrimSpace(toString(info["account_name"]))
			accountEmail = strings.TrimSpace(toString(info["account_email"]))
			if accountEmail == "" {
				accountEmail = strings.TrimSpace(toString(info["refresh_profile_email"]))
			}
			if accountName == "" {
				accountName = strings.TrimSpace(toString(info["refresh_profile_name"]))
			}
		}
	}

	if accountEmail == "" && session != nil {
		accountEmail = strings.TrimSpace(session.Email)
	}

	return accountName, accountEmail
}

func (s *Server) logVideoRequestFailure(operation, prompt, modelID string, duration, width, height int, tokenID string, session *leonardo.TokenSession, tokenAttempt int, statusCode int, errorMessage string) {
	if s.ReqLog == nil {
		return
	}
	if tokenAttempt <= 0 {
		tokenAttempt = 1
	}
	accountName, accountEmail := s.resolveReqLogAccount(tokenID, session)
	s.ReqLog.Add(reqlog.Entry{
		ID:           fmt.Sprintf("log-%d", time.Now().UnixNano()),
		StatusCode:   statusCode,
		TaskStatus:   "FAILED",
		Type:         "video",
		TokenID:      tokenID,
		TokenAttempt: tokenAttempt,
		AccountName:  accountName,
		AccountEmail: accountEmail,
		Model:        fmt.Sprintf("%s (%s)", publicVideoModelID(modelID), videoModelParams(modelID, width, height, duration)),
		Prompt:       prompt,
		ErrorCode:    strconv.Itoa(statusCode),
		ErrorMessage: errorMessage,
		Operation:    operation,
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) loadGenerationRetryPolicy() generationRetryPolicy {
	policy := generationRetryPolicy{
		Enabled:     false,
		MaxAttempts: 1,
		BackoffBase: time.Second,
		StatusCodes: map[int]struct{}{},
	}
	if s == nil || s.Config == nil {
		return policy
	}

	policy.Enabled = s.Config.GetBool("retry_enabled", true)
	policy.MaxAttempts = s.Config.GetInt("retry_max_attempts", 3)
	if policy.MaxAttempts < 1 {
		policy.MaxAttempts = 1
	}
	if !policy.Enabled {
		policy.MaxAttempts = 1
	}

	backoffSeconds := s.Config.GetFloat("retry_backoff_seconds", 1)
	if backoffSeconds < 0 {
		backoffSeconds = 0
	}
	policy.BackoffBase = time.Duration(backoffSeconds * float64(time.Second))

	for _, code := range s.Config.GetIntSlice("retry_on_status_codes", []int{429, 451, 500, 502, 503, 504}) {
		if code > 0 {
			policy.StatusCodes[code] = struct{}{}
		}
	}
	for _, item := range s.Config.GetStringSlice("retry_on_error_types", []string{"timeout", "connection", "proxy"}) {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			policy.ErrorMatchers = append(policy.ErrorMatchers, item)
		}
	}
	for _, item := range s.Config.GetStringSlice("retry_same_token_error_types", []string{"provider_moderation_error"}) {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			policy.SameTokenErrorMatchers = append(policy.SameTokenErrorMatchers, item)
		}
	}
	return policy
}

func (p generationRetryPolicy) retryAction(failure *videoGenerationAttemptFailure, phase generationRetryPhase) generationRetryAction {
	if !p.Enabled || failure == nil {
		return generationRetryActionNone
	}
	if phase == generationRetryPhaseAsyncTask {
		if retryFailureMatches(failure, p.SameTokenErrorMatchers) {
			return generationRetryActionSameToken
		}
		return generationRetryActionNone
	}
	if _, ok := p.StatusCodes[failure.StatusCode]; ok {
		return generationRetryActionNextToken
	}
	if retryFailureMatches(failure, p.ErrorMatchers) {
		return generationRetryActionNextToken
	}
	return generationRetryActionNone
}

func (p generationRetryPolicy) shouldRetryAsyncSubmission(err error) bool {
	if !p.Enabled || err == nil {
		return false
	}
	if isIntrinsicRetryableAsyncSubmissionError(err) {
		return true
	}

	failure := &videoGenerationAttemptFailure{
		Message:         strings.TrimSpace(err.Error()),
		RetryCodeSource: extractRetryCodeSource(err.Error()),
	}
	if statusCode, ok := explicitStatusCodeFromGenerationError(err); ok {
		failure.StatusCode = statusCode
		if _, configured := p.StatusCodes[statusCode]; configured {
			return true
		}
	}
	return retryFailureMatches(failure, p.ErrorMatchers)
}

func retryFailureMatches(failure *videoGenerationAttemptFailure, matchers []string) bool {
	if failure == nil {
		return false
	}
	haystacks := []string{
		strings.ToLower(strings.TrimSpace(failure.Message)),
		strings.ToLower(strings.TrimSpace(failure.RetryCodeSource)),
		normalizeRetryMatcher(failure.Message),
		normalizeRetryMatcher(failure.RetryCodeSource),
	}
	for _, matcher := range matchers {
		matcher = strings.ToLower(strings.TrimSpace(matcher))
		if matcher == "" {
			continue
		}
		normalizedMatcher := normalizeRetryMatcher(matcher)
		for _, haystack := range haystacks {
			if haystack == "" {
				continue
			}
			if strings.Contains(haystack, matcher) || (normalizedMatcher != "" && strings.Contains(haystack, normalizedMatcher)) {
				return true
			}
		}
	}
	return false
}

func (p generationRetryPolicy) backoffDelay(attempt int) time.Duration {
	if attempt <= 0 || p.BackoffBase <= 0 {
		return 0
	}
	delay := p.BackoffBase
	for i := 1; i < attempt; i++ {
		delay *= 2
	}
	return delay
}

func isRetryableGuidancePreparationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}

	// Source URL and validation errors are deterministic; switching tokens will
	// not fix a bad image URL, unsupported content type, or malformed payload.
	nonRetryableMarkers := []string{
		"image url returned 400",
		"image url returned 401",
		"image url returned 403",
		"image url returned 404",
		"image url returned empty body",
		"image url exceeds",
		"image url did not return an image content type",
		"image url must use http or https",
		"either id or url is required",
	}
	for _, marker := range nonRetryableMarkers {
		if strings.Contains(msg, marker) {
			return false
		}
	}

	retryableStageMarkers := []string{
		"upload init failed",
		"s3 upload failed",
		"wait for init image failed",
		"wait for staged video asset failed",
	}
	for _, marker := range retryableStageMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func normalizeRetryMatcher(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "_", "-", "_", ":", "_", ";", "_", ",", "_", ".", "_", "/", "_", "\\", "_", "(", "_", ")", "_")
	return replacer.Replace(raw)
}

type readCloser struct {
	data []byte
	pos  int
}

func (rc *readCloser) Read(p []byte) (int, error) {
	if rc.pos >= len(rc.data) {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, rc.data[rc.pos:])
	rc.pos += n
	return n, nil
}
func (rc *readCloser) Close() error { return nil }

func (s *Server) reloadRuntimeClients() {
	basicProxy := ""
	if s.Config.GetBool("use_proxy", false) {
		basicProxy = s.Config.GetString("proxy", "")
	}

	uploadMode := "basic"
	uploadProxy := ""
	if s.Config != nil {
		uploadMode = s.Config.GetString("leonardo_upload_proxy_mode", "basic")
		uploadProxy = s.Config.GetString("leonardo_upload_proxy", "")
	}
	leonardoClient, err := leonardo.NewClientWithUploadProxyConfig(basicProxy, uploadMode, uploadProxy)
	if err != nil {
		log.Printf("[proxy] invalid Leonardo upload proxy config (mode=%s): %v; fallback to basic", uploadMode, err)
		leonardoClient = leonardo.NewClient(basicProxy)
	}
	s.LeonardoClient = leonardoClient
	if s.Config != nil {
		s.LeonardoClient.SetJWTRefreshMarginMinutes(s.Config.GetInt("jwt_refresh_margin_minutes", 5))
	}
	if s.ReqLog != nil {
		limit := 5000
		if s.Config != nil {
			limit = s.Config.GetInt("request_log_retention_limit", 5000)
		}
		s.ReqLog.SetMaxEntries(limit)
	}
	// 保留现有 Leonardo 会话缓存，避免仅仅因为保存系统配置就丢失 JWT，
	// 导致下一次手动刷新总是重新续成 1 小时。
	s.leoSessionMu.Lock()
	if s.leoSessions == nil {
		s.leoSessions = make(map[string]*leonardo.TokenSession)
	}
	s.leoSessionMu.Unlock()
}

func normalizeVideoModelID(modelID string) (string, bool) {
	switch strings.TrimSpace(modelID) {
	case "video-2.0", "seedance-2.0":
		return "seedance-2.0", true
	case "video-2.0-fast", "seedance-2.0-fast":
		return "seedance-2.0-fast", true
	case "video-2.0-mini", "seedance-2.0-mini":
		return "seedance-2.0-mini", true
	case "video-2.0-480p", "seedance-2.0-480p":
		return "seedance-2.0-480p", true
	case "video-2.0-fast-480p", "seedance-2.0-fast-480p":
		return "seedance-2.0-fast-480p", true
	case "video-2.0-mini-480p", "seedance-2.0-mini-480p":
		return "seedance-2.0-mini-480p", true
	case "sora-2", "sora2":
		return "sora-2", true
	case "ko3", "kling-o3", "kling-video-o-3":
		return "kling-video-o-3", true
	case "minimax-h3":
		return "minimax-h3", true
	default:
		return "", false
	}
}

func publicVideoModelID(modelID string) string {
	switch strings.TrimSpace(modelID) {
	case "seedance-2.0", "video-2.0":
		return "video-2.0"
	case "seedance-2.0-fast", "video-2.0-fast":
		return "video-2.0-fast"
	case "seedance-2.0-mini", "video-2.0-mini":
		return "video-2.0-mini"
	case "seedance-2.0-480p", "video-2.0-480p":
		return "video-2.0-480p"
	case "seedance-2.0-fast-480p", "video-2.0-fast-480p":
		return "video-2.0-fast-480p"
	case "seedance-2.0-mini-480p", "video-2.0-mini-480p":
		return "video-2.0-mini-480p"
	case "sora2", "sora-2":
		return "sora2"
	case "kling-video-o-3", "kling-o3", "ko3":
		return "ko3"
	case "hailuo-03", "minimax-h3":
		return "minimax-h3"
	default:
		return strings.TrimSpace(modelID)
	}
}

func publicRequestLogModel(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "hailuo-03" {
		return "minimax-h3"
	}
	if strings.HasPrefix(modelID, "hailuo-03 (") {
		return "minimax-h3" + strings.TrimPrefix(modelID, "hailuo-03")
	}
	return publicVideoModelID(modelID)
}

func isSora2ModelID(modelID string) bool {
	modelID = strings.TrimSpace(modelID)
	return strings.EqualFold(modelID, "sora-2") || strings.EqualFold(modelID, "sora2")
}

func isKlingO3ModelID(modelID string) bool {
	switch strings.TrimSpace(modelID) {
	case "kling-video-o-3", "kling-o3", "ko3":
		return true
	default:
		return false
	}
}

func isMinimaxH3ModelID(modelID string) bool {
	switch strings.TrimSpace(modelID) {
	case "hailuo-03", "minimax-h3":
		return true
	default:
		return false
	}
}

func isSeedanceModelID(modelID string) bool {
	switch strings.TrimSpace(modelID) {
	case "seedance-2.0", "video-2.0", "seedance-2.0-fast", "video-2.0-fast", "seedance-2.0-mini", "video-2.0-mini", "seedance-2.0-480p", "video-2.0-480p", "seedance-2.0-fast-480p", "video-2.0-fast-480p", "seedance-2.0-mini-480p", "video-2.0-mini-480p":
		return true
	default:
		return false
	}
}

func isSeedance480pModelID(modelID string) bool {
	switch strings.TrimSpace(modelID) {
	case "seedance-2.0-480p", "video-2.0-480p", "seedance-2.0-fast-480p", "video-2.0-fast-480p", "seedance-2.0-mini-480p", "video-2.0-mini-480p":
		return true
	default:
		return false
	}
}

func defaultVideoDuration(modelID string) int {
	if isSora2ModelID(modelID) {
		return defaultSora2VideoDuration
	}
	if isKlingO3ModelID(modelID) {
		return defaultKlingO3VideoDuration
	}
	if isMinimaxH3ModelID(modelID) {
		return defaultMinimaxH3VideoDuration
	}
	return defaultSeedanceVideoDuration
}

func defaultVideoSize(modelID string) (int, int) {
	if isSora2ModelID(modelID) {
		return 720, 1280
	}
	if isKlingO3ModelID(modelID) {
		return 1080, 1920
	}
	if isMinimaxH3ModelID(modelID) {
		return 2560, 1440
	}
	if isSeedance480pModelID(modelID) {
		return 864, 496
	}
	return 1280, 720
}

func videoSizeForAspectRatio(modelID string, aspectRatio string) (int, int, bool) {
	aspectRatio = strings.TrimSpace(aspectRatio)
	switch aspectRatio {
	case "16:9":
		if isMinimaxH3ModelID(modelID) {
			return 2560, 1440, true
		}
		if isKlingO3ModelID(modelID) {
			return 1920, 1080, true
		}
		if isSeedance480pModelID(modelID) {
			return 864, 496, true
		}
		return 1280, 720, true
	case "9:16":
		if isMinimaxH3ModelID(modelID) {
			return 1440, 2560, true
		}
		if isKlingO3ModelID(modelID) {
			return 1080, 1920, true
		}
		if isSeedance480pModelID(modelID) {
			return 496, 864, true
		}
		return 720, 1280, true
	case "1:1":
		if isSora2ModelID(modelID) {
			return 0, 0, false
		}
		if isMinimaxH3ModelID(modelID) {
			return 1440, 1440, true
		}
		if isKlingO3ModelID(modelID) {
			return 1440, 1440, true
		}
		if isSeedance480pModelID(modelID) {
			return 640, 640, true
		}
		return 960, 960, true
	case "4:3":
		if isMinimaxH3ModelID(modelID) {
			return 1920, 1440, true
		}
		return 0, 0, false
	case "3:4":
		if isMinimaxH3ModelID(modelID) {
			return 1440, 1920, true
		}
		return 0, 0, false
	case "21:9":
		if isMinimaxH3ModelID(modelID) {
			return 3360, 1440, true
		}
		return 0, 0, false
	default:
		return 0, 0, false
	}
}

func videoModelParams(_ string, width int, height int, duration int) string {
	return fmt.Sprintf("%dx%d %ds", width, height, duration)
}

func videoGenerationPollURL(requestPath string, generationID string) string {
	prefix := "/v1/video/generations"
	if strings.HasPrefix(requestPath, "/v1/video/async-generations") {
		prefix = "/v1/video/async-generations"
	}
	return fmt.Sprintf("%s/%s", prefix, generationID)
}

func videoGenerationIDFromPath(path string) string {
	if id := extractPathParam(path, "/v1/video/generations/"); id != "" {
		return id
	}
	return extractPathParam(path, "/v1/video/async-generations/")
}

func isAllowedSora2Duration(duration int) bool {
	switch duration {
	case 4, 8, 12:
		return true
	default:
		return false
	}
}

func isAllowedSora2Size(width int, height int) bool {
	return (width == 720 && height == 1280) || (width == 1280 && height == 720)
}

func isAllowedSeedance480pSize(width int, height int) bool {
	return (width == 496 && height == 864) || (width == 864 && height == 496) || (width == 640 && height == 640)
}

func isAllowedMinimaxH3Duration(duration int) bool {
	return duration >= 5 && duration <= 15
}

func isAllowedMinimaxH3Size(width int, height int) bool {
	return (width == 2560 && height == 1440) ||
		(width == 1440 && height == 2560) ||
		(width == 1440 && height == 1440) ||
		(width == 1920 && height == 1440) ||
		(width == 1440 && height == 1920) ||
		(width == 3360 && height == 1440)
}

func isAllowedKlingO3Duration(duration int, videoReferenceMode bool) bool {
	return duration >= 3 && duration <= 15
}

func klingO3DurationError(videoReferenceMode bool) string {
	return "ko3 duration must be between 3 and 15 seconds"
}

func isAllowedKlingO3Size(width int, height int, videoReferenceMode bool) bool {
	if videoReferenceMode && width == 0 && height == 0 {
		return true
	}
	return (width == 1440 && height == 1440) || (width == 1080 && height == 1920) || (width == 1920 && height == 1080)
}

func klingO3SizeError(videoReferenceMode bool) string {
	if videoReferenceMode {
		return "ko3 video reference size must be 0x0, 1440x1440, 1080x1920, or 1920x1080"
	}
	return "ko3 size must be 1440x1440, 1080x1920, or 1920x1080"
}

func hasVideoReferenceInput(data map[string]interface{}) bool {
	if data == nil {
		return false
	}
	if strings.TrimSpace(toString(data["video_url"])) != "" {
		return true
	}
	if rawVideos, ok := data["video_reference"].([]interface{}); ok {
		for _, item := range rawVideos {
			entry, _ := item.(map[string]interface{})
			if strings.TrimSpace(toString(entry["id"])) != "" || strings.TrimSpace(toString(entry["url"])) != "" {
				return true
			}
		}
	}
	return false
}

func hasAudioReferenceInput(data map[string]interface{}) bool {
	if data == nil {
		return false
	}
	if strings.TrimSpace(toString(data["audio_url"])) != "" {
		return true
	}
	if rawAudios, ok := audioReferenceInputs(data); ok {
		for _, item := range rawAudios {
			entry, _ := item.(map[string]interface{})
			if audio, ok := entry["audio"].(map[string]interface{}); ok {
				entry = audio
			}
			if strings.TrimSpace(toString(entry["id"])) != "" || strings.TrimSpace(toString(entry["url"])) != "" {
				return true
			}
		}
	}
	return false
}

func minimaxH3ImageReferenceInputs(data map[string]interface{}) []interface{} {
	if data == nil {
		return nil
	}
	if rawRefs, ok := data["image_reference"].([]interface{}); ok {
		return rawRefs
	}
	guidances, _ := data["guidances"].(map[string]interface{})
	if rawRefs, ok := guidances["image_reference"].([]interface{}); ok {
		return rawRefs
	}
	return nil
}

func validImageReferenceEntry(item interface{}) bool {
	entry, _ := item.(map[string]interface{})
	if image, ok := entry["image"].(map[string]interface{}); ok {
		entry = image
	}
	return strings.TrimSpace(toString(entry["id"])) != "" || strings.TrimSpace(toString(entry["url"])) != ""
}

func countMinimaxH3ImageReferenceInputs(data map[string]interface{}) int {
	if data == nil {
		return 0
	}
	count := 0
	if strings.TrimSpace(toString(data["image_url"])) != "" {
		count++
	}
	if rawURLs, ok := data["image_urls"].([]interface{}); ok {
		for _, rawURL := range rawURLs {
			if strings.TrimSpace(toString(rawURL)) != "" {
				count++
			}
		}
	}
	if rawRefs, ok := data["image_guidance"].([]interface{}); ok {
		for _, item := range rawRefs {
			if validImageReferenceEntry(item) {
				count++
			}
		}
	}
	for _, item := range minimaxH3ImageReferenceInputs(data) {
		if validImageReferenceEntry(item) {
			count++
		}
	}
	return count
}

func countFrameInputs(data map[string]interface{}, urlField string, frameField string) int {
	if data == nil {
		return 0
	}
	count := 0
	if strings.TrimSpace(toString(data[urlField])) != "" {
		count++
	}
	if rawFrames, ok := data[frameField].([]interface{}); ok {
		for _, item := range rawFrames {
			if validImageReferenceEntry(item) {
				count++
			}
		}
	}
	return count
}

func validateMinimaxH3GuidanceInput(data map[string]interface{}) error {
	imageCount := countMinimaxH3ImageReferenceInputs(data)
	startCount := countFrameInputs(data, "start_image_url", "start_frame")
	endCount := countFrameInputs(data, "end_image_url", "end_frame")
	hasFrames := startCount > 0 || endCount > 0

	if hasVideoReferenceInput(data) {
		return fmt.Errorf("minimax-h3 does not support video_reference")
	}
	if imageCount > 5 {
		return fmt.Errorf("minimax-h3 supports at most 5 image references")
	}
	if startCount > 1 || endCount > 1 {
		return fmt.Errorf("minimax-h3 supports at most one start frame and one end frame")
	}
	if hasFrames && imageCount > 0 {
		return fmt.Errorf("minimax-h3 image-reference mode cannot be combined with start/end-frame mode")
	}
	if hasAudioReferenceInput(data) && (imageCount == 0 || hasFrames) {
		return fmt.Errorf("minimax-h3 audio_reference is only supported in image-reference mode")
	}
	return nil
}

func hasUnsupportedKlingO3GuidanceInput(data map[string]interface{}) bool {
	return false
}

func leonardoVideoResolutionMode(modelID string, width int, height int) string {
	if isMinimaxH3ModelID(modelID) {
		return ""
	}
	if isKlingO3ModelID(modelID) {
		return "RESOLUTION_1080"
	}
	if isSeedance480pModelID(modelID) {
		return ""
	}
	return "RESOLUTION_720"
}

func (s *Server) submitLeonardoVideoGeneration(session *leonardo.TokenSession, usedTokenID string, tokenAttempt int, prompt string, modelID string, duration int, width int, height int, imageRefs []leonardo.ImageRef, startFrames []leonardo.FrameRef, endFrames []leonardo.FrameRef, videoRefs []leonardo.VideoRef, audioRefs []leonardo.AudioRef) (*videoGenerationSubmission, *videoGenerationAttemptFailure) {
	if s.LeonardoClient == nil {
		return nil, &videoGenerationAttemptFailure{
			StatusCode:      http.StatusInternalServerError,
			Message:         "Leonardo client not initialized",
			ErrorType:       "server_error",
			RetryCodeSource: "server_error leonardo_client_not_initialized",
		}
	}

	genReq := &leonardo.GenerateRequest{
		Model:  modelID,
		Public: true,
		Params: leonardo.GenerateParams{
			Prompt:         prompt,
			Mode:           leonardoVideoResolutionMode(modelID, width, height),
			Duration:       duration,
			Width:          width,
			Height:         height,
			MotionHasAudio: true,
			ImageRefs:      imageRefs,
			StartFrame:     startFrames,
			EndFrame:       endFrames,
			VideoRefs:      videoRefs,
			AudioRefs:      audioRefs,
		},
	}

	startTime := time.Now()
	result, err := s.LeonardoClient.Generate(session, genReq)
	if err != nil {
		statusCode := statusCodeFromGenerationError(err)
		insufficientTokens := isInsufficientTokensMessage(err.Error())
		return nil, &videoGenerationAttemptFailure{
			StatusCode:      statusCode,
			Message:         fmt.Sprintf("generation failed: %v", err),
			ErrorType:       "server_error",
			RetryCodeSource: extractRetryCodeSource(err.Error()),
			MarkInvalid:     !insufficientTokens && !isRetryableGenerationError(err),
			Insufficient:    insufficientTokens,
		}
	}
	s.applyTokenCreditCost(usedTokenID, result.APICreditCost)

	if s.ReqLog != nil {
		accountName, accountEmail := s.resolveReqLogAccount(usedTokenID, session)
		if tokenAttempt <= 0 {
			tokenAttempt = 1
		}
		s.ReqLog.Add(reqlog.Entry{
			Timestamp:            float64(startTime.Unix()),
			StatusCode:           200,
			TaskStatus:           "IN_PROGRESS",
			Type:                 "video",
			TokenID:              usedTokenID,
			TokenAttempt:         tokenAttempt,
			AccountName:          accountName,
			AccountEmail:         accountEmail,
			Model:                publicVideoModelID(modelID),
			ModelParams:          videoModelParams(modelID, width, height, duration),
			Prompt:               prompt,
			GenerationID:         result.GenerationID,
			UpstreamGenerationID: result.GenerationID,
			CreditCost:           result.APICreditCost,
			Operation:            "leonardo.generate",
		})
	}

	return &videoGenerationSubmission{
		GenerationID: result.GenerationID,
		CreatedAt:    startTime,
		Request:      genReq,
	}, nil
}

func (s *Server) trackLeonardoVideoGeneration(ctx *asyncVideoGenerationContext, pollInterval time.Duration, attemptTimeout time.Duration) {
	if s == nil || ctx == nil || ctx.Session == nil || s.LeonardoClient == nil {
		return
	}
	if ctx.Attempt < 1 {
		ctx.Attempt = 1
	}
	if ctx.StartedAt.IsZero() {
		ctx.StartedAt = time.Now()
	}
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	if attemptTimeout <= 0 {
		attemptTimeout = 10 * time.Minute
	}

attemptLoop:
	for {
		deadline := time.Now().Add(attemptTimeout)
		for time.Now().Before(deadline) {
			time.Sleep(pollInterval)
			currentGenerationID := strings.TrimSpace(ctx.UpstreamGenerationID)
			status, pollErr := s.LeonardoClient.PollGenerationStatus(ctx.Session, currentGenerationID)
			if pollErr != nil {
				continue
			}
			elapsed := time.Since(ctx.StartedAt).Seconds()
			switch status.Status {
			case "FAILED":
				failureMessage := s.waitForGenerationFailureReason(ctx.Session, currentGenerationID, "generation")
				s.scheduleFailedGenerationCreditsReconciliation(ctx.TokenID, ctx.Session, currentGenerationID)
				if retried, retryFailure := s.retryAsyncVideoGenerationSameToken(ctx, failureMessage); retried {
					continue attemptLoop
				} else if retryFailure != "" {
					failureMessage = retryFailure
				}
				if s.ReqLog != nil {
					s.ReqLog.UpdateByGenerationID(ctx.PublicGenerationID, "FAILED", 502, "", "", failureMessage)
					s.ReqLog.UpdateDuration(ctx.PublicGenerationID, elapsed)
				}
				return

			case "COMPLETE":
				detail, detailErr := s.LeonardoClient.GetGenerationDetail(ctx.Session, currentGenerationID)
				if detailErr != nil {
					continue
				}
				previewURL := ""
				previewKind := ""
				for _, img := range detail.Images {
					if img.MotionMP4 != "" {
						previewURL = img.MotionMP4
						previewKind = "video"
						break
					}
					if previewURL == "" && img.URL != "" {
						previewURL = img.URL
						previewKind = "image"
					}
				}
				if previewURL != "" {
					finalURL, materializeErr := s.materializeGeneratedMedia(previewURL, ctx.PublicGenerationID, previewKind)
					if materializeErr != nil {
						if s.ReqLog != nil {
							s.ReqLog.UpdateByGenerationID(ctx.PublicGenerationID, "FAILED", 502, "", "", fmt.Sprintf("save generated media failed: %v", materializeErr))
							s.ReqLog.UpdateDuration(ctx.PublicGenerationID, elapsed)
						}
						s.refreshTokenCredits(ctx.TokenID, ctx.Session)
						return
					}
					previewURL = finalURL
				}
				if s.ReqLog != nil {
					s.ReqLog.UpdateByGenerationID(ctx.PublicGenerationID, "COMPLETE", 200, previewURL, previewKind, "")
					s.ReqLog.UpdateDuration(ctx.PublicGenerationID, elapsed)
				}
				s.reportVideoGenerationSuccess(ctx.TokenID, ctx.ModelID)
				s.refreshTokenCredits(ctx.TokenID, ctx.Session)
				return
			}
		}

		if s.ReqLog != nil {
			s.ReqLog.UpdateByGenerationID(ctx.PublicGenerationID, "FAILED", 504, "", "", "Generation timed out")
			s.ReqLog.UpdateDuration(ctx.PublicGenerationID, time.Since(ctx.StartedAt).Seconds())
		}
		s.refreshTokenCredits(ctx.TokenID, ctx.Session)
		return
	}
}

func (s *Server) retryAsyncVideoGenerationSameToken(ctx *asyncVideoGenerationContext, failureMessage string) (bool, string) {
	policy := s.loadGenerationRetryPolicy()
	failure := &videoGenerationAttemptFailure{
		StatusCode:      http.StatusBadGateway,
		Message:         strings.TrimSpace(failureMessage),
		ErrorType:       "server_error",
		RetryCodeSource: extractRetryCodeSource(failureMessage),
	}
	if policy.retryAction(failure, generationRetryPhaseAsyncTask) != generationRetryActionSameToken || ctx.Attempt >= policy.MaxAttempts {
		return false, ""
	}
	if ctx.Request == nil {
		return false, strings.TrimSpace(failureMessage) + "; async retry context is unavailable"
	}

	for ctx.Attempt < policy.MaxAttempts {
		if delay := policy.backoffDelay(ctx.Attempt); delay > 0 {
			time.Sleep(delay)
		}

		nextAttempt := ctx.Attempt + 1
		result, err := s.LeonardoClient.Generate(ctx.Session, ctx.Request)
		ctx.Attempt = nextAttempt
		if s.ReqLog != nil {
			s.ReqLog.UpdateAttemptByGenerationID(ctx.PublicGenerationID, nextAttempt)
		}
		if err != nil {
			retryable := policy.shouldRetryAsyncSubmission(err)
			if ctx.Attempt < policy.MaxAttempts && retryable {
				log.Printf("[generation] async task %s same-token submission %d/%d failed with retryable error; retrying: %v", ctx.PublicGenerationID, nextAttempt, policy.MaxAttempts, err)
				continue
			}
			log.Printf("[generation] async task %s same-token submission %d/%d stopped: %v", ctx.PublicGenerationID, nextAttempt, policy.MaxAttempts, err)
			return false, asyncRetryPublicFailureMessage(failureMessage)
		}

		s.applyTokenCreditCost(ctx.TokenID, result.APICreditCost)
		ctx.UpstreamGenerationID = result.GenerationID
		if s.ReqLog != nil {
			s.ReqLog.UpdateRetryByGenerationID(ctx.PublicGenerationID, result.GenerationID, nextAttempt)
		}
		log.Printf("[generation] async task %s failed with %s; same-token retry %d/%d submitted as %s", ctx.PublicGenerationID, strings.TrimSpace(failureMessage), nextAttempt, policy.MaxAttempts, result.GenerationID)
		return true, ""
	}
	return false, asyncRetryPublicFailureMessage(failureMessage)
}

func asyncRetryPublicFailureMessage(failureMessage string) string {
	failureMessage = strings.TrimSpace(failureMessage)
	if failureMessage == "" {
		return "generation failed"
	}
	return failureMessage
}

func (s *Server) resolveOpenAIVideoGuidanceInputs(data map[string]interface{}, session *leonardo.TokenSession, modelID string) ([]leonardo.ImageRef, []leonardo.FrameRef, []leonardo.FrameRef, []leonardo.VideoRef, []leonardo.AudioRef, error) {
	uploadCache := make(map[string]string)

	var imageRefs []leonardo.ImageRef
	var startFrames []leonardo.FrameRef
	var endFrames []leonardo.FrameRef
	var videoRefs []leonardo.VideoRef
	var audioRefs []leonardo.AudioRef
	audioUploadCache := make(map[string]string)

	if imageURL := strings.TrimSpace(toString(data["image_url"])); imageURL != "" {
		imageID, err := s.resolveLeonardoImageID(session, "", imageURL, uploadCache)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("invalid image_url: %w", err)
		}
		if isSora2ModelID(modelID) {
			startFrames = append(startFrames, leonardo.FrameRef{ID: imageID, Type: "UPLOADED"})
		} else {
			imageRefs = append(imageRefs, leonardo.ImageRef{
				ID:       imageID,
				Type:     "UPLOADED",
				Strength: "MID",
			})
		}
	}

	if imageURL := strings.TrimSpace(toString(data["start_image_url"])); imageURL != "" {
		imageID, err := s.resolveLeonardoImageID(session, "", imageURL, uploadCache)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("invalid start_image_url: %w", err)
		}
		startFrames = append(startFrames, leonardo.FrameRef{ID: imageID, Type: "UPLOADED"})
	}

	if imageURL := strings.TrimSpace(toString(data["end_image_url"])); imageURL != "" {
		imageID, err := s.resolveLeonardoImageID(session, "", imageURL, uploadCache)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("invalid end_image_url: %w", err)
		}
		endFrames = append(endFrames, leonardo.FrameRef{ID: imageID, Type: "UPLOADED"})
	}

	if rawURLs, ok := data["image_urls"].([]interface{}); ok {
		for idx, rawURL := range rawURLs {
			imageURL := strings.TrimSpace(toString(rawURL))
			if imageURL == "" {
				continue
			}
			imageID, err := s.resolveLeonardoImageID(session, "", imageURL, uploadCache)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("invalid image_urls[%d]: %w", idx, err)
			}
			imageRefs = append(imageRefs, leonardo.ImageRef{
				ID:       imageID,
				Type:     "UPLOADED",
				Strength: "MID",
			})
		}
	}

	if rawGuidance, ok := data["image_guidance"].([]interface{}); ok {
		for idx, item := range rawGuidance {
			entry, _ := item.(map[string]interface{})
			rawID := toString(entry["id"])
			rawURL := toString(entry["url"])
			imageID, err := s.resolveLeonardoImageID(session, rawID, rawURL, uploadCache)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("invalid image_guidance[%d]: %w", idx, err)
			}
			refType := strings.TrimSpace(toString(entry["type"]))
			if refType == "" || (strings.TrimSpace(rawID) == "" && strings.TrimSpace(rawURL) != "") {
				refType = "UPLOADED"
			}
			strength := strings.ToUpper(strings.TrimSpace(toString(entry["strength"])))
			if strength == "" {
				strength = "MID"
			}
			imageRefs = append(imageRefs, leonardo.ImageRef{
				ID:       imageID,
				Type:     refType,
				Strength: strength,
			})
		}
	}

	for idx, item := range minimaxH3ImageReferenceInputs(data) {
		entry, _ := item.(map[string]interface{})
		strength := strings.ToUpper(strings.TrimSpace(toString(entry["strength"])))
		if image, ok := entry["image"].(map[string]interface{}); ok {
			entry = image
		}
		rawID := toString(entry["id"])
		rawURL := toString(entry["url"])
		imageID, err := s.resolveLeonardoImageID(session, rawID, rawURL, uploadCache)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("invalid image_reference[%d]: %w", idx, err)
		}
		refType := strings.TrimSpace(toString(entry["type"]))
		if refType == "" || (strings.TrimSpace(rawID) == "" && strings.TrimSpace(rawURL) != "") {
			refType = "UPLOADED"
		}
		if strength == "" {
			strength = "MID"
		}
		imageRefs = append(imageRefs, leonardo.ImageRef{ID: imageID, Type: refType, Strength: strength})
	}

	if rawFrames, ok := data["start_frame"].([]interface{}); ok {
		for idx, item := range rawFrames {
			entry, _ := item.(map[string]interface{})
			if image, ok := entry["image"].(map[string]interface{}); ok {
				entry = image
			}
			rawID := toString(entry["id"])
			rawURL := toString(entry["url"])
			imageID, err := s.resolveLeonardoImageID(session, rawID, rawURL, uploadCache)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("invalid start_frame[%d]: %w", idx, err)
			}
			refType := strings.TrimSpace(toString(entry["type"]))
			if refType == "" || (strings.TrimSpace(rawID) == "" && strings.TrimSpace(rawURL) != "") {
				refType = "UPLOADED"
			}
			startFrames = append(startFrames, leonardo.FrameRef{ID: imageID, Type: refType})
		}
	}

	if rawFrames, ok := data["end_frame"].([]interface{}); ok {
		for idx, item := range rawFrames {
			entry, _ := item.(map[string]interface{})
			if image, ok := entry["image"].(map[string]interface{}); ok {
				entry = image
			}
			rawID := toString(entry["id"])
			rawURL := toString(entry["url"])
			imageID, err := s.resolveLeonardoImageID(session, rawID, rawURL, uploadCache)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("invalid end_frame[%d]: %w", idx, err)
			}
			refType := strings.TrimSpace(toString(entry["type"]))
			if refType == "" || (strings.TrimSpace(rawID) == "" && strings.TrimSpace(rawURL) != "") {
				refType = "UPLOADED"
			}
			endFrames = append(endFrames, leonardo.FrameRef{ID: imageID, Type: refType})
		}
	}

	if videoURL := strings.TrimSpace(toString(data["video_url"])); videoURL != "" {
		videoRef, err := s.resolveLeonardoVideoRef(session, "", videoURL, 0, uploadCache)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("invalid video_url: %w", err)
		}
		videoRefs = append(videoRefs, videoRef)
	}

	if rawVideos, ok := data["video_reference"].([]interface{}); ok {
		for idx, item := range rawVideos {
			entry, _ := item.(map[string]interface{})
			rawID := toString(entry["id"])
			rawURL := toString(entry["url"])
			var durationHint float64
			switch durationValue := entry["duration"].(type) {
			case float64:
				durationHint = durationValue
			case int:
				durationHint = float64(durationValue)
			}
			videoRef, err := s.resolveLeonardoVideoRef(session, rawID, rawURL, durationHint, uploadCache)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("invalid video_reference[%d]: %w", idx, err)
			}
			refType := strings.TrimSpace(toString(entry["type"]))
			if refType == "" || (strings.TrimSpace(rawID) == "" && strings.TrimSpace(rawURL) != "") {
				refType = "UPLOADED"
			}
			videoRef.Type = refType
			videoRefs = append(videoRefs, videoRef)
		}
	}

	if audioURL := strings.TrimSpace(toString(data["audio_url"])); audioURL != "" {
		audioRef, err := s.resolveLeonardoAudioRef(session, "", audioURL, 0, audioUploadCache)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("invalid audio_url: %w", err)
		}
		audioRefs = append(audioRefs, audioRef)
	}

	if rawAudios, ok := audioReferenceInputs(data); ok {
		for idx, item := range rawAudios {
			ref, err := s.resolveLeonardoAudioRefFromInput(session, item, audioUploadCache)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("invalid audio_reference[%d]: %w", idx, err)
			}
			audioRefs = append(audioRefs, ref)
		}
	}

	return imageRefs, startFrames, endFrames, videoRefs, audioRefs, nil
}

func (s *Server) resolveLeonardoAudioRefFromInput(session *leonardo.TokenSession, item interface{}, cache map[string]string) (leonardo.AudioRef, error) {
	entry, _ := item.(map[string]interface{})
	if audio, ok := entry["audio"].(map[string]interface{}); ok {
		entry = audio
	}
	rawID := toString(entry["id"])
	rawURL := toString(entry["url"])
	duration := toFloat64(entry["duration"])
	ref, err := s.resolveLeonardoAudioRef(session, rawID, rawURL, duration, cache)
	if err != nil {
		return leonardo.AudioRef{}, err
	}
	refType := strings.TrimSpace(toString(entry["type"]))
	if refType == "" || (strings.TrimSpace(rawID) == "" && strings.TrimSpace(rawURL) != "") {
		refType = "UPLOADED"
	}
	ref.Type = refType
	return ref, nil
}

func audioReferenceInputs(data map[string]interface{}) ([]interface{}, bool) {
	if data == nil {
		return nil, false
	}
	if rawAudios, ok := data["audio_reference"].([]interface{}); ok {
		return rawAudios, true
	}
	guidances, _ := data["guidances"].(map[string]interface{})
	if guidances == nil {
		return nil, false
	}
	rawAudios, ok := guidances["audio_reference"].([]interface{})
	return rawAudios, ok
}

func hasUnsupportedSora2GuidanceInput(data map[string]interface{}) bool {
	if data == nil {
		return false
	}
	stringFields := []string{"end_image_url", "video_url", "audio_url"}
	for _, field := range stringFields {
		if strings.TrimSpace(toString(data[field])) != "" {
			return true
		}
	}
	arrayFields := []string{"image_urls", "image_guidance", "end_frame", "video_reference", "audio_reference"}
	for _, field := range arrayFields {
		if rawItems, ok := data[field].([]interface{}); ok && len(rawItems) > 0 {
			return true
		}
	}
	return false
}

func countSora2StartFrameInputs(data map[string]interface{}) int {
	if data == nil {
		return 0
	}
	count := 0
	if strings.TrimSpace(toString(data["image_url"])) != "" {
		count++
	}
	if strings.TrimSpace(toString(data["start_image_url"])) != "" {
		count++
	}
	if rawFrames, ok := data["start_frame"].([]interface{}); ok {
		for _, item := range rawFrames {
			entry, _ := item.(map[string]interface{})
			if strings.TrimSpace(toString(entry["id"])) != "" || strings.TrimSpace(toString(entry["url"])) != "" {
				count++
			}
		}
	}
	return count
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch value := v.(type) {
	case string:
		return value
	default:
		return fmt.Sprintf("%v", value)
	}
}

func toFloat64(v interface{}) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case int32:
		return float64(value)
	case json.Number:
		f, _ := value.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return f
	default:
		f, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprintf("%v", value)), 64)
		return f
	}
}

func isExpiredTokenInfo(info map[string]interface{}) bool {
	if info == nil {
		return false
	}
	expiresAt := toFloat64(info["expires_at"])
	if expiresAt <= 0 {
		return false
	}
	return float64(time.Now().Unix()) >= expiresAt
}

func statusCodeFromGenerationError(err error) int {
	if statusCode, ok := explicitStatusCodeFromGenerationError(err); ok {
		return statusCode
	}
	return http.StatusBadGateway
}

func explicitStatusCodeFromGenerationError(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, " 429"), strings.Contains(msg, "(429)"), strings.Contains(msg, "returned 429"), strings.Contains(msg, "rate limit"):
		return http.StatusTooManyRequests, true
	case strings.Contains(msg, " 451"), strings.Contains(msg, "(451)"), strings.Contains(msg, "returned 451"):
		return 451, true
	case strings.Contains(msg, " 500"), strings.Contains(msg, "(500)"), strings.Contains(msg, "returned 500"):
		return http.StatusInternalServerError, true
	case strings.Contains(msg, " 502"), strings.Contains(msg, "(502)"), strings.Contains(msg, "returned 502"):
		return http.StatusBadGateway, true
	case strings.Contains(msg, " 503"), strings.Contains(msg, "(503)"), strings.Contains(msg, "returned 503"):
		return http.StatusServiceUnavailable, true
	case strings.Contains(msg, " 504"), strings.Contains(msg, "(504)"), strings.Contains(msg, "returned 504"), strings.Contains(msg, "timeout"):
		return http.StatusGatewayTimeout, true
	default:
		return 0, false
	}
}

func isRetryableGenerationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "returned 429") ||
		strings.Contains(msg, "(429)") ||
		strings.Contains(msg, "proxy")
}

func isIntrinsicRetryableAsyncSubmissionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "context deadline exceeded")
}

func extractRetryCodeSource(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	codes := []string{}
	for _, code := range []string{"429", "451", "500", "502", "503", "504"} {
		if strings.Contains(raw, code) {
			codes = append(codes, code)
		}
	}

	keywords := []string{}
	for _, item := range []string{"timeout", "connection", "proxy", "insufficient_tokens", "insufficient tokens", "rate limit", "unexpected eof", "server_error"} {
		if strings.Contains(raw, item) {
			keywords = append(keywords, item)
		}
	}

	parts := []string{raw, normalizeRetryMatcher(raw)}
	if len(codes) > 0 {
		parts = append(parts, strings.Join(codes, " "))
	}
	if len(keywords) > 0 {
		parts = append(parts, strings.Join(keywords, " "))
	}
	return strings.Join(parts, " ")
}
