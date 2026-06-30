package llm

import "testing"

func TestResolveAPIKeyUsesExplicitKey(t *testing.T) {
	cfg := Config{Provider: ProviderOpenAI, APIKey: "explicit"}

	if err := ResolveAPIKey(&cfg, "env"); err != nil {
		t.Fatalf("ResolveAPIKey returned error: %v", err)
	}
	if cfg.APIKey != "explicit" {
		t.Fatalf("APIKey = %q, want explicit", cfg.APIKey)
	}
}

func TestResolveAPIKeyUsesEnvKey(t *testing.T) {
	cfg := Config{Provider: ProviderOpenAI}

	if err := ResolveAPIKey(&cfg, "env"); err != nil {
		t.Fatalf("ResolveAPIKey returned error: %v", err)
	}
	if cfg.APIKey != "env" {
		t.Fatalf("APIKey = %q, want env", cfg.APIKey)
	}
}

func TestResolveAPIKeyAllowsLocalOpenAICompatibleEndpoint(t *testing.T) {
	cfg := Config{Provider: ProviderOpenAI, BaseURL: "http://localhost:8000/v1"}

	if err := ResolveAPIKey(&cfg, ""); err != nil {
		t.Fatalf("ResolveAPIKey returned error: %v", err)
	}
	if cfg.APIKey != "none" {
		t.Fatalf("APIKey = %q, want none", cfg.APIKey)
	}
}

func TestResolveAPIKeyRequiresKeyForOpenAIWithoutBaseURL(t *testing.T) {
	cfg := Config{Provider: ProviderOpenAI}

	if err := ResolveAPIKey(&cfg, ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveAPIKeyRequiresKeyForAnthropic(t *testing.T) {
	cfg := Config{Provider: ProviderAnthropic, BaseURL: "http://localhost:8000/v1"}

	if err := ResolveAPIKey(&cfg, ""); err == nil {
		t.Fatal("expected error")
	}
}
