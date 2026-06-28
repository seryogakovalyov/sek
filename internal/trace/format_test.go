package trace

import (
	"strings"
	"testing"
	"time"

	"github.com/anomalyco/sek/internal/models"
)

func TestFormatKnowledgeIncludesSourceTraceAndWhy(t *testing.T) {
	k := models.Knowledge{
		Level:      models.LevelLesson,
		CreatedAt:  time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
		Content:    "Use anchored .gitignore patterns.",
		SourceIDs:  []string{"obs-1", "obs-2"},
		Score:      1.234,
		EventType:  models.EventSuccessfulFix,
		Importance: models.ImportanceHigh,
	}

	got := FormatKnowledge(k, true)

	for _, want := range []string{
		"[lesson] 2026-06-28 (score: 1.234)",
		"Use anchored .gitignore patterns.",
		"Trace:",
		"- source_ids: obs-1, obs-2",
		"- why: retrieval score 1.234 after similarity, recency, and importance adjustments",
		"- event_type: successful_fix",
		"- importance: 0.80",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted output missing %q:\n%s", want, got)
		}
	}
}

func TestFormatKnowledgeCanOmitScoreReason(t *testing.T) {
	k := models.Knowledge{
		Level:     models.LevelObservation,
		CreatedAt: time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
		Content:   "Observation.",
		SourceIDs: []string{"event-1"},
		Score:     1.234,
	}

	got := FormatKnowledge(k, false)
	if strings.Contains(got, "score:") || strings.Contains(got, "- why:") {
		t.Fatalf("expected no score details:\n%s", got)
	}
	if !strings.Contains(got, "- source_ids: event-1") {
		t.Fatalf("expected source trace:\n%s", got)
	}
}
