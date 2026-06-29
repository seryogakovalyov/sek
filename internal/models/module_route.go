package models

import "time"

type ModuleRouteLog struct {
	ID          string    `json:"id"`
	KnowledgeID string    `json:"knowledge_id"`
	Timestamp   time.Time `json:"timestamp"`
	Module      string    `json:"module"`
	Confidence  float64   `json:"confidence,omitempty"`
	Reason      string    `json:"reason,omitempty"`
}
