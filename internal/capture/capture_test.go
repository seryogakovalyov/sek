package capture

import (
	"context"
	"strings"
	"testing"

	"github.com/anomalyco/sek/internal/models"
	"github.com/anomalyco/sek/internal/store"
)

func TestCaptureRedactsSecretsBeforeStore(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	svc := NewService(st)
	event, err := svc.Capture(
		context.Background(),
		"s",
		"ss",
		models.EventFailure,
		"test",
		"failed with OPENAI_API_KEY=sk-secret123456 and Authorization: Bearer abcdefghijklmnop",
	)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecret(t, event.Content)

	events, err := st.Query(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	assertNoSecret(t, events[0].Content)
	if !strings.Contains(events[0].Content, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got %q", events[0].Content)
	}
}

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.NewSQLite(t.TempDir() + "/store.db")
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func assertNoSecret(t *testing.T, s string) {
	t.Helper()
	for _, leaked := range []string{"sk-secret123456", "abcdefghijklmnop"} {
		if strings.Contains(s, leaked) {
			t.Fatalf("content leaked %q: %s", leaked, s)
		}
	}
}
