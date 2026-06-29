package trace

import (
	"fmt"
	"strings"

	"github.com/seryogakovalyov/sek/internal/models"
)

func FormatKnowledge(k models.Knowledge, includeTrace bool) string {
	header := fmt.Sprintf("[%s] %s", k.Level, k.CreatedAt.Format("2006-01-02"))
	if includeTrace && k.Score > 0 {
		header += fmt.Sprintf(" (score: %.3f)", k.Score)
	}

	lines := []string{header, "---", "id: " + k.ID, k.Content}

	traceLines := SourceTrace(k, includeTrace)
	if len(traceLines) > 0 {
		lines = append(lines, "Trace:")
		lines = append(lines, traceLines...)
	}

	return strings.Join(lines, "\n")
}

func SourceTrace(k models.Knowledge, includeScore bool) []string {
	var lines []string
	if !includeScore {
		return lines
	}
	if len(k.SourceIDs) > 0 {
		lines = append(lines, "- source_ids: "+strings.Join(k.SourceIDs, ", "))
	}
	if k.Score > 0 {
		lines = append(lines, fmt.Sprintf("- why: retrieval score %.3f after similarity, recency, and importance adjustments", k.Score))
	}
	if k.Breakdown.FinalScore > 0 {
		lines = append(lines, fmt.Sprintf("- score_breakdown: vector=%.3f keyword=%.3f base=%.3f recency=%.3f importance=%.3f usage=%.3f final=%.3f matches=%s",
			k.Breakdown.VectorScore,
			k.Breakdown.KeywordScore,
			k.Breakdown.BaseScore,
			k.Breakdown.RecencyBoost,
			k.Breakdown.ImportanceBoost,
			k.Breakdown.UsageBoost,
			k.Breakdown.FinalScore,
			strings.Join(k.Breakdown.MatchTypes, "+"),
		))
	}
	if k.EventType != "" {
		lines = append(lines, "- event_type: "+string(k.EventType))
	}
	if k.Importance > 0 {
		lines = append(lines, fmt.Sprintf("- importance: %.2f", k.Importance))
	}
	return lines
}
