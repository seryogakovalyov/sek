package trace

import (
	"strings"
	"testing"
	"time"

	"github.com/seryogakovalyov/sek/internal/models"
)

func TestFormatKnowledgeIncludesSourceTraceAndWhy(t *testing.T) {
	k := models.Knowledge{
		ID:        "lesson-1",
		Level:     models.LevelLesson,
		CreatedAt: time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
		Content:   "Use anchored .gitignore patterns.",
		SourceIDs: []string{"obs-1", "obs-2"},
		Score:     1.234,
		Breakdown: models.ScoreBreakdown{
			VectorScore:     0.7,
			KeywordScore:    0.4,
			BaseScore:       0.8,
			RecencyBoost:    0.1,
			ImportanceBoost: 0.2,
			UsageBoost:      0.05,
			FinalScore:      1.234,
			MatchTypes:      []string{"vector", "keyword"},
		},
		EventType:  models.EventSuccessfulFix,
		Importance: models.ImportanceHigh,
	}

	got := FormatKnowledge(k, true)

	for _, want := range []string{
		"[lesson] 2026-06-28 (score: 1.234)",
		"id: lesson-1",
		"Use anchored .gitignore patterns.",
		"Trace:",
		"- source_ids: obs-1, obs-2",
		"- why: retrieval score 1.234 after similarity, recency, and importance adjustments",
		"- score_breakdown: vector=0.700 keyword=0.400 base=0.800 recency=0.100 importance=0.200 usage=0.050 final=1.234 matches=vector+keyword",
		"- event_type: successful_fix",
		"- importance: 0.80",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted output missing %q:\n%s", want, got)
		}
	}
}

func TestFormatKnowledgeCanOmitTrace(t *testing.T) {
	k := models.Knowledge{
		ID:        "obs-1",
		Level:     models.LevelObservation,
		CreatedAt: time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
		Content:   "Observation.",
		SourceIDs: []string{"event-1"},
		Score:     1.234,
	}

	got := FormatKnowledge(k, false)
	for _, unwanted := range []string{"score:", "Trace:", "- why:", "- source_ids:", "- event_type:", "- importance:"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("expected compact output without %q:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, "Observation.") {
		t.Fatalf("expected content in compact output:\n%s", got)
	}
}
