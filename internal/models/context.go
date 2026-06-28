package models

type ContextBudget struct {
	MaxTokens  int
	MaxEntries int
}

type ReuseRequest struct {
	Task      string   `json:"task"`
	OpenFiles []string `json:"open_files,omitempty"`
	Budget    ContextBudget
}

type ReuseResult struct {
	Knowledge   []Knowledge `json:"knowledge"`
	TotalTokens int         `json:"total_tokens"`
}
