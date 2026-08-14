package leonardo

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// ImageGenerateRequest is the input for a Leonardo still-image generation.
type ImageGenerateRequest struct {
	RequestModel string // Leonardo request.model slug; e.g. gpt-image-2 / nano-banana-2
	ModelID      string // Leonardo custom model UUID, used by legacy/custom-model routes
	Prompt       string
	Quality      string // server-selected: low / medium / high
	Width        int
	Height       int
	Quantity     int
	InitImageIDs []string
	Public       bool

	// NativeModelRequest matches the new Leonardo capture shape:
	// request.model carries the model slug, parameters omit modelId/dimensions/negative_prompt.
	NativeModelRequest bool
	PromptEnhance      string   // AUTO / ON / OFF
	StyleIDs           []string // optional style ids for native image routes
}

// ImageGenerateResponse is the response from the Generate mutation for images.
type ImageGenerateResponse struct {
	GenerationID  string
	APICreditCost int
}

// GenerateImage submits a still-image generation request.
func (c *Client) GenerateImage(session *TokenSession, imgReq *ImageGenerateRequest) (*ImageGenerateResponse, error) {
	if imgReq == nil {
		return nil, fmt.Errorf("image generation request is required")
	}
	if err := c.EnsureValidJWT(session); err != nil {
		return nil, fmt.Errorf("ensure JWT: %w", err)
	}

	session.mu.RLock()
	jwt := session.JWT
	session.mu.RUnlock()

	requestModel := strings.TrimSpace(imgReq.RequestModel)
	if requestModel == "" {
		requestModel = "nano-banana-2"
	}
	modelID := strings.TrimSpace(imgReq.ModelID)
	if modelID == "" && !imgReq.NativeModelRequest {
		return nil, fmt.Errorf("image model id is required")
	}
	quantity := imgReq.Quantity
	if quantity < 1 {
		quantity = 1
	}
	if quantity > 4 {
		quantity = 4
	}
	width := imgReq.Width
	height := imgReq.Height
	if width <= 0 || height <= 0 {
		width, height = 1536, 1536
	}

	params := map[string]interface{}{
		"width":    width,
		"height":   height,
		"prompt":   strings.TrimSpace(imgReq.Prompt),
		"quantity": quantity,
	}
	if quality := strings.TrimSpace(imgReq.Quality); quality != "" {
		if imgReq.NativeModelRequest {
			quality = strings.ToUpper(quality)
		}
		params["quality"] = quality
	}
	if promptEnhance := strings.TrimSpace(imgReq.PromptEnhance); promptEnhance != "" {
		params["prompt_enhance"] = strings.ToUpper(promptEnhance)
	}
	if len(imgReq.StyleIDs) > 0 {
		styleIDs := make([]string, 0, len(imgReq.StyleIDs))
		for _, styleID := range imgReq.StyleIDs {
			styleID = strings.TrimSpace(styleID)
			if styleID != "" {
				styleIDs = append(styleIDs, styleID)
			}
		}
		if len(styleIDs) > 0 {
			params["style_ids"] = styleIDs
		}
	}
	if !imgReq.NativeModelRequest {
		params["dimensions"] = fmt.Sprintf("%dx%d", width, height)
		params["modelId"] = modelID
		params["negative_prompt"] = ""
	}

	if len(imgReq.InitImageIDs) > 0 {
		refs := make([]map[string]interface{}, 0, len(imgReq.InitImageIDs))
		for _, id := range imgReq.InitImageIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			refs = append(refs, map[string]interface{}{
				"image":    map[string]interface{}{"id": id, "type": "UPLOADED"},
				"strength": "MID",
			})
		}
		if len(refs) > 0 {
			params["guidances"] = map[string]interface{}{"image_reference": refs}
		}
	}

	gqlReq := graphqlRequest{
		OperationName: "Generate",
		Variables: map[string]interface{}{
			"request": map[string]interface{}{
				"model":      requestModel,
				"public":     imgReq.Public,
				"parameters": params,
			},
		},
		Query: generateMutation,
	}

	body, err := c.doGraphQL(jwt, gqlReq)
	if err != nil {
		return nil, err
	}

	var gqlResp struct {
		Data struct {
			Generate struct {
				APICreditCost int    `json:"apiCreditCost"`
				GenerationID  string `json:"generationId"`
			} `json:"generate"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, fmt.Errorf("parse image generate response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("image generate error: %s", gqlResp.Errors[0].Message)
	}
	if strings.TrimSpace(gqlResp.Data.Generate.GenerationID) == "" {
		return nil, fmt.Errorf("image generate failed: empty generation id")
	}

	log.Printf("[Leonardo] Image generation submitted: id=%s model=%s quality=%s cost=%d",
		gqlResp.Data.Generate.GenerationID, modelID, imgReq.Quality, gqlResp.Data.Generate.APICreditCost)

	return &ImageGenerateResponse{
		GenerationID:  gqlResp.Data.Generate.GenerationID,
		APICreditCost: gqlResp.Data.Generate.APICreditCost,
	}, nil
}
