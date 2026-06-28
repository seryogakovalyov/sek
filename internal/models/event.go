package models

import "time"

type EventType string

const (
	EventRequest              EventType = "request"
	EventResponse             EventType = "response"
	EventToolUsage            EventType = "tool_usage"
	EventFailure              EventType = "failure"
	EventDecision             EventType = "decision"
	EventImplementationChoice EventType = "implementation_choice"
	EventSuccessfulFix        EventType = "successful_fix"
)

type Event struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"session_id"`
	ServerSession string    `json:"-"`
	Timestamp     time.Time `json:"timestamp"`
	Type          EventType `json:"type"`
	Source        string    `json:"source"`
	Content       string    `json:"content"`
}
