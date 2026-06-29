package models

const (
	ModuleEngineering   = "engineering"
	ModuleLocalAI       = "local-ai"
	ModuleAgentBehavior = "agent-behavior"
	ModulePersonal      = "personal"
	ModuleCompany       = "company"
)

type Module struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Levels      []KnowledgeLevel `json:"levels,omitempty"`
	EventTypes  []EventType      `json:"event_types,omitempty"`
}

func DefaultEngineeringModule() Module {
	return Module{
		Name:        ModuleEngineering,
		Description: "Reusable engineering experience for AI coding agents.",
		Levels: []KnowledgeLevel{
			LevelObservation,
			LevelLesson,
			LevelPattern,
		},
		EventTypes: []EventType{
			EventRequest,
			EventResponse,
			EventToolUsage,
			EventFailure,
			EventDecision,
			EventImplementationChoice,
			EventSuccessfulFix,
		},
	}
}

type ModuleRoute struct {
	Module     string  `json:"module"`
	Confidence float64 `json:"confidence,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}
