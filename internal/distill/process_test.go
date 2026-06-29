package distill

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/seryogakovalyov/sek/internal/llm"
	"github.com/seryogakovalyov/sek/internal/models"
)

type sequenceProvider struct {
	responses []string
	calls     int
}

func (p *sequenceProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if p.calls >= len(p.responses) {
		return nil, errors.New("unexpected chat call")
	}
	response := p.responses[p.calls]
	p.calls++
	return &llm.ChatResponse{Content: response, Model: req.Model}, nil
}

type failingEmbedder struct{}

func (failingEmbedder) Embed(ctx context.Context, input []string) ([][]float32, error) {
	return nil, errors.New("embedding unavailable")
}

func TestProcessShadowModuleRouteDoesNotBlockObservationSave(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	provider := &sequenceProvider{
		responses: []string{
			"Use --global for machine-wide memory.",
			"not json",
		},
	}
	p := NewPipeline(provider, failingEmbedder{}, "test-model", s)

	err := p.Process(context.Background(), []models.Event{
		{
			ID:        "event-1",
			SessionID: "session-1",
			Timestamp: time.Now(),
			Type:      models.EventDecision,
			Source:    "test",
			Content:   "Use --global for machine-wide memory.",
		},
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("expected distill and route calls, got %d", provider.calls)
	}

	stats, err := s.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.KnowledgeCount != 1 {
		t.Fatalf("expected observation to be saved despite routing failure, got %d", stats.KnowledgeCount)
	}
}

func TestProcessLogsShadowModuleRouteTelemetry(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	provider := &sequenceProvider{
		responses: []string{
			"llama.cpp embeddings require --embeddings --pooling mean.",
			`{"module":"local-ai","confidence":0.95,"reason":"local embeddings setup"}`,
		},
	}
	p := NewPipeline(provider, failingEmbedder{}, "test-model", s)

	err := p.Process(context.Background(), []models.Event{
		{
			ID:        "event-1",
			SessionID: "session-1",
			Timestamp: time.Now(),
			Type:      models.EventDecision,
			Source:    "test",
			Content:   "llama.cpp embeddings require --embeddings --pooling mean.",
		},
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}

	routes, err := s.ListModuleRoutes(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 module route log, got %d", len(routes))
	}
	if routes[0].KnowledgeID != "obs-event-1" {
		t.Fatalf("KnowledgeID = %q", routes[0].KnowledgeID)
	}
	if routes[0].Module != models.ModuleLocalAI {
		t.Fatalf("Module = %q, want %q", routes[0].Module, models.ModuleLocalAI)
	}
	if routes[0].Confidence != 0.95 {
		t.Fatalf("Confidence = %.2f, want 0.95", routes[0].Confidence)
	}
	if routes[0].Reason != "local embeddings setup" {
		t.Fatalf("Reason = %q", routes[0].Reason)
	}
}
