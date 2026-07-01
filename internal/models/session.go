package models

import "time"

type GitSnapshot struct {
	CapturedAt   time.Time `json:"captured_at"`
	Available    bool      `json:"available"`
	RepoRoot     string    `json:"repo_root,omitempty"`
	Head         string    `json:"head,omitempty"`
	Dirty        bool      `json:"dirty"`
	Status       string    `json:"status,omitempty"`
	ChangedFiles []string  `json:"changed_files,omitempty"`
	DiffStat     string    `json:"diff_stat,omitempty"`
	Error        string    `json:"error,omitempty"`
}

type SessionLog struct {
	ID            string       `json:"id"`
	StartedAt     time.Time    `json:"started_at"`
	EndedAt       time.Time    `json:"ended_at,omitempty"`
	ProjectDir    string       `json:"project_dir"`
	Status        string       `json:"status"`
	StartSnapshot *GitSnapshot `json:"start_snapshot,omitempty"`
	EndSnapshot   *GitSnapshot `json:"end_snapshot,omitempty"`
}
