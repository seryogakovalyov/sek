package models

import (
	"slices"
	"testing"
)

func TestDefaultEngineeringModule(t *testing.T) {
	module := DefaultEngineeringModule()

	if module.Name != ModuleEngineering {
		t.Fatalf("Name = %q, want %q", module.Name, ModuleEngineering)
	}
	if module.Description == "" {
		t.Fatal("expected module description")
	}
	for _, level := range []KnowledgeLevel{LevelObservation, LevelLesson, LevelPattern} {
		if !slices.Contains(module.Levels, level) {
			t.Fatalf("expected level %q in default engineering module", level)
		}
	}
	for _, eventType := range []EventType{
		EventRequest,
		EventResponse,
		EventToolUsage,
		EventFailure,
		EventDecision,
		EventImplementationChoice,
		EventSuccessfulFix,
	} {
		if !slices.Contains(module.EventTypes, eventType) {
			t.Fatalf("expected event type %q in default engineering module", eventType)
		}
	}
}

func TestKnownModuleConstants(t *testing.T) {
	for _, module := range []string{
		ModuleEngineering,
		ModuleLocalAI,
		ModuleAgentBehavior,
		ModulePersonal,
		ModuleCompany,
	} {
		if module == "" {
			t.Fatal("module constant must not be empty")
		}
	}
}
