package mcp

import (
	"context"
	"fmt"
	"strings"

	mcpsdk "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/seryogakovalyov/sek/internal/models"
	"github.com/seryogakovalyov/sek/internal/store"
	"github.com/seryogakovalyov/sek/internal/trace"
)

const (
	resourceRecent       = "sek://knowledge/recent"
	resourceObservations = "sek://knowledge/observations"
	resourceLessons      = "sek://knowledge/lessons"
	resourcePatterns     = "sek://knowledge/patterns"
	resourceStats        = "sek://stats"
	resourceMIME         = "text/markdown"
)

func addResources(s *server.MCPServer, st store.Store) {
	s.AddResources(sekResources(st)...)
}

func sekResources(st store.Store) []server.ServerResource {
	return []server.ServerResource{
		knowledgeResource(st, resourceRecent, "Recent SEK knowledge", "", "Recent observations, lessons, and patterns."),
		knowledgeResource(st, resourceObservations, "SEK observations", models.LevelObservation, "Recent observation-level knowledge."),
		knowledgeResource(st, resourceLessons, "SEK lessons", models.LevelLesson, "Recent lesson-level knowledge."),
		knowledgeResource(st, resourcePatterns, "SEK patterns", models.LevelPattern, "Recent pattern-level knowledge."),
		{
			Resource: mcpsdk.NewResource(resourceStats, "SEK store stats",
				mcpsdk.WithMIMEType(resourceMIME),
				mcpsdk.WithResourceDescription("Read-only counters for the current SEK store."),
			),
			Handler: func(ctx context.Context, req mcpsdk.ReadResourceRequest) ([]mcpsdk.ResourceContents, error) {
				stats, err := st.Stats(ctx)
				if err != nil {
					return nil, err
				}
				return textResource(req.Params.URI, formatStats(stats)), nil
			},
		},
	}
}

func knowledgeResource(st store.Store, uri string, name string, level models.KnowledgeLevel, description string) server.ServerResource {
	return server.ServerResource{
		Resource: mcpsdk.NewResource(uri, name,
			mcpsdk.WithMIMEType(resourceMIME),
			mcpsdk.WithResourceDescription(description),
		),
		Handler: func(ctx context.Context, req mcpsdk.ReadResourceRequest) ([]mcpsdk.ResourceContents, error) {
			knowledge, err := st.List(ctx, level, 20)
			if err != nil {
				return nil, err
			}
			return textResource(req.Params.URI, formatKnowledgeResource(name, knowledge)), nil
		},
	}
}

func textResource(uri string, text string) []mcpsdk.ResourceContents {
	return []mcpsdk.ResourceContents{
		mcpsdk.TextResourceContents{
			URI:      uri,
			MIMEType: resourceMIME,
			Text:     text,
		},
	}
}

func formatKnowledgeResource(title string, knowledge []models.Knowledge) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	if len(knowledge) == 0 {
		b.WriteString("No knowledge stored yet.\n")
		return b.String()
	}
	for _, k := range knowledge {
		b.WriteString(trace.FormatKnowledge(k, false))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func formatStats(stats *store.StoreStats) string {
	return fmt.Sprintf(`# SEK store stats

- knowledge: %d
- events: %d
- db_size_bytes: %d
`, stats.KnowledgeCount, stats.EventCount, stats.DBSizeBytes)
}
