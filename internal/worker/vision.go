package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VisionClient describes images via an OpenAI-compatible multimodal chat API.
type VisionClient struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func NewVisionClient(baseURL, apiKey, model string) *VisionClient {
	return &VisionClient{
		BaseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (v *VisionClient) Enabled() bool {
	return v != nil && strings.TrimSpace(v.BaseURL) != ""
}

func (v *VisionClient) Describe(ctx context.Context, mimeType string, data []byte) (string, error) {
	if !v.Enabled() {
		return "", fmt.Errorf("vision disabled")
	}
	endpoint := v.BaseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint = strings.TrimRight(endpoint, "/") + "/chat/completions"
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	if mimeType == "" {
		mimeType = "image/png"
	}
	dataURL := "data:" + mimeType + ";base64," + b64
	body := map[string]any{
		"model": v.Model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": "用中文简要描述这张图片中的关键信息，便于以后检索。不要编造看不清的文字。"},
					{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
				},
			},
		},
		"temperature": 0,
		"max_tokens":  400,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if v.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+v.APIKey)
	}
	res, err := v.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		n := len(respBody)
		if n > 200 {
			n = 200
		}
		return "", fmt.Errorf("vision http %s: %s", res.Status, string(respBody[:n]))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("empty vision choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
