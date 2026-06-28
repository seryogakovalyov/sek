package models

import "time"

type KnowledgeLevel string

const (
	LevelObservation KnowledgeLevel = "observation"
	LevelLesson      KnowledgeLevel = "lesson"
	LevelPattern     KnowledgeLevel = "pattern"
)

type Importance float64

const (
	ImportanceLow      Importance = 0.2
	ImportanceNormal   Importance = 0.5
	ImportanceHigh     Importance = 0.8
	ImportanceCritical Importance = 1.0
)

type Knowledge struct {
	ID         string         `json:"id"`
	Level      KnowledgeLevel `json:"level"`
	CreatedAt  time.Time      `json:"created_at"`
	Content    string         `json:"content"`
	SourceIDs  []string       `json:"source_ids,omitempty"`
	Embedding  []float32      `json:"-"`
	Score      float64        `json:"score,omitempty"`
	EventType  EventType      `json:"event_type,omitempty"`
	Importance Importance     `json:"importance,omitempty"`
	UsageCount int            `json:"usage_count,omitempty"`
}

func EventImportance(et EventType) Importance {
	switch et {
	case EventFailure, EventSuccessfulFix:
		return ImportanceHigh
	case EventDecision, EventImplementationChoice:
		return 0.65
	case EventToolUsage:
		return ImportanceNormal
	default:
		return ImportanceLow
	}
}
