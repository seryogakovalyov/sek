package reuse

import (
	"testing"
	"time"

	"github.com/seryogakovalyov/sek/internal/models"
)

func TestRecencyFactor(t *testing.T) {
	now := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		created  time.Time
		wantHigh bool
	}{
		{"just now", now, true},
		{"1 day ago", now.Add(-24 * time.Hour), true},
		{"15 days ago", now.Add(-15 * 24 * time.Hour), true},
		{"60 days ago", now.Add(-60 * 24 * time.Hour), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boost := recencyFactor(tt.created, now)
			if tt.wantHigh && boost < 0.05 {
				t.Errorf("expected high boost, got %f", boost)
			}
			if !tt.wantHigh && boost > 0.05 {
				t.Errorf("expected low boost, got %f", boost)
			}
		})
	}
}

func TestApplyScoreAdjustments(t *testing.T) {
	now := time.Now()

	knowledge := []models.Knowledge{
		{
			ID:         "old-low",
			Content:    "old low importance",
			Score:      0.8,
			CreatedAt:  now.Add(-60 * 24 * time.Hour),
			Importance: models.ImportanceLow,
		},
		{
			ID:         "new-high",
			Content:    "new high importance",
			Score:      0.7,
			CreatedAt:  now,
			Importance: models.ImportanceHigh,
		},
		{
			ID:         "mid",
			Content:    "mid recency and importance",
			Score:      0.75,
			CreatedAt:  now.Add(-10 * 24 * time.Hour),
			Importance: models.ImportanceNormal,
		},
	}

	adjusted := applyScoreAdjustments(knowledge)

	if len(adjusted) != 3 {
		t.Fatalf("expected 3, got %d", len(adjusted))
	}

	if adjusted[0].ID != "new-high" {
		t.Fatalf("expected new-high as top result, got %s", adjusted[0].ID)
	}
}

func TestApplyScoreAdjustmentsBoostsUsedKnowledge(t *testing.T) {
	now := time.Now()
	knowledge := []models.Knowledge{
		{
			ID:         "unused",
			Content:    "unused",
			Score:      0.8,
			CreatedAt:  now,
			Importance: models.ImportanceNormal,
		},
		{
			ID:         "used",
			Content:    "used",
			Score:      0.8,
			CreatedAt:  now,
			Importance: models.ImportanceNormal,
			UsageCount: 3,
		},
	}

	adjusted := applyScoreAdjustments(knowledge)

	if adjusted[0].ID != "used" {
		t.Fatalf("expected used knowledge first, got %s", adjusted[0].ID)
	}
	if adjusted[0].Score <= adjusted[1].Score {
		t.Fatalf("expected usage boost to increase score: used=%f unused=%f", adjusted[0].Score, adjusted[1].Score)
	}
}
