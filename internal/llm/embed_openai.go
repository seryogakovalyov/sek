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

type OpenAIEmbedder struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
}

func NewOpenAIEmbedder(apiKey, baseURL, model string) *OpenAIEmbedder {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIEmbedder{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{},
	}
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, input []string) ([][]float32, error) {
	body := openAIEmbedRequest{
		Model: e.model,
		Input: input,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	url := strings.TrimRight(e.baseURL, "/") + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var embedResp openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	result := make([][]float32, len(embedResp.Data))
	for _, d := range embedResp.Data {
		if d.Index >= 0 && d.Index < len(result) {
			result[d.Index] = d.Embedding
		}
	}

	return result, nil
}
