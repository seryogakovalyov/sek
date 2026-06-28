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

type OpenAIProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

type openAIRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
}

func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	messages := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = openAIMessage{Role: string(m.Role), Content: m.Content}
	}

	body := openAIRequest{
		Model:       req.Model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(p.baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var openAIResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &ChatResponse{
		Content: openAIResp.Choices[0].Message.Content,
		Model:   openAIResp.Model,
	}, nil
}
