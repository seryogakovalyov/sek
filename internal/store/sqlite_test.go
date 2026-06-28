package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anomalyco/sek/internal/models"
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

func TestMigrateDropsLegacyProjectIDColumns(t *testing.T) {
	f, err := os.CreateTemp("", "sek-legacy-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := sql.Open("sqlite", f.Name())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE events (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			server_session TEXT DEFAULT '',
			timestamp TEXT NOT NULL,
			type TEXT NOT NULL,
			source TEXT NOT NULL,
			content TEXT NOT NULL
		);
		CREATE TABLE knowledge (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			level TEXT NOT NULL,
			created_at TEXT NOT NULL,
			content TEXT NOT NULL,
			source_ids TEXT DEFAULT '[]',
			embedding BLOB,
			event_type TEXT DEFAULT '',
			importance REAL DEFAULT 0.5,
			usage_count INTEGER DEFAULT 0
		);
		CREATE TABLE retrieval_log (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			task TEXT NOT NULL,
			results TEXT DEFAULT '[]',
			used_ids TEXT DEFAULT '[]'
		);
		INSERT INTO events (id, project_id, session_id, timestamp, type, source, content)
			VALUES ('e1', 'p', 's', '2026-06-28T00:00:00Z', 'request', 'test', 'event');
		INSERT INTO knowledge (id, project_id, level, created_at, content)
			VALUES ('k1', 'p', 'observation', '2026-06-28T00:00:00Z', 'knowledge');
		INSERT INTO retrieval_log (id, project_id, session_id, timestamp, task)
			VALUES ('r1', 'p', 's', '2026-06-28T00:00:00Z', 'task');
	`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}

	s, err := NewSQLite(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for _, table := range []string{"events", "knowledge", "retrieval_log"} {
		hasProjectID, err := tableHasColumn(s.(*sqliteStore).db, table, "project_id")
		if err != nil {
			t.Fatal(err)
		}
		if hasProjectID {
			t.Fatalf("%s still has project_id column", table)
		}
	}

	stats, err := s.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.EventCount != 1 || stats.KnowledgeCount != 1 {
		t.Fatalf("unexpected migrated counts: events=%d knowledge=%d", stats.EventCount, stats.KnowledgeCount)
	}
}
