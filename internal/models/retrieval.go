package models

import "time"

type RetrievalLog struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Task      string    `json:"task"`
	Results   string    `json:"results"`
	UsedIDs   string    `json:"used_ids"`
}

type UsageSummary struct {
	Retrievals       int `json:"retrievals"`
	UsedRetrievals   int `json:"used_retrievals"`
	UsedMarks        int `json:"used_marks"`
	KnowledgeWithUse int `json:"knowledge_with_use"`
	TotalUsageCount  int `json:"total_usage_count"`
}

type SessionUsage struct {
	SessionID      string    `json:"session_id"`
	Retrievals     int       `json:"retrievals"`
	UsedRetrievals int       `json:"used_retrievals"`
	UsedMarks      int       `json:"used_marks"`
	LastSeen       time.Time `json:"last_seen"`
}
