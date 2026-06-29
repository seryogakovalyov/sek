package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/anomalyco/sek/internal/llm"
	"github.com/anomalyco/sek/internal/models"
	"github.com/anomalyco/sek/internal/reuse"
	"github.com/anomalyco/sek/internal/store"
	"github.com/anomalyco/sek/internal/trace"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("sekctl: ")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "init":
		cmdInit(args)
	case "list":
		cmdList(args)
	case "log":
		cmdLog(args)
	case "rm":
		cmdRemove(args)
	case "gc":
		cmdGC(args)
	case "status", "stats":
		cmdStatus(args)
	case "prune":
		cmdPrune(args)
	case "query":
		cmdQuery(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage: sekctl <command> [flags]

Commands:
  init              Create .sek directory and store
  list              List knowledge entries
  log               List recent events
  rm <id>           Delete knowledge by ID
  gc                Delete old entries (GC by TTL or absolute cutoff)
  status, stats     Show project statistics
  prune             Delete all knowledge and events
  query <task>      Query experience (needs LLM flags)

Run 'sekctl <command> -help' for command-specific flags.
`)
}

func projectDir() string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("getwd: %v", err)
	}
	return dir
}

func storePath(project string) string {
	if project == "" {
		project = projectDir()
	}
	if project == "_global" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("home dir: %v", err)
		}
		return filepath.Join(home, ".sek", "store.db")
	}
	return filepath.Join(project, ".sek", "store.db")
}

func storeKind(project string) string {
	if project == "_global" {
		return "global shared"
	}
	return "per-project"
}

func openStore(project string) store.Store {
	s, err := store.NewSQLite(storePath(project))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	return s
}

// --- init ---

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	fs.Parse(args)

	dir := *project
	if dir == "" {
		dir = projectDir()
	}

	sp := storePath(dir)
	if err := os.MkdirAll(filepath.Dir(sp), 0755); err != nil {
		log.Fatalf("create .sek dir: %v", err)
	}
	if _, err := os.Stat(sp); err == nil {
		fmt.Printf("store already exists at %s\n", sp)
		return
	}

	s, err := store.NewSQLite(sp)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}
	s.Close()
	fmt.Printf("initialized project store at %s\n", sp)
}

// --- list ---

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	level := fs.String("level", "", "filter by level: observation, lesson, pattern")
	limit := fs.Int("limit", 20, "max entries")
	fs.Parse(args)

	s := openStore(*project)
	defer s.Close()

	ctx := context.Background()
	knowledge, err := s.List(ctx, models.KnowledgeLevel(*level), *limit)
	if err != nil {
		log.Fatalf("list: %v", err)
	}

	if len(knowledge) == 0 {
		fmt.Println("no knowledge stored")
		return
	}
	reverseKnowledge(knowledge)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tLEVEL\tCREATED\tSOURCES\tCONTENT")
	for _, k := range knowledge {
		content := strings.ReplaceAll(k.Content, "\n", " ")
		if len(content) > 60 {
			content = content[:60] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", k.ID, k.Level, k.CreatedAt.Format("2006-01-02 15:04"), strings.Join(k.SourceIDs, ","), content)
	}
	w.Flush()
	fmt.Printf("\n%d entries\n", len(knowledge))
}

// --- log ---

func cmdLog(args []string) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	limit := fs.Int("limit", 20, "max events")
	fs.Parse(args)

	s := openStore(*project)
	defer s.Close()

	ctx := context.Background()
	events, err := s.Query(ctx, *limit)
	if err != nil {
		log.Fatalf("query events: %v", err)
	}

	if len(events) == 0 {
		fmt.Println("no events recorded")
		return
	}
	reverseEvents(events)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tSOURCE\tTIME\tCONTENT")
	for _, e := range events {
		content := strings.ReplaceAll(e.Content, "\n", " ")
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.ID, e.Type, e.Source, e.Timestamp.Format("2006-01-02 15:04"), content)
	}
	w.Flush()
	fmt.Printf("\n%d events\n", len(events))
}

func reverseKnowledge(items []models.Knowledge) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func reverseEvents(items []models.Event) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

// --- rm ---

func cmdRemove(args []string) {
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	fs.Parse(args)

	id := fs.Arg(0)
	if id == "" {
		log.Fatal("usage: sekctl rm <knowledge-id>")
	}

	s := openStore(*project)
	defer s.Close()

	ctx := context.Background()
	if err := s.DeleteKnowledge(ctx, id); err != nil {
		log.Fatalf("delete: %v", err)
	}
	fmt.Printf("deleted %s\n", id)
}

// --- status ---

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	fs.Parse(args)

	s := openStore(*project)
	defer s.Close()

	ctx := context.Background()
	stats, err := s.Stats(ctx)
	if err != nil {
		log.Fatalf("stats: %v", err)
	}

	fmt.Printf("Store:       %s\n", storeKind(*project))
	fmt.Printf("DB path:     %s\n", storePath(*project))
	fmt.Printf("DB size:     %d KB\n", stats.DBSizeBytes/1024)
	fmt.Printf("Events:      %d\n", stats.EventCount)
	fmt.Printf("Knowledge:   %d\n", stats.KnowledgeCount)
}

// --- gc ---

func cmdGC(args []string) {
	fs := flag.NewFlagSet("gc", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	olderThan := fs.String("older-than", "", "delete entries older than this duration (e.g. 720h, 168h)")
	before := fs.String("before", "", "delete entries created before this timestamp (RFC3339 or YYYY-MM-DD)")
	dryRun := fs.Bool("dry-run", false, "show what would be deleted without deleting")
	fs.Parse(args)

	if *olderThan == "" && *before == "" {
		*olderThan = "720h"
	}
	if *olderThan != "" && *before != "" {
		log.Fatalf("--older-than and --before are mutually exclusive")
	}

	sp := storePath(*project)
	if _, err := os.Stat(sp); os.IsNotExist(err) {
		fmt.Println("no store found at", sp)
		return
	}

	s := openStore(*project)
	defer s.Close()

	var cutoff time.Time
	if *before != "" {
		var err error
		cutoff, err = parseTimestamp(*before)
		if err != nil {
			log.Fatalf("invalid --before timestamp %q: %v", *before, err)
		}
	} else {
		cutoff = time.Now().Add(-parseDuration(*olderThan))
	}
	cutoffStr := cutoff.Format(time.RFC3339Nano)

	if *dryRun {
		fmt.Printf("Store:  %s\n", sp)
		if *before != "" {
			fmt.Printf("Cutoff: %s (before %s)\n", cutoff.Format("2006-01-02"), *before)
		} else {
			fmt.Printf("Cutoff: %s (older than %s)\n", cutoff.Format("2006-01-02"), *olderThan)
		}
		fmt.Println("(dry-run, no changes made)")
		return
	}

	ctx := context.Background()
	result, err := s.GC(ctx, cutoffStr)
	if err != nil {
		log.Fatalf("gc: %v", err)
	}
	fmt.Printf("GC complete (cutoff: %s)\n", cutoff.Format("2006-01-02"))
	fmt.Printf("  knowledge deleted: %d\n", result.KnowledgeDeleted)
	fmt.Printf("  events deleted:    %d\n", result.EventsDeleted)
	fmt.Printf("  orphans deleted:   %d\n", result.OrphansDeleted)
	fmt.Printf("  retrieval deleted: %d\n", result.RetrievalDeleted)
}

func parseTimestamp(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp format (try RFC3339 or YYYY-MM-DD)")
}

func splitArgs(args []string) (task string, flags []string) {
	var parts []string
	afterSep := false
	for _, a := range args {
		if a == "--" {
			afterSep = true
			continue
		}
		if afterSep || !strings.HasPrefix(a, "-") {
			parts = append(parts, a)
		} else {
			flags = append(flags, a)
		}
	}
	task = strings.Join(parts, " ")
	return
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Fatalf("invalid duration %q: %v", s, err)
	}
	return d
}

// --- prune ---

func cmdPrune(args []string) {
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	force := fs.Bool("force", false, "skip confirmation")
	fs.Parse(args)

	if !*force {
		fmt.Print("WARNING: this will delete ALL knowledge and events. Continue? [y/N]: ")
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(answer) != "y" && strings.ToLower(answer) != "yes" {
			fmt.Println("cancelled")
			return
		}
	}

	s := openStore(*project)
	defer s.Close()

	ctx := context.Background()
	if err := s.Clear(ctx); err != nil {
		log.Fatalf("prune: %v", err)
	}
	fmt.Println("store cleared")
}

// --- query ---

func cmdQuery(args []string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	llmProvider := fs.String("llm-provider", "openai", "LLM provider")
	llmModel := fs.String("llm-model", "gpt-4o", "LLM model")
	llmKey := fs.String("llm-key", "", "LLM API key")
	llmBaseURL := fs.String("llm-base-url", "", "LLM API base URL")
	maxTokens := fs.Int("max-tokens", 2000, "max tokens in response")
	maxEntries := fs.Int("max-entries", 10, "max entries")

	task, flagArgs := splitArgs(args)
	fs.Parse(flagArgs)
	if task == "" {
		log.Fatal("usage: sekctl query <task description>")
	}

	if *llmKey == "" {
		*llmKey = os.Getenv("SEK_LLM_KEY")
	}
	if *llmKey == "" {
		log.Fatal("LLM API key required: set --llm-key or SEK_LLM_KEY")
	}

	cfg := llm.Config{
		Provider: llm.ProviderType(*llmProvider),
		APIKey:   *llmKey,
		BaseURL:  *llmBaseURL,
		Model:    *llmModel,
	}
	provider, err := llm.NewProvider(cfg)
	if err != nil {
		log.Fatalf("llm: %v", err)
	}
	embedder := llm.NewOpenAIEmbedder(cfg.APIKey, cfg.BaseURL, cfg.Model)

	s := openStore(*project)
	defer s.Close()

	engine := reuse.NewEngine(provider, embedder, s)
	ctx := context.Background()
	result, err := engine.Query(ctx, models.ReuseRequest{
		Task: task,
		Budget: models.ContextBudget{
			MaxTokens:  *maxTokens,
			MaxEntries: *maxEntries,
		},
	})
	if err != nil {
		log.Fatalf("query: %v", err)
	}

	if len(result.Knowledge) == 0 {
		fmt.Println("no relevant experience found")
		return
	}

	for _, k := range result.Knowledge {
		fmt.Println(trace.FormatKnowledge(k, true))
		fmt.Println()
	}
	fmt.Printf("total tokens: %d\n", result.TotalTokens)
}
