package llm

import "context"

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

type Message struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type ChatResponse struct {
	Content string `json:"content"`
	Model   string `json:"model"`
}

type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
