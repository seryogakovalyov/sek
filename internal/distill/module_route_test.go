package distill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/seryogakovalyov/sek/internal/llm"
	"github.com/seryogakovalyov/sek/internal/models"
)

type moduleRouteGoldenCase struct {
	Name           string  `json:"name"`
	Observation    string  `json:"observation"`
	ExpectedModule string  `json:"expected_module"`
	Confidence     float64 `json:"confidence"`
	Reason         string  `json:"reason"`
}

type moduleRouteProvider struct {
	t        *testing.T
	expected moduleRouteGoldenCase
}

func (p moduleRouteProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	p.t.Helper()

	if len(req.Messages) != 2 {
		p.t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}

	system := req.Messages[0].Content
	for _, required := range []string{
		"Classify this distilled observation into one SEK memory module",
		"Classify by the meaning of the observation, not by source/channel",
		"If confidence is low, choose engineering",
		"Do not invent modules",
		`"module":"engineering"`,
	} {
		if !strings.Contains(system, required) {
			p.t.Fatalf("module routing prompt missing %q:\n%s", required, system)
		}
	}

	if req.Messages[1].Content != p.expected.Observation {
		p.t.Fatalf("unexpected observation prompt:\nwant: %s\n got: %s", p.expected.Observation, req.Messages[1].Content)
	}

	return &llm.ChatResponse{
		Content: fmt.Sprintf(`{"module":%q,"confidence":%.2f,"reason":%q}`, p.expected.ExpectedModule, p.expected.Confidence, p.expected.Reason),
		Model:   req.Model,
	}, nil
}

func TestModuleRouteGoldenCases(t *testing.T) {
	cases := loadModuleRouteGoldenCases(t)

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			p := NewPipeline(moduleRouteProvider{t: t, expected: tc}, nil, "golden-model", nil)

			route, err := p.routeModule(context.Background(), tc.Observation)
			if err != nil {
				t.Fatal(err)
			}
			if route.Module != tc.ExpectedModule {
				t.Fatalf("Module = %q, want %q", route.Module, tc.ExpectedModule)
			}
			if route.Confidence != tc.Confidence {
				t.Fatalf("Confidence = %.2f, want %.2f", route.Confidence, tc.Confidence)
			}
			if route.Reason != tc.Reason {
				t.Fatalf("Reason = %q, want %q", route.Reason, tc.Reason)
			}
		})
	}
}

func TestModuleRouteGoldenCasesWithRealModel(t *testing.T) {
	if os.Getenv("SEK_MODULE_ROUTE_EVAL") != "1" {
		t.Skip("set SEK_MODULE_ROUTE_EVAL=1 to run real model routing evals")
	}

	provider, err := llm.NewProvider(llm.Config{
		Provider: llm.ProviderType(envOrDefault("SEK_LLM_PROVIDER", "openai")),
		APIKey:   envOrDefault("SEK_LLM_KEY", "none"),
		BaseURL:  os.Getenv("SEK_LLM_BASE_URL"),
		Model:    envOrDefault("SEK_LLM_MODEL", "gpt-4o"),
	})
	if err != nil {
		t.Fatal(err)
	}

	p := NewPipeline(provider, nil, envOrDefault("SEK_LLM_MODEL", "gpt-4o"), nil)
	for _, tc := range loadModuleRouteGoldenCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			route, err := p.routeModule(context.Background(), tc.Observation)
			if err != nil {
				t.Fatal(err)
			}
			if route.Module != tc.ExpectedModule {
				t.Fatalf("Module = %q, want %q; reason=%q confidence=%.2f", route.Module, tc.ExpectedModule, route.Reason, route.Confidence)
			}
		})
	}
}

func TestModuleRouteFallsBackToEngineeringForUnknownModule(t *testing.T) {
	p := NewPipeline(staticRouteProvider{content: `{"module":"sales","confidence":0.99,"reason":"bad module"}`}, nil, "golden-model", nil)

	route, err := p.routeModule(context.Background(), "Some observation.")
	if err != nil {
		t.Fatal(err)
	}
	if route.Module != models.ModuleEngineering {
		t.Fatalf("Module = %q, want %q", route.Module, models.ModuleEngineering)
	}
	if route.Confidence != 0 {
		t.Fatalf("Confidence = %.2f, want 0", route.Confidence)
	}
}

func TestModuleRouteFallsBackToEngineeringForLowConfidence(t *testing.T) {
	p := NewPipeline(staticRouteProvider{content: `{"module":"personal","confidence":0.10,"reason":"weak signal"}`}, nil, "golden-model", nil)

	route, err := p.routeModule(context.Background(), "Some observation.")
	if err != nil {
		t.Fatal(err)
	}
	if route.Module != models.ModuleEngineering {
		t.Fatalf("Module = %q, want %q", route.Module, models.ModuleEngineering)
	}
	if route.Reason != "low-confidence module route; fell back to engineering" {
		t.Fatalf("Reason = %q", route.Reason)
	}
}

type staticRouteProvider struct {
	content string
}

func (p staticRouteProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: p.content, Model: req.Model}, nil
}

func loadModuleRouteGoldenCases(t *testing.T) []moduleRouteGoldenCase {
	t.Helper()

	data, err := os.ReadFile("testdata/golden_module_routing.json")
	if err != nil {
		t.Fatal(err)
	}

	var cases []moduleRouteGoldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("expected at least one golden case")
	}
	return cases
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
