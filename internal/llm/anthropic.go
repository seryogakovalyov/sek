package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type AnthropicProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Model string `json:"model"`
}

func NewAnthropicProvider(apiKey, baseURL string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	return &AnthropicProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	var system string
	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			system = m.Content
		} else {
			messages = append(messages, anthropicMessage{Role: string(m.Role), Content: m.Content})
		}
	}

	body := anthropicRequest{
		Model:       req.Model,
		Messages:    messages,
		System:      system,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	if body.MaxTokens == 0 {
		body.MaxTokens = 1024
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(p.baseURL, "/") + "/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var anthropicResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	return &ChatResponse{
		Content: anthropicResp.Content[0].Text,
		Model:   anthropicResp.Model,
	}, nil
}
