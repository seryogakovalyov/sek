package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/seryogakovalyov/sek/internal/llm"
	"github.com/seryogakovalyov/sek/internal/models"
	"github.com/seryogakovalyov/sek/internal/reuse"
	"github.com/seryogakovalyov/sek/internal/store"
	"github.com/seryogakovalyov/sek/internal/storepath"
	"github.com/seryogakovalyov/sek/internal/trace"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("sekctl: ")
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(argv []string, stderr io.Writer) int {
	if len(argv) < 1 {
		printUsage(stderr)
		return 1
	}

	cmd := argv[0]
	args := argv[1:]

	switch cmd {
	case "help", "-h", "--help":
		printUsage(stderr)
		return 0
	case "init":
		cmdInit(args)
	case "list":
		cmdList(args)
	case "log":
		cmdLog(args)
	case "show":
		cmdShow(args)
	case "rm":
		cmdRemove(args)
	case "gc":
		cmdGC(args)
	case "diff":
		cmdDiff(args)
	case "usage", "telemetry":
		cmdUsage(args)
	case "status", "stats":
		cmdStatus(args)
	case "prune":
		cmdPrune(args)
	case "query":
		cmdQuery(args)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", cmd)
		printUsage(stderr)
		return 1
	}

	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: sekctl <command> [flags]

Commands:
  init              Create .sek directory and store
  list              List knowledge entries
  log               List recent events
  show <id>         Show a full knowledge entry or event
  rm <id>           Delete knowledge by ID
  gc                Delete old entries (GC by TTL or absolute cutoff)
  diff              Review events and knowledge added by time range or session
  usage, telemetry  Show retrieval usage telemetry
  status, stats     Show project statistics
  prune             Delete all knowledge and events
  query <task>      Query experience (needs LLM flags)

Run 'sekctl --help' to show this help.
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

func storePath(project string, global bool, explicitPath string) string {
	if project == "" {
		project = projectDir()
	}
	path, err := storepath.Resolve(storepath.Options{
		ProjectDir:   project,
		ExplicitPath: explicitPath,
		Global:       global,
	})
	if err != nil {
		log.Fatalf("resolve store path: %v", err)
	}
	return path
}

func openStore(project string, global bool, explicitPath string) store.Store {
	s, err := store.NewSQLite(storePath(project, global, explicitPath))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	return s
}

// --- init ---

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
	fs.Parse(args)

	sp := storePath(*project, *global, *storeFlag)
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
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
	level := fs.String("level", "", "filter by level: observation, lesson, pattern")
	limit := fs.Int("limit", 20, "max entries")
	fs.Parse(args)

	s := openStore(*project, *global, *storeFlag)
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
	fmt.Fprintln(w, "ID\tLEVEL\tCREATED\tSOURCES")
	for _, k := range knowledge {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", k.ID, k.Level, k.CreatedAt.Format("2006-01-02 15:04"), compactSources(k.SourceIDs))
		w.Flush()
		fmt.Printf("  %s\n\n", compact(k.Content, 100))
	}
	w.Flush()
	fmt.Printf("\n%d entries\n", len(knowledge))
}

func compactSources(sourceIDs []string) string {
	switch len(sourceIDs) {
	case 0:
		return ""
	case 1:
		return sourceIDs[0]
	default:
		return fmt.Sprintf("%d sources", len(sourceIDs))
	}
}

// --- log ---

func cmdLog(args []string) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
	limit := fs.Int("limit", 20, "max events")
	fs.Parse(args)

	s := openStore(*project, *global, *storeFlag)
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
	fmt.Fprintln(w, "ID\tTYPE\tSOURCE\tSESSION\tTIME")
	for _, e := range events {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.ID, e.Type, e.Source, displaySession(e), e.Timestamp.Format("2006-01-02 15:04"))
		w.Flush()
		fmt.Printf("  %s\n\n", compact(e.Content, 100))
	}
	w.Flush()
	fmt.Printf("\n%d events\n", len(events))
}

func displaySession(e models.Event) string {
	if e.ServerSession != "" {
		return e.ServerSession
	}
	return e.SessionID
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

// --- show ---

func cmdShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
	fs.Parse(args)

	id := fs.Arg(0)
	if id == "" {
		log.Fatal("usage: sekctl show <id>")
	}

	s := openStore(*project, *global, *storeFlag)
	defer s.Close()

	ctx := context.Background()
	if k, err := s.GetKnowledge(ctx, id); err == nil {
		printKnowledgeFull(*k)
		return
	} else if !isNotFound(err) {
		log.Fatalf("show knowledge: %v", err)
	}

	if e, err := s.GetEvent(ctx, id); err == nil {
		printEventFull(*e)
		return
	} else if !isNotFound(err) {
		log.Fatalf("show event: %v", err)
	}

	log.Fatalf("not found: %s", id)
}

func printKnowledgeFull(k models.Knowledge) {
	fmt.Printf("ID:          %s\n", k.ID)
	fmt.Printf("Type:        knowledge\n")
	fmt.Printf("Level:       %s\n", k.Level)
	fmt.Printf("Created:     %s\n", k.CreatedAt.Format(time.RFC3339Nano))
	if len(k.SourceIDs) > 0 {
		fmt.Printf("Sources:     %s\n", strings.Join(k.SourceIDs, ","))
	}
	if k.EventType != "" {
		fmt.Printf("Event type:  %s\n", k.EventType)
	}
	if k.Importance != 0 {
		fmt.Printf("Importance:  %.2f\n", k.Importance)
	}
	fmt.Printf("Usage count: %d\n", k.UsageCount)
	fmt.Println()
	fmt.Println(k.Content)
}

func printEventFull(e models.Event) {
	fmt.Printf("ID:             %s\n", e.ID)
	fmt.Printf("Type:           event\n")
	fmt.Printf("Event type:     %s\n", e.Type)
	fmt.Printf("Source:         %s\n", e.Source)
	fmt.Printf("Session:        %s\n", e.SessionID)
	if e.ServerSession != "" {
		fmt.Printf("Server session: %s\n", e.ServerSession)
	}
	fmt.Printf("Timestamp:      %s\n", e.Timestamp.Format(time.RFC3339Nano))
	fmt.Println()
	fmt.Println(e.Content)
}

func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// --- rm ---

func cmdRemove(args []string) {
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
	fs.Parse(args)

	id := fs.Arg(0)
	if id == "" {
		log.Fatal("usage: sekctl rm <knowledge-id>")
	}

	s := openStore(*project, *global, *storeFlag)
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
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
	fs.Parse(args)

	s := openStore(*project, *global, *storeFlag)
	defer s.Close()

	ctx := context.Background()
	stats, err := s.Stats(ctx)
	if err != nil {
		log.Fatalf("stats: %v", err)
	}

	fmt.Printf("DB path:     %s\n", storePath(*project, *global, *storeFlag))
	fmt.Printf("DB size:     %d KB\n", stats.DBSizeBytes/1024)
	fmt.Printf("Events:      %d\n", stats.EventCount)
	fmt.Printf("Knowledge:   %d\n", stats.KnowledgeCount)
}

// --- gc ---

func cmdGC(args []string) {
	fs := flag.NewFlagSet("gc", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
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

	sp := storePath(*project, *global, *storeFlag)
	if _, err := os.Stat(sp); os.IsNotExist(err) {
		fmt.Println("no store found at", sp)
		return
	}

	s := openStore(*project, *global, *storeFlag)
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
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			afterSep = true
			continue
		}
		if afterSep || !strings.HasPrefix(a, "-") {
			parts = append(parts, a)
		} else {
			flags = append(flags, a)
			if flagNeedsValue(a) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	task = strings.Join(parts, " ")
	return
}

func flagNeedsValue(arg string) bool {
	if strings.Contains(arg, "=") {
		return false
	}
	name := strings.TrimLeft(arg, "-")
	switch name {
	case "project", "store", "llm-provider", "llm-model", "llm-key", "llm-base-url", "max-tokens", "max-entries":
		return true
	default:
		return false
	}
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Fatalf("invalid duration %q: %v", s, err)
	}
	return d
}

// --- diff ---

func cmdDiff(args []string) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
	since := fs.String("since", "", "show records since this duration ago (e.g. 24h, 168h)")
	from := fs.String("from", "", "show records from this timestamp (RFC3339 or YYYY-MM-DD)")
	to := fs.String("to", "", "show records until this timestamp (RFC3339 or YYYY-MM-DD)")
	session := fs.String("session", "", "show records linked to this session or server session")
	limit := fs.Int("limit", 100, "max events and knowledge entries to inspect")
	full := fs.Bool("full", false, "print full content instead of compact rows")
	fs.Parse(args)

	if *session != "" && (*since != "" || *from != "" || *to != "") {
		log.Fatalf("--session cannot be combined with --since, --from, or --to")
	}
	if *session != "" && looksLikeKnowledgeID(*session) {
		log.Fatalf("%q looks like a knowledge id, not a session id; use `sekctl show %s` to inspect its source event/session", *session, *session)
	}
	if *since != "" && *from != "" {
		log.Fatalf("--since and --from are mutually exclusive")
	}

	s := openStore(*project, *global, *storeFlag)
	defer s.Close()

	ctx := context.Background()
	var events []models.Event
	var knowledge []models.Knowledge
	var title string
	var err error

	if *session != "" {
		title = fmt.Sprintf("session %s", *session)
		events, knowledge, err = diffBySession(ctx, s, *session, *limit)
	} else {
		start, end := diffTimeRange(*since, *from, *to)
		title = fmt.Sprintf("%s to %s", start.Format(time.RFC3339), end.Format(time.RFC3339))
		events, knowledge, err = diffByTimeRange(ctx, s, start, end, *limit)
	}
	if err != nil {
		log.Fatalf("diff: %v", err)
	}

	printDiff(title, events, knowledge, *full)
}

func diffTimeRange(since string, from string, to string) (time.Time, time.Time) {
	end := time.Now()
	if to != "" {
		parsed, err := parseTimestamp(to)
		if err != nil {
			log.Fatalf("invalid --to timestamp %q: %v", to, err)
		}
		end = parsed
	}

	if since == "" && from == "" {
		since = "24h"
	}
	if since != "" {
		return end.Add(-parseDuration(since)), end
	}

	start, err := parseTimestamp(from)
	if err != nil {
		log.Fatalf("invalid --from timestamp %q: %v", from, err)
	}
	return start, end
}

func diffByTimeRange(ctx context.Context, s store.Store, start time.Time, end time.Time, limit int) ([]models.Event, []models.Knowledge, error) {
	if limit <= 0 {
		limit = 100
	}

	events, err := s.Query(ctx, limit)
	if err != nil {
		return nil, nil, err
	}
	knowledge, err := s.List(ctx, "", limit)
	if err != nil {
		return nil, nil, err
	}

	events = filterEventsByTime(events, start, end)
	knowledge = filterKnowledgeByTime(knowledge, start, end)
	reverseEvents(events)
	reverseKnowledge(knowledge)
	return events, knowledge, nil
}

func diffBySession(ctx context.Context, s store.Store, session string, limit int) ([]models.Event, []models.Knowledge, error) {
	if limit <= 0 {
		limit = 100
	}

	eventsByID := make(map[string]models.Event)
	for _, fetch := range []func(context.Context, string, int) ([]models.Event, error){
		s.EventsBySession,
		s.EventsByServerSession,
	} {
		events, err := fetch(ctx, session, limit)
		if err != nil {
			return nil, nil, err
		}
		for _, e := range events {
			eventsByID[e.ID] = e
		}
	}

	events := make([]models.Event, 0, len(eventsByID))
	sourceIDs := make(map[string]bool)
	for _, e := range eventsByID {
		events = append(events, e)
		sourceIDs[e.ID] = true
	}
	sortEvents(events)

	allKnowledge, err := s.List(ctx, "", limit)
	if err != nil {
		return nil, nil, err
	}
	knowledge := filterKnowledgeBySources(allKnowledge, sourceIDs)
	reverseKnowledge(knowledge)
	return events, knowledge, nil
}

func filterEventsByTime(events []models.Event, start time.Time, end time.Time) []models.Event {
	var result []models.Event
	for _, e := range events {
		if !e.Timestamp.Before(start) && !e.Timestamp.After(end) {
			result = append(result, e)
		}
	}
	return result
}

func filterKnowledgeByTime(knowledge []models.Knowledge, start time.Time, end time.Time) []models.Knowledge {
	var result []models.Knowledge
	for _, k := range knowledge {
		if !k.CreatedAt.Before(start) && !k.CreatedAt.After(end) {
			result = append(result, k)
		}
	}
	return result
}

func filterKnowledgeBySources(knowledge []models.Knowledge, sourceIDs map[string]bool) []models.Knowledge {
	var result []models.Knowledge
	knownIDs := make(map[string]bool)
	changed := true
	for changed {
		changed = false
		for _, k := range knowledge {
			if knownIDs[k.ID] {
				continue
			}
			if hasAnySource(k.SourceIDs, sourceIDs) {
				result = append(result, k)
				knownIDs[k.ID] = true
				sourceIDs[k.ID] = true
				changed = true
			}
		}
	}
	return result
}

func hasAnySource(sourceIDs []string, wanted map[string]bool) bool {
	for _, id := range sourceIDs {
		if wanted[id] {
			return true
		}
	}
	return false
}

func sortEvents(events []models.Event) {
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].Timestamp.Before(events[j-1].Timestamp); j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
}

func printDiff(title string, events []models.Event, knowledge []models.Knowledge, full bool) {
	obs, lessons, patterns := countKnowledgeLevels(knowledge)
	fmt.Printf("Diff: %s\n", title)
	fmt.Printf("Events: %d\n", len(events))
	fmt.Printf("Knowledge: %d (observations: %d, lessons: %d, patterns: %d)\n\n", len(knowledge), obs, lessons, patterns)

	if len(events) > 0 {
		fmt.Println("Events:")
		for _, e := range events {
			if full {
				printEventFull(e)
				fmt.Println()
			} else {
				fmt.Printf("- %s [%s] %s\n", e.ID, e.Type, compact(e.Content, 90))
			}
		}
		fmt.Println()
	}

	if len(knowledge) > 0 {
		fmt.Println("Knowledge:")
		for _, k := range knowledge {
			if full {
				printKnowledgeFull(k)
				fmt.Println()
			} else {
				fmt.Printf("- %s [%s] %s\n", k.ID, k.Level, compact(k.Content, 100))
			}
		}
	}
}

func countKnowledgeLevels(knowledge []models.Knowledge) (int, int, int) {
	var obs, lessons, patterns int
	for _, k := range knowledge {
		switch k.Level {
		case models.LevelObservation:
			obs++
		case models.LevelLesson:
			lessons++
		case models.LevelPattern:
			patterns++
		}
	}
	return obs, lessons, patterns
}

// --- usage ---

func cmdUsage(args []string) {
	fs := flag.NewFlagSet("usage", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
	top := fs.Int("top", 10, "number of top used knowledge entries")
	limit := fs.Int("limit", 20, "max retrievals or sessions to print")
	sessions := fs.Bool("sessions", false, "show usage grouped by session")
	session := fs.String("session", "", "show retrievals for one session")
	unused := fs.Bool("unused", false, "show retrievals with no reported usage")
	full := fs.Bool("full", false, "print full task/content instead of compact rows")
	fs.Parse(args)

	s := openStore(*project, *global, *storeFlag)
	defer s.Close()

	ctx := context.Background()
	if *sessions {
		items, err := s.UsageBySession(ctx, *limit)
		if err != nil {
			log.Fatalf("usage sessions: %v", err)
		}
		printUsageSessions(items)
		return
	}

	if *session != "" || *unused {
		items, err := s.ListRetrievals(ctx, *session, *unused, *limit)
		if err != nil {
			log.Fatalf("usage retrievals: %v", err)
		}
		printRetrievals(items, *full)
		return
	}

	summary, err := s.UsageSummary(ctx)
	if err != nil {
		log.Fatalf("usage summary: %v", err)
	}
	knowledge, err := s.TopUsedKnowledge(ctx, *top)
	if err != nil {
		log.Fatalf("usage top: %v", err)
	}
	printUsageSummary(*summary, knowledge, *full)
}

func printUsageSummary(summary models.UsageSummary, knowledge []models.Knowledge, full bool) {
	fmt.Printf("Retrievals:         %d\n", summary.Retrievals)
	fmt.Printf("Used retrievals:    %d\n", summary.UsedRetrievals)
	fmt.Printf("Use rate:           %.1f%%\n", percent(summary.UsedRetrievals, summary.Retrievals))
	fmt.Printf("Used marks:         %d\n", summary.UsedMarks)
	fmt.Printf("Knowledge with use: %d\n", summary.KnowledgeWithUse)
	fmt.Printf("Total usage count:  %d\n", summary.TotalUsageCount)

	if len(knowledge) == 0 {
		fmt.Println("\nNo used knowledge yet")
		return
	}

	fmt.Println("\nTop used knowledge:")
	fmt.Printf("%-5s %-41s %-11s %s\n", "USES", "ID", "LEVEL", "CREATED")
	for _, k := range knowledge {
		fmt.Printf("%-5d %-41s %-11s %s\n", k.UsageCount, k.ID, k.Level, k.CreatedAt.Format("2006-01-02 15:04"))
		if full {
			fmt.Printf("  %s\n\n", k.Content)
		} else {
			fmt.Printf("  %s\n\n", compact(k.Content, 100))
		}
	}
}

func printUsageSessions(items []models.SessionUsage) {
	if len(items) == 0 {
		fmt.Println("no retrieval telemetry recorded")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SESSION\tRETRIEVALS\tUSED\tRATE\tMARKS\tLAST SEEN")
	for _, item := range items {
		fmt.Fprintf(w, "%s\t%d\t%d\t%.1f%%\t%d\t%s\n",
			item.SessionID,
			item.Retrievals,
			item.UsedRetrievals,
			percent(item.UsedRetrievals, item.Retrievals),
			item.UsedMarks,
			item.LastSeen.Format("2006-01-02 15:04"),
		)
	}
	w.Flush()
}

func printRetrievals(items []models.RetrievalLog, full bool) {
	if len(items) == 0 {
		fmt.Println("no retrieval telemetry found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSESSION\tTIME\tUSED")
	for _, item := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", item.ID, item.SessionID, item.Timestamp.Format("2006-01-02 15:04"), usedIDCount(item.UsedIDs))
		w.Flush()
		if full {
			fmt.Printf("  %s\n", item.Task)
			if item.UsedIDs != "" && item.UsedIDs != "[]" {
				fmt.Printf("  used: %s\n", item.UsedIDs)
			}
			fmt.Println()
		} else {
			fmt.Printf("  %s\n\n", compact(item.Task, 100))
		}
	}
	w.Flush()
}

func percent(part int, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) * 100 / float64(total)
}

func usedIDCount(raw string) int {
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return 0
	}
	return len(ids)
}

func compact(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func looksLikeKnowledgeID(id string) bool {
	return strings.HasPrefix(id, "obs-") ||
		strings.HasPrefix(id, "lesson-") ||
		strings.HasPrefix(id, "pattern-")
}

// --- prune ---

func cmdPrune(args []string) {
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: cwd)")
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
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

	s := openStore(*project, *global, *storeFlag)
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
	global := fs.Bool("global", false, "use global ~/.sek store")
	storeFlag := fs.String("store", "", "explicit store path")
	llmProvider := fs.String("llm-provider", "openai", "LLM provider")
	llmModel := fs.String("llm-model", "gpt-4o", "LLM model")
	llmKey := fs.String("llm-key", "", "LLM API key")
	llmBaseURL := fs.String("llm-base-url", "", "LLM API base URL")
	maxTokens := fs.Int("max-tokens", 1000, "max tokens in response")
	maxEntries := fs.Int("max-entries", 5, "max entries")
	includeTrace := fs.Bool("trace", false, "include source trace and score breakdown")

	task, flagArgs := splitArgs(args)
	fs.Parse(flagArgs)
	if task == "" {
		log.Fatal("usage: sekctl query <task description>")
	}

	cfg := llm.Config{
		Provider: llm.ProviderType(*llmProvider),
		APIKey:   *llmKey,
		BaseURL:  *llmBaseURL,
		Model:    *llmModel,
	}
	if err := llm.ResolveAPIKey(&cfg, os.Getenv("SEK_LLM_KEY")); err != nil {
		log.Fatal(err)
	}
	provider, err := llm.NewProvider(cfg)
	if err != nil {
		log.Fatalf("llm: %v", err)
	}
	embedder := llm.NewOpenAIEmbedder(cfg.APIKey, cfg.BaseURL, cfg.Model)

	s := openStore(*project, *global, *storeFlag)
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
		fmt.Println(trace.FormatKnowledge(k, *includeTrace))
		fmt.Println()
	}
	fmt.Printf("total tokens: %d\n", result.TotalTokens)
}
