package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/seryogakovalyov/sek/internal/models"
)

func TestRunTopLevelHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h", "help"} {
		t.Run(arg, func(t *testing.T) {
			var stderr bytes.Buffer

			if code := run([]string{arg}, &stderr); code != 0 {
				t.Fatalf("run(%q) exit code = %d, want 0", arg, code)
			}

			output := stderr.String()
			if !strings.Contains(output, "Usage: sekctl <command> [flags]") {
				t.Fatalf("help output missing usage:\n%s", output)
			}
			if strings.Contains(output, "unknown command") {
				t.Fatalf("help output should not report unknown command:\n%s", output)
			}
		})
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer

	if code := run([]string{"nope"}, &stderr); code != 1 {
		t.Fatalf("run unknown command exit code = %d, want 1", code)
	}

	output := stderr.String()
	if !strings.Contains(output, "unknown command: nope") {
		t.Fatalf("unknown command output missing error:\n%s", output)
	}
	if !strings.Contains(output, "Usage: sekctl <command> [flags]") {
		t.Fatalf("unknown command output missing usage:\n%s", output)
	}
}

func TestSplitArgsKeepsFlagValues(t *testing.T) {
	task, flags := splitArgs([]string{
		"--llm-base-url", "http://localhost:8000/v1",
		"--max-entries", "5",
		"how", "to", "query",
	})

	if task != "how to query" {
		t.Fatalf("task = %q, want %q", task, "how to query")
	}

	want := []string{
		"--llm-base-url", "http://localhost:8000/v1",
		"--max-entries", "5",
	}
	if len(flags) != len(want) {
		t.Fatalf("flags = %#v, want %#v", flags, want)
	}
	for i := range want {
		if flags[i] != want[i] {
			t.Fatalf("flags = %#v, want %#v", flags, want)
		}
	}
}

func TestSplitArgsKeepsEqualsFlags(t *testing.T) {
	task, flags := splitArgs([]string{
		"--llm-base-url=http://localhost:8000/v1",
		"--trace",
		"how", "to", "query",
	})

	if task != "how to query" {
		t.Fatalf("task = %q, want %q", task, "how to query")
	}

	want := []string{
		"--llm-base-url=http://localhost:8000/v1",
		"--trace",
	}
	if len(flags) != len(want) {
		t.Fatalf("flags = %#v, want %#v", flags, want)
	}
	for i := range want {
		if flags[i] != want[i] {
			t.Fatalf("flags = %#v, want %#v", flags, want)
		}
	}
}

func TestFilterKnowledgeBySourcesFollowsDerivedKnowledge(t *testing.T) {
	knowledge := []models.Knowledge{
		{ID: "obs-1", Level: models.LevelObservation, SourceIDs: []string{"event-1"}},
		{ID: "lesson-1", Level: models.LevelLesson, SourceIDs: []string{"obs-1"}},
		{ID: "pattern-1", Level: models.LevelPattern, SourceIDs: []string{"lesson-1"}},
		{ID: "obs-other", Level: models.LevelObservation, SourceIDs: []string{"event-other"}},
	}

	got := filterKnowledgeBySources(knowledge, map[string]bool{"event-1": true})
	if len(got) != 3 {
		t.Fatalf("filtered knowledge count = %d, want 3: %#v", len(got), got)
	}
	if got[0].ID != "obs-1" || got[1].ID != "lesson-1" || got[2].ID != "pattern-1" {
		t.Fatalf("unexpected filtered knowledge: %#v", got)
	}
}

func TestFilterByTime(t *testing.T) {
	start := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

	events := filterEventsByTime([]models.Event{
		{ID: "before", Timestamp: start.Add(-time.Minute)},
		{ID: "inside", Timestamp: start.Add(time.Minute)},
		{ID: "after", Timestamp: end.Add(time.Minute)},
	}, start, end)
	if len(events) != 1 || events[0].ID != "inside" {
		t.Fatalf("events = %#v, want inside only", events)
	}

	knowledge := filterKnowledgeByTime([]models.Knowledge{
		{ID: "before", CreatedAt: start.Add(-time.Minute)},
		{ID: "inside", CreatedAt: start.Add(time.Minute)},
		{ID: "after", CreatedAt: end.Add(time.Minute)},
	}, start, end)
	if len(knowledge) != 1 || knowledge[0].ID != "inside" {
		t.Fatalf("knowledge = %#v, want inside only", knowledge)
	}
}

func TestCountKnowledgeLevels(t *testing.T) {
	obs, lessons, patterns := countKnowledgeLevels([]models.Knowledge{
		{Level: models.LevelObservation},
		{Level: models.LevelLesson},
		{Level: models.LevelPattern},
		{Level: models.LevelObservation},
	})

	if obs != 2 || lessons != 1 || patterns != 1 {
		t.Fatalf("counts = %d/%d/%d, want 2/1/1", obs, lessons, patterns)
	}
}

func TestLooksLikeKnowledgeID(t *testing.T) {
	for _, id := range []string{"obs-1", "lesson-1", "pattern-1"} {
		if !looksLikeKnowledgeID(id) {
			t.Fatalf("looksLikeKnowledgeID(%q) = false, want true", id)
		}
	}
	for _, id := range []string{"sek-1", "event-1", "abc"} {
		if looksLikeKnowledgeID(id) {
			t.Fatalf("looksLikeKnowledgeID(%q) = true, want false", id)
		}
	}
}

func TestCompactSources(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
		want string
	}{
		{name: "empty", want: ""},
		{name: "single", ids: []string{"event-1"}, want: "event-1"},
		{name: "multiple", ids: []string{"event-1", "event-2", "event-3"}, want: "3 sources"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compactSources(tc.ids); got != tc.want {
				t.Fatalf("compactSources(%v) = %q, want %q", tc.ids, got, tc.want)
			}
		})
	}
}
