package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/anomalyco/sek/internal/capture"
	"github.com/anomalyco/sek/internal/distill"
	"github.com/anomalyco/sek/internal/llm"
	"github.com/anomalyco/sek/internal/models"
	"github.com/anomalyco/sek/internal/reuse"
	"github.com/anomalyco/sek/internal/store"
	"github.com/anomalyco/sek/internal/trace"
)

func newMCPServer(st store.Store, provider llm.Provider, embedder llm.Embedder, modelName string, serverSessionID string) *server.MCPServer {
	captureSvc := capture.NewService(st)
	distillPipe := distill.NewPipeline(provider, embedder, modelName, st)
	reuseEngine := reuse.NewEngine(provider, embedder, st)

	s := server.NewMCPServer("SEK", "0.1.0",
		server.WithLogging(),
	)

	s.AddTool(captureTool(), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		eventType := mcp.ParseString(req, "event_type", "")
		source := mcp.ParseString(req, "source", "")
		content := mcp.ParseString(req, "content", "")
		projectID := mcp.ParseString(req, "project_id", "default")
		sessionID := mcp.ParseString(req, "session_id", "")
		if sessionID == "" {
			sessionID = serverSessionID
		}

		event, err := captureSvc.Capture(ctx, projectID, sessionID, serverSessionID, models.EventType(eventType), source, content)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("capture failed: %v", err)), nil
		}

		distillErr := distillPipe.Process(ctx, []models.Event{*event})

		msg := fmt.Sprintf("captured event: %s", event.ID)
		if distillErr != nil {
			msg += fmt.Sprintf("\ndistill warning: %v", distillErr)
		}

		return mcp.NewToolResultText(msg), nil
	})

	s.AddTool(queryTool(), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID := mcp.ParseString(req, "project_id", "default")
		task := mcp.ParseString(req, "task", "")
		maxTokens := mcp.ParseInt(req, "max_tokens", 2000)
		maxEntries := mcp.ParseInt(req, "max_entries", 10)

		result, err := reuseEngine.Query(ctx, models.ReuseRequest{
			ProjectID: projectID,
			Task:      task,
			Budget: models.ContextBudget{
				MaxTokens:  maxTokens,
				MaxEntries: maxEntries,
			},
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("query failed: %v", err)), nil
		}

		output := ""
		for _, k := range result.Knowledge {
			output += trace.FormatKnowledge(k, true) + "\n\n"
		}
		if output == "" {
			output = "No relevant experience found."
		}

		return mcp.NewToolResultText(output), nil
	})

	s.AddTool(listKnowledgeTool(), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID := mcp.ParseString(req, "project_id", "default")
		level := mcp.ParseString(req, "level", "")
		limit := mcp.ParseInt(req, "limit", 20)

		knowledge, err := st.List(ctx, projectID, models.KnowledgeLevel(level), limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list failed: %v", err)), nil
		}

		output := ""
		for _, k := range knowledge {
			output += trace.FormatKnowledge(k, false) + "\n\n"
		}
		if output == "" {
			output = "No knowledge stored yet."
		}

		return mcp.NewToolResultText(output), nil
	})

	return s
}

func Serve(ctx context.Context, st store.Store, provider llm.Provider, embedder llm.Embedder, modelName string, serverSessionID string) error {
	s := newMCPServer(st, provider, embedder, modelName, serverSessionID)
	log.Println("SEK MCP server starting (stdio)...")
	return server.ServeStdio(s)
}

func ServeHTTP(ctx context.Context, st store.Store, provider llm.Provider, embedder llm.Embedder, modelName string, serverSessionID string, addr string) error {
	s := newMCPServer(st, provider, embedder, modelName, serverSessionID)
	rawServer := server.NewStreamableHTTPServer(s,
		server.WithEndpointPath("/"),
		server.WithStateLess(true),
	)

	handler := corsMiddleware(rawServer)

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("SEK MCP server starting (Streamable HTTP) on %s", addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, MCP-Version, Mcp-Session-Id, mcp-protocol-version, mcp-session-id")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type, Mcp-Session-Id, mcp-session-id, mcp-protocol-version")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func captureTool() mcp.Tool {
	return mcp.NewTool("capture_event",
		mcp.WithDescription(`Record a notable event for future sessions. Call this when something is worth remembering:

WHEN TO CALL (trigger rules):
- FAILURE: an error/bug was encountered — describe what went wrong and how
- SUCCESSFUL_FIX: a bug was fixed or a problem was solved — describe the root cause and fix
- DECISION: a design/architecture/approach decision was made — include rationale
- IMPLEMENTATION_CHOICE: a specific library, pattern, or technique was chosen — include why
- TOOL_USAGE: a tool/command was used in a novel or important way — include exact command
- REQUEST / RESPONSE: only for key insights from the conversation itself (rare)

WHAT TO CAPTURE: concrete, specific, actionable. Include error messages, command names, file paths, library versions. Think "what would the next session's agent be glad to know without re-discovering?"

ATOMICITY: capture one reusable lesson per event. If a task produced multiple independent lessons (for example a roadmap decision, a build failure, and a config fix), call capture_event multiple times instead of packing them into one long event. This keeps distillation precise and searchable.

WHAT NOT TO CAPTURE: trivial steps, work-in-progress, obvious boilerplate, every single tool call.
`),
		mcp.WithString("event_type",
			mcp.Required(),
			mcp.Description("Event type: request, response, tool_usage, failure, decision, implementation_choice, successful_fix"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("Source component (e.g. opencode, claude-code, cursor, or specific tool name)"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Detailed description of the event — include error messages, file paths, command examples, rationale. The more context, the better the distilled observation will be."),
		),
		mcp.WithString("project_id",
			mcp.Description("Project identifier (default: 'default')"),
		),
		mcp.WithString("session_id",
			mcp.Description("Session identifier (auto-tagged with server session if empty)"),
		),
	)
}

func queryTool() mcp.Tool {
	return mcp.NewTool("query_experience",
		mcp.WithDescription(`Search past project experience relevant to the current task. ALWAYS use this when the user asks ANY question about the project, even if you think you know the answer.

WHEN TO USE (ALWAYS call — do NOT answer from your own knowledge):
- User asks "how to", "what's the best", "which to choose", "why did we" — call this tool
- User asks for recommendations, best practices for this project — call this tool
- User reports an error or bug — call this tool with the error text
- User mentions a file, library, or pattern — call this tool to find related experience
- User asks "как", "почему", "что лучше", "какую БД" — call this tool

WHEN NOT TO USE:
- Only skip if the question is purely about general coding knowledge unrelated to this project

HOW TO QUERY:
- Be specific and include context (e.g. "how do we run integration tests?" not "testing")
- For errors: paste the actual error message
- The search is semantic (vector-based), so language doesn't matter — queries in ANY language find related English observations
`),
		mcp.WithString("project_id",
			mcp.Description("Project identifier (default: 'default')"),
		),
		mcp.WithString("task",
			mcp.Required(),
			mcp.Description("Task description or question — be specific, include error messages or file paths where relevant"),
		),
		mcp.WithInteger("max_tokens",
			mcp.Description("Maximum tokens for returned experience (default: 2000)"),
		),
		mcp.WithInteger("max_entries",
			mcp.Description("Maximum number of experience entries (default: 10)"),
		),
	)
}

func listKnowledgeTool() mcp.Tool {
	return mcp.NewTool("list_knowledge",
		mcp.WithDescription("List stored knowledge entries"),
		mcp.WithString("project_id",
			mcp.Description("Project identifier (default: 'default')"),
		),
		mcp.WithString("level",
			mcp.Description("Filter by level: observation, lesson, pattern"),
		),
		mcp.WithInteger("limit",
			mcp.Description("Maximum entries to return (default: 20)"),
		),
	)
}
