package trace

import (
	"fmt"
	"strings"

	"github.com/anomalyco/sek/internal/models"
)

func FormatKnowledge(k models.Knowledge, includeScore bool) string {
	header := fmt.Sprintf("[%s] %s", k.Level, k.CreatedAt.Format("2006-01-02"))
	if includeScore && k.Score > 0 {
		header += fmt.Sprintf(" (score: %.3f)", k.Score)
	}

	lines := []string{header, "---", "id: " + k.ID, k.Content}

	traceLines := SourceTrace(k, includeScore)
	if len(traceLines) > 0 {
		lines = append(lines, "Trace:")
		lines = append(lines, traceLines...)
	}

	return strings.Join(lines, "\n")
}

func SourceTrace(k models.Knowledge, includeScore bool) []string {
	var lines []string
	if len(k.SourceIDs) > 0 {
		lines = append(lines, "- source_ids: "+strings.Join(k.SourceIDs, ", "))
	}
	if includeScore && k.Score > 0 {
		lines = append(lines, fmt.Sprintf("- why: retrieval score %.3f after similarity, recency, and importance adjustments", k.Score))
	}
	if k.EventType != "" {
		lines = append(lines, "- event_type: "+string(k.EventType))
	}
	if k.Importance > 0 {
		lines = append(lines, fmt.Sprintf("- importance: %.2f", k.Importance))
	}
	return lines
}
