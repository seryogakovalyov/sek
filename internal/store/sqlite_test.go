package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/seryogakovalyov/sek/internal/models"
)

func newTestStore(t *testing.T) Store {
	t.Helper()
	f, err := os.CreateTemp("", "sek-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	s, err := NewSQLite(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSaveAndList(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	k := &models.Knowledge{
		ID:         "test-1",
		Level:      models.LevelObservation,
		CreatedAt:  time.Now(),
		Content:    "test observation",
		SourceIDs:  []string{"evt-1"},
		EventType:  models.EventFailure,
		Importance: models.ImportanceHigh,
	}
	if err := s.Save(ctx, k); err != nil {
		t.Fatal(err)
	}

	list, err := s.List(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if list[0].EventType != models.EventFailure {
		t.Fatalf("expected EventType failure, got %s", list[0].EventType)
	}
	if list[0].Importance != models.ImportanceHigh {
		t.Fatalf("expected ImportanceHigh, got %f", list[0].Importance)
	}
}

func TestGetKnowledge(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	err := s.Save(ctx, &models.Knowledge{
		ID:         "knowledge-get",
		Level:      models.LevelLesson,
		CreatedAt:  time.Now(),
		Content:    "full lesson content",
		SourceIDs:  []string{"event-1"},
		EventType:  models.EventDecision,
		Importance: models.ImportanceHigh,
		UsageCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetKnowledge(ctx, "knowledge-get")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "knowledge-get" {
		t.Fatalf("ID = %q", got.ID)
	}
	if got.Content != "full lesson content" {
		t.Fatalf("Content = %q", got.Content)
	}
	if got.Level != models.LevelLesson {
		t.Fatalf("Level = %q", got.Level)
	}
	if got.EventType != models.EventDecision {
		t.Fatalf("EventType = %q", got.EventType)
	}
	if got.UsageCount != 2 {
		t.Fatalf("UsageCount = %d", got.UsageCount)
	}
}

func TestGetEvent(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	err := s.Append(ctx, &models.Event{
		ID:            "event-get",
		SessionID:     "session-1",
		ServerSession: "server-1",
		Timestamp:     time.Now(),
		Type:          models.EventDecision,
		Source:        "test",
		Content:       "full event content",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetEvent(ctx, "event-get")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "event-get" {
		t.Fatalf("ID = %q", got.ID)
	}
	if got.Content != "full event content" {
		t.Fatalf("Content = %q", got.Content)
	}
	if got.SessionID != "session-1" {
		t.Fatalf("SessionID = %q", got.SessionID)
	}
	if got.ServerSession != "server-1" {
		t.Fatalf("ServerSession = %q", got.ServerSession)
	}
	if got.Type != models.EventDecision {
		t.Fatalf("Type = %q", got.Type)
	}
}

func TestLogAndListModuleRoutes(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	err := s.LogModuleRoute(ctx, &models.ModuleRouteLog{
		ID:          "route-1",
		KnowledgeID: "obs-1",
		Timestamp:   time.Now(),
		Module:      models.ModuleLocalAI,
		Confidence:  0.95,
		Reason:      "local embeddings setup",
	})
	if err != nil {
		t.Fatal(err)
	}

	routes, err := s.ListModuleRoutes(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].ID != "route-1" {
		t.Fatalf("ID = %q", routes[0].ID)
	}
	if routes[0].KnowledgeID != "obs-1" {
		t.Fatalf("KnowledgeID = %q", routes[0].KnowledgeID)
	}
	if routes[0].Module != models.ModuleLocalAI {
		t.Fatalf("Module = %q", routes[0].Module)
	}
	if routes[0].Confidence != 0.95 {
		t.Fatalf("Confidence = %.2f", routes[0].Confidence)
	}
	if routes[0].Reason != "local embeddings setup" {
		t.Fatalf("Reason = %q", routes[0].Reason)
	}
}

func TestAppendRedactsSecrets(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	err := s.Append(ctx, &models.Event{
		ID:        "event-secret",
		SessionID: "s",
		Timestamp: time.Now(),
		Type:      models.EventFailure,
		Source:    "test",
		Content:   "request failed with token=abc123SECRET and bearer qwertyuiopasdfgh",
	})
	if err != nil {
		t.Fatal(err)
	}

	events, err := s.Query(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	assertRedacted(t, events[0].Content, "abc123SECRET", "qwertyuiopasdfgh")
}

func TestSaveRedactsSecrets(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	err := s.Save(ctx, &models.Knowledge{
		ID:        "knowledge-secret",
		Level:     models.LevelObservation,
		CreatedAt: time.Now(),
		Content:   "use api_key=sk-secret123456 with https://user:pass@example.test/path",
	})
	if err != nil {
		t.Fatal(err)
	}

	knowledge, err := s.List(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(knowledge) != 1 {
		t.Fatalf("expected 1 knowledge entry, got %d", len(knowledge))
	}
	assertRedacted(t, knowledge[0].Content, "sk-secret123456", "user:pass")
}

func assertRedacted(t *testing.T, content string, leaked ...string) {
	t.Helper()
	for _, value := range leaked {
		if strings.Contains(content, value) {
			t.Fatalf("content leaked %q: %s", value, content)
		}
	}
	if !strings.Contains(content, "[REDACTED]") {
		t.Fatalf("expected redaction marker in %q", content)
	}
}

func TestSearchMatchesQueryTokens(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()
	now := time.Now()

	entries := []models.Knowledge{
		{
			ID:         "generic-mcp",
			Level:      models.LevelLesson,
			CreatedAt:  now,
			Content:    "MCP startup was fixed by adding an explicit stdio flag.",
			Importance: models.ImportanceHigh,
		},
		{
			ID:         "retrieval-telemetry",
			Level:      models.LevelLesson,
			CreatedAt:  now.Add(-time.Hour),
			Content:    "Retrieval telemetry writes retrieval_log entries and report_usage increments usage_count for feedback scoring.",
			SourceIDs:  []string{"internal/mcp/server.go", "internal/store/sqlite.go"},
			Importance: models.ImportanceNormal,
		},
	}
	for i := range entries {
		if err := s.Save(ctx, &entries[i]); err != nil {
			t.Fatal(err)
		}
	}

	results, err := s.Search(ctx, "review retrieval telemetry report_usage usage_count feedback loop", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected keyword results")
	}
	if results[0].ID != "retrieval-telemetry" {
		t.Fatalf("expected retrieval-telemetry first, got %s", results[0].ID)
	}
	if results[0].Score <= 0 {
		t.Fatalf("expected positive keyword score, got %f", results[0].Score)
	}
}

func TestFindSimilar(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	emb := make([]float32, 4)
	for i := range emb {
		emb[i] = 0.5
	}

	k1 := &models.Knowledge{
		ID:         "obs-1",
		Level:      models.LevelObservation,
		CreatedAt:  time.Now(),
		Content:    "first observation",
		SourceIDs:  []string{"evt-1"},
		Embedding:  emb,
		EventType:  models.EventRequest,
		Importance: models.ImportanceLow,
	}

	emb2 := []float32{-0.5, -0.5, -0.5, -0.5}
	k2 := &models.Knowledge{
		ID:         "obs-2",
		Level:      models.LevelObservation,
		CreatedAt:  time.Now(),
		Content:    "second observation",
		SourceIDs:  []string{"evt-2"},
		Embedding:  emb2,
		EventType:  models.EventDecision,
		Importance: 0.65,
	}

	for _, k := range []*models.Knowledge{k1, k2} {
		if err := s.Save(ctx, k); err != nil {
			t.Fatal(err)
		}
	}

	similar, err := s.FindSimilar(ctx, emb, 0.9, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(similar) != 1 {
		t.Fatalf("expected 1 similar, got %d", len(similar))
	}
	if similar[0].ID != "obs-1" {
		t.Fatalf("expected obs-1, got %s", similar[0].ID)
	}
}

func TestUpdateSourceIDs(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	k := &models.Knowledge{
		ID:        "obs-1",
		Level:     models.LevelObservation,
		CreatedAt: time.Now(),
		Content:   "test",
		SourceIDs: []string{"evt-1"},
	}
	if err := s.Save(ctx, k); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateSourceIDs(ctx, "obs-1", []string{"evt-1", "evt-2"}); err != nil {
		t.Fatal(err)
	}

	list, err := s.List(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list[0].SourceIDs) != 2 {
		t.Fatalf("expected 2 source IDs, got %d", len(list[0].SourceIDs))
	}
}

func TestFilterByLevel(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	obs := &models.Knowledge{
		ID:        "obs-1",
		Level:     models.LevelObservation,
		CreatedAt: time.Now(),
		Content:   "an observation",
	}
	lesson := &models.Knowledge{
		ID:        "lesson-1",
		Level:     models.LevelLesson,
		CreatedAt: time.Now(),
		Content:   "a lesson",
	}

	for _, k := range []*models.Knowledge{obs, lesson} {
		if err := s.Save(ctx, k); err != nil {
			t.Fatal(err)
		}
	}

	all, err := s.List(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}

	lessons, err := s.List(ctx, models.LevelLesson, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lessons) != 1 {
		t.Fatalf("expected 1 lesson, got %d", len(lessons))
	}
}

func TestDeleteKnowledge(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	k := &models.Knowledge{
		ID:        "to-delete",
		Level:     models.LevelObservation,
		CreatedAt: time.Now(),
		Content:   "delete me",
	}
	if err := s.Save(ctx, k); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteKnowledge(ctx, "to-delete"); err != nil {
		t.Fatal(err)
	}

	list, err := s.List(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list))
	}
}

func TestUnobservedEvents(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	e1 := &models.Event{ID: "e-1", SessionID: "s1", Type: models.EventRequest, Source: "test", Content: "event 1", Timestamp: time.Now()}
	e2 := &models.Event{ID: "e-2", SessionID: "s1", Type: models.EventFailure, Source: "test", Content: "event 2", Timestamp: time.Now()}

	for _, e := range []*models.Event{e1, e2} {
		if err := s.Append(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	obs := &models.Knowledge{
		ID:        "obs-1",
		Level:     models.LevelObservation,
		CreatedAt: time.Now(),
		Content:   "observation from event 1",
		SourceIDs: []string{"e-1"},
	}
	if err := s.Save(ctx, obs); err != nil {
		t.Fatal(err)
	}

	unobserved, err := s.UnobservedEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(unobserved) != 1 {
		t.Fatalf("expected 1 unobserved event, got %d", len(unobserved))
	}
	if unobserved[0].ID != "e-2" {
		t.Fatalf("expected e-2, got %s", unobserved[0].ID)
	}
}

func TestClear(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		k := &models.Knowledge{
			ID:        fmt.Sprintf("k-%d", i),
			Level:     models.LevelObservation,
			CreatedAt: time.Now(),
			Content:   fmt.Sprintf("item %d", i),
		}
		if err := s.Save(ctx, k); err != nil {
			t.Fatal(err)
		}
		ev := &models.Event{
			ID:        fmt.Sprintf("e-%d", i),
			SessionID: "test-session",
			Type:      models.EventRequest,
			Source:    "test",
			Content:   fmt.Sprintf("event %d", i),
			Timestamp: time.Now(),
		}
		if err := s.Append(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.KnowledgeCount != 0 {
		t.Fatalf("expected 0 knowledge, got %d", stats.KnowledgeCount)
	}
	if stats.EventCount != 0 {
		t.Fatalf("expected 0 events, got %d", stats.EventCount)
	}
}

func TestRetrievalUsageIsValidatedAndIdempotent(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	k := &models.Knowledge{
		ID:        "obs-1",
		Level:     models.LevelObservation,
		CreatedAt: time.Now(),
		Content:   "test observation",
	}
	if err := s.Save(ctx, k); err != nil {
		t.Fatal(err)
	}

	results, _ := json.Marshal([]retrievalResultEntry{{ID: "obs-1", Score: 0.9}})
	if err := s.LogRetrieval(ctx, &models.RetrievalLog{
		ID:        "ret-1",
		SessionID: "s1",
		Timestamp: time.Now(),
		Task:      "test task",
		Results:   string(results),
		UsedIDs:   "[]",
	}); err != nil {
		t.Fatal(err)
	}

	added, err := s.MarkRetrievalUsed(ctx, "ret-1", "obs-1")
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("expected first mark to add usage")
	}
	if err := s.IncrementUsageCount(ctx, "obs-1"); err != nil {
		t.Fatal(err)
	}

	added, err = s.MarkRetrievalUsed(ctx, "ret-1", "obs-1")
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Fatal("expected duplicate mark to be idempotent")
	}

	list, err := s.List(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].UsageCount != 1 {
		t.Fatalf("expected usage_count 1, got %d", list[0].UsageCount)
	}

	usedIDs := queryString(t, s, `SELECT used_ids FROM retrieval_log WHERE id = 'ret-1'`)
	if usedIDs != `["obs-1"]` {
		t.Fatalf("unexpected used_ids: %s", usedIDs)
	}
}

func TestRetrievalUsageRejectsInvalidIDs(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	results, _ := json.Marshal([]retrievalResultEntry{{ID: "obs-1", Score: 0.9}})
	if err := s.LogRetrieval(ctx, &models.RetrievalLog{
		ID:        "ret-1",
		SessionID: "s1",
		Timestamp: time.Now(),
		Task:      "test task",
		Results:   string(results),
		UsedIDs:   "[]",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.MarkRetrievalUsed(ctx, "ret-1", "obs-missing"); err == nil {
		t.Fatal("expected error for knowledge not returned by retrieval")
	}
	if err := s.IncrementUsageCount(ctx, "obs-missing"); err == nil {
		t.Fatal("expected error for missing knowledge")
	}
}

func TestClearDeletesRetrievalLog(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	if err := s.LogRetrieval(ctx, &models.RetrievalLog{
		ID:        "ret-1",
		SessionID: "s1",
		Timestamp: time.Now(),
		Task:      "test task",
		Results:   "[]",
		UsedIDs:   "[]",
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, s, "retrieval_log"); got != 0 {
		t.Fatalf("expected retrieval_log to be empty, got %d", got)
	}
}

func TestGCDeletesOldRetrievalLog(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	oldTime := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	for _, logEntry := range []models.RetrievalLog{
		{ID: "old", SessionID: "s1", Timestamp: oldTime, Task: "old", Results: "[]", UsedIDs: "[]"},
		{ID: "new", SessionID: "s1", Timestamp: newTime, Task: "new", Results: "[]", UsedIDs: "[]"},
	} {
		entry := logEntry
		if err := s.LogRetrieval(ctx, &entry); err != nil {
			t.Fatal(err)
		}
	}

	result, err := s.GC(ctx, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	if result.RetrievalDeleted != 1 {
		t.Fatalf("expected 1 retrieval deleted, got %d", result.RetrievalDeleted)
	}
	if got := countRows(t, s, "retrieval_log"); got != 1 {
		t.Fatalf("expected one retrieval log to remain, got %d", got)
	}
}

func queryString(t *testing.T, s Store, query string) string {
	t.Helper()
	sqlite := s.(*sqliteStore)
	var value string
	if err := sqlite.db.QueryRow(query).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func countRows(t *testing.T, s Store, table string) int {
	t.Helper()
	sqlite := s.(*sqliteStore)
	var count int
	if err := sqlite.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
