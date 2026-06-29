package distill

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/seryogakovalyov/sek/internal/models"
	"github.com/seryogakovalyov/sek/internal/store"
)

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	f, err := os.CreateTemp("", "sek-digest-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	s, err := store.NewSQLite(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func addEvent(t *testing.T, ctx context.Context, s store.Store, id, serverSession, evType, content string) {
	t.Helper()
	err := s.Append(ctx, &models.Event{
		ID:            id,
		SessionID:     serverSession,
		ServerSession: serverSession,
		Timestamp:     time.Now(),
		Type:          models.EventType(evType),
		Source:        "test",
		Content:       content,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func addObservation(t *testing.T, ctx context.Context, s store.Store, id, content string, sourceIDs []string) {
	t.Helper()
	err := s.Save(ctx, &models.Knowledge{
		ID:        id,
		Level:     models.LevelObservation,
		CreatedAt: time.Now(),
		Content:   content,
		SourceIDs: sourceIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSessionDigestSkipsWhenNoEvents(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	SessionDigest(ctx, s, nil, nil, "", "sek-test")
	stats, _ := s.Stats(ctx)
	if stats.KnowledgeCount != 0 {
		t.Fatalf("expected 0 knowledge, got %d", stats.KnowledgeCount)
	}
}

func TestSessionDigestSkipsWhenFewerThan3(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	addEvent(t, ctx, s, "e1", "sek-test", "request", "first event")
	addEvent(t, ctx, s, "e2", "sek-test", "response", "second event")

	SessionDigest(ctx, s, nil, nil, "", "sek-test")
	stats, _ := s.Stats(ctx)
	if stats.KnowledgeCount != 0 {
		t.Fatalf("expected 0 knowledge, got %d", stats.KnowledgeCount)
	}
}

func TestUnobservedEventsFilter(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	addEvent(t, ctx, s, "e1", "s1", "failure", "broken build")
	addEvent(t, ctx, s, "e2", "s1", "decision", "use testify")
	addEvent(t, ctx, s, "e3", "s1", "successful_fix", "fixed by adding timeout")

	addObservation(t, ctx, s, "obs-1", "found broken build", []string{"e1"})

	events, err := s.UnobservedEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 unobserved, got %d", len(events))
	}

	allEvents, err := s.UnobservedEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(allEvents) < 2 {
		t.Fatalf("expected at least 2 unobserved events, got %d", len(allEvents))
	}
}
