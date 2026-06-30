package llm

import "fmt"

type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderAnthropic ProviderType = "anthropic"
)

type Config struct {
	Provider ProviderType `json:"provider"`
	APIKey   string       `json:"api_key"`
	BaseURL  string       `json:"base_url,omitempty"`
	Model    string       `json:"model"`
}

func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "":
		return NewOpenAIProvider(cfg.APIKey, cfg.BaseURL), nil
	case ProviderOpenAI:
		return NewOpenAIProvider(cfg.APIKey, cfg.BaseURL), nil
	case ProviderAnthropic:
		return NewAnthropicProvider(cfg.APIKey, cfg.BaseURL), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}

func ResolveAPIKey(cfg *Config, envKey string) error {
	if cfg.APIKey == "" {
		cfg.APIKey = envKey
	}
	if cfg.APIKey != "" {
		return nil
	}
	if (cfg.Provider == "" || cfg.Provider == ProviderOpenAI) && cfg.BaseURL != "" {
		cfg.APIKey = "none"
		return nil
	}
	return fmt.Errorf("LLM API key required: set --llm-key or SEK_LLM_KEY")
}
