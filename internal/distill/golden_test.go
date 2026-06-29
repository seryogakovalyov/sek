package distill

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/seryogakovalyov/sek/internal/llm"
	"github.com/seryogakovalyov/sek/internal/models"
)

type goldenCase struct {
	Name                string `json:"name"`
	EventType           string `json:"event_type"`
	Source              string `json:"source"`
	Content             string `json:"content"`
	ExpectedObservation string `json:"expected_observation"`
}

type goldenProvider struct {
	t        *testing.T
	expected string
}

func (p goldenProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	p.t.Helper()

	if len(req.Messages) != 2 {
		p.t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}

	system := req.Messages[0].Content
	for _, required := range []string{
		"Preserve concrete technical details",
		"file paths, config keys/sections, commands, flags, tool names",
		"preserve the exact error text",
		"preserve the exact command",
		"Do not over-generalize",
	} {
		if !strings.Contains(system, required) {
			p.t.Fatalf("distillation system prompt missing %q:\n%s", required, system)
		}
	}

	user := req.Messages[1].Content
	if strings.Contains(user, "abcdefghijklmnop") {
		p.t.Fatalf("distillation prompt leaked bearer token:\n%s", user)
	}
	if strings.Contains(user, "Authorization: Bearer") && !strings.Contains(user, "Authorization: Bearer [REDACTED]") {
		p.t.Fatalf("distillation prompt did not preserve redacted bearer context:\n%s", user)
	}

	return &llm.ChatResponse{Content: p.expected, Model: req.Model}, nil
}

func TestDistillGoldenEvents(t *testing.T) {
	data, err := os.ReadFile("testdata/golden_events.json")
	if err != nil {
		t.Fatal(err)
	}

	var cases []goldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("expected at least one golden case")
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			p := NewPipeline(goldenProvider{t: t, expected: tc.ExpectedObservation}, nil, "golden-model", nil)
			event := models.Event{
				ID:        tc.Name,
				SessionID: "s",
				Timestamp: time.Now(),
				Type:      models.EventType(tc.EventType),
				Source:    tc.Source,
				Content:   tc.Content,
			}

			obs, err := p.distillEvent(context.Background(), event)
			if err != nil {
				t.Fatal(err)
			}
			if obs == nil {
				t.Fatal("expected observation")
			}
			if obs.Content != tc.ExpectedObservation {
				t.Fatalf("unexpected observation:\nwant: %s\n got: %s", tc.ExpectedObservation, obs.Content)
			}
			if obs.ID != "obs-"+tc.Name {
				t.Fatalf("unexpected observation ID: %s", obs.ID)
			}
			if len(obs.SourceIDs) != 1 || obs.SourceIDs[0] != tc.Name {
				t.Fatalf("unexpected source IDs: %#v", obs.SourceIDs)
			}
			if strings.Contains(obs.Content, "abcdefghijklmnop") {
				t.Fatalf("observation leaked bearer token: %s", obs.Content)
			}
		})
	}
}
