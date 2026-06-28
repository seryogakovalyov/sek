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
