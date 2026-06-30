package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"

	"github.com/seryogakovalyov/sek/internal/models"
	"github.com/seryogakovalyov/sek/internal/store"
)

func TestFormatKnowledgeResourceEmpty(t *testing.T) {
	got := formatKnowledgeResource("Recent SEK knowledge", nil)

	if !strings.Contains(got, "# Recent SEK knowledge") {
		t.Fatalf("missing title:\n%s", got)
	}
	if !strings.Contains(got, "No knowledge stored yet.") {
		t.Fatalf("missing empty message:\n%s", got)
	}
}

func TestFormatKnowledgeResource(t *testing.T) {
	got := formatKnowledgeResource("SEK observations", []models.Knowledge{
		{
			ID:        "obs-1",
			Level:     models.LevelObservation,
			Content:   "Use GOCACHE=/tmp/sek-go-build-cache for sandboxed Go tests.",
			CreatedAt: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
		},
	})

	if !strings.Contains(got, "# SEK observations") {
		t.Fatalf("missing title:\n%s", got)
	}
	if !strings.Contains(got, "obs-1") {
		t.Fatalf("missing knowledge id:\n%s", got)
	}
	if !strings.Contains(got, "Use GOCACHE=/tmp/sek-go-build-cache") {
		t.Fatalf("missing knowledge content:\n%s", got)
	}
}

func TestFormatStats(t *testing.T) {
	got := formatStats(&store.StoreStats{
		KnowledgeCount: 3,
		EventCount:     7,
		DBSizeBytes:    4096,
	})

	for _, want := range []string{
		"# SEK store stats",
		"- knowledge: 3",
		"- events: 7",
		"- db_size_bytes: 4096",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
}

func TestResourcesCanBeListedAndRead(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewSQLite(t.TempDir() + "/store.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	err = st.Save(ctx, &models.Knowledge{
		ID:        "obs-resource-test",
		Level:     models.LevelObservation,
		CreatedAt: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
		Content:   "MCP resources expose read-only SEK knowledge snapshots.",
	})
	if err != nil {
		t.Fatalf("save knowledge: %v", err)
	}

	srv := mcptest.NewUnstartedServer(t)
	defer srv.Close()
	srv.AddResources(sekResources(st)...)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start MCP test server: %v", err)
	}

	list, err := srv.Client().ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if len(list.Resources) != 5 {
		t.Fatalf("resource count = %d, want 5", len(list.Resources))
	}
	t.Logf("resources: %s", resourceURIs(list.Resources))
	if !hasResource(list.Resources, resourceObservations) {
		t.Fatalf("missing %s in %#v", resourceObservations, list.Resources)
	}

	var req mcp.ReadResourceRequest
	req.Params.URI = resourceObservations
	read, err := srv.Client().ReadResource(ctx, req)
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("content count = %d, want 1", len(read.Contents))
	}
	content, ok := read.Contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("content type = %T, want TextResourceContents", read.Contents[0])
	}
	if !strings.Contains(content.Text, "MCP resources expose read-only SEK knowledge snapshots.") {
		t.Fatalf("resource content missing knowledge:\n%s", content.Text)
	}
	t.Logf("read %s:\n%s", resourceObservations, content.Text)
}

func hasResource(resources []mcp.Resource, uri string) bool {
	for _, r := range resources {
		if r.URI == uri {
			return true
		}
	}
	return false
}

func resourceURIs(resources []mcp.Resource) string {
	uris := make([]string, 0, len(resources))
	for _, r := range resources {
		uris = append(uris, r.URI)
	}
	return strings.Join(uris, ", ")
}
