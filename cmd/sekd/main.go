package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/seryogakovalyov/sek/internal/config"
	"github.com/seryogakovalyov/sek/internal/distill"
	"github.com/seryogakovalyov/sek/internal/llm"
	"github.com/seryogakovalyov/sek/internal/mcp"
	"github.com/seryogakovalyov/sek/internal/store"
	"github.com/seryogakovalyov/sek/internal/storepath"
)

func main() {
	// CLI flags
	projectDir := flag.String("project", "", "project directory (default: cwd)")
	dataDir := flag.String("data-dir", "", "data directory for global store (default: ~/.sek)")
	useGlobal := flag.Bool("global", false, "use global store in ~/.sek/store.db")
	storePathExplicit := flag.String("store", "", "explicit store path (overrides --project and --global)")
	httpAddr := flag.String("http", "", "Streamable HTTP address (e.g. :9090)")
	stdio := flag.Bool("stdio", false, "force stdio transport, overriding config mcp.http_addr")
	llmProvider := flag.String("llm-provider", "openai", "LLM provider: openai or anthropic")
	llmModel := flag.String("llm-model", "gpt-4o", "LLM model name")
	llmKey := flag.String("llm-key", "", "LLM API key")
	llmBaseURL := flag.String("llm-base-url", "", "LLM API base URL")
	configPath := flag.String("config", "", "config file path (default: .sek/config.json)")
	flag.Parse()

	// 1. Resolve project directory
	if *projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("getwd: %v", err)
		}
		*projectDir = cwd
	}

	// 2. Load config file
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = config.DefaultPath(*projectDir)
	}
	cfg := &config.Config{ProjectDir: *projectDir}
	if _, err := os.Stat(cfgPath); err == nil {
		loaded, err := config.Load(cfgPath)
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
		cfg = loaded
		if cfg.ProjectDir == "" {
			cfg.ProjectDir = *projectDir
		}
		log.Printf("loaded config from %s", cfgPath)
	}

	// 3. CLI overrides config (only explicitly set flags override)
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "project":
			cfg.ProjectDir = *projectDir
		case "data-dir":
			cfg.DataDir = *dataDir
		case "global":
			// handled at store path resolution time
		case "store":
			// handled at store path resolution time
		case "http":
			cfg.MCP.HTTPAddr = *httpAddr
		case "stdio":
			if *stdio {
				cfg.MCP.HTTPAddr = ""
			}
		case "llm-provider":
			cfg.LLM.Provider = llm.ProviderType(*llmProvider)
		case "llm-model":
			cfg.LLM.Model = *llmModel
		case "llm-key":
			cfg.LLM.APIKey = *llmKey
		case "llm-base-url":
			cfg.LLM.BaseURL = *llmBaseURL
		}
	})
	cfg.Normalize()
	effectiveStorePath := *storePathExplicit
	if effectiveStorePath == "" && !*useGlobal {
		effectiveStorePath = cfg.Store.Path
	}
	pathOpts := storepath.Options{
		ProjectDir:   cfg.ProjectDir,
		DataDir:      cfg.DataDirPath(),
		ExplicitPath: effectiveStorePath,
		Global:       *useGlobal,
	}
	if storepath.RequiresProject(pathOpts) {
		if err := validateProjectDir(cfg.ProjectDir); err != nil {
			log.Fatal(err)
		}
	}

	// 4. API key fallback
	if cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = os.Getenv("SEK_LLM_KEY")
	}
	if cfg.LLM.APIKey == "" {
		log.Fatal("LLM API key required: set --llm-key or SEK_LLM_KEY")
	}

	// 5. Determine store path. Priority: --store > --global > config store.path > --project.
	storePath, err := storepath.Resolve(pathOpts)
	if err != nil {
		log.Fatalf("resolve store path: %v", err)
	}
	if *useGlobal {
		log.Printf("global store: %s", storePath)
	}

	if err := os.MkdirAll(filepath.Dir(storePath), 0755); err != nil {
		log.Fatalf("create store dir: %v", err)
	}

	// 6. Open store
	st, err := store.NewSQLite(storePath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	// 7. LLM provider + embedder
	provider, err := llm.NewProvider(cfg.LLM)
	if err != nil {
		log.Fatalf("llm: %v", err)
	}

	embedder := llm.NewOpenAIEmbedder(cfg.LLM.APIKey, cfg.LLM.BaseURL, cfg.LLM.Model)

	// 8. Session ID
	sessionID := generateSessionID()
	log.Printf("session ID: %s", sessionID)

	// 9. Run
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if cfg.MCP.HTTPAddr != "" {
		log.Printf("starting HTTP server on %s", cfg.MCP.HTTPAddr)
		if err := mcp.ServeHTTP(ctx, st, provider, embedder, cfg.LLM.Model, sessionID, cfg.MCP.HTTPAddr); err != nil {
			log.Fatalf("mcp sse: %v", err)
		}
	} else {
		// stdio mode
		if err := mcp.Serve(ctx, st, provider, embedder, cfg.LLM.Model, sessionID); err != nil {
			log.Fatalf("mcp: %v", err)
		}
	}

	// 10. Session digest
	log.Println("session ended, running session digest...")
	digestCtx, digestCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer digestCancel()
	distill.SessionDigest(digestCtx, st, provider, embedder, cfg.LLM.Model, sessionID)
}

func generateSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "sek-" + hex.EncodeToString(b)
}

func validateProjectDir(projectDir string) error {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return err
	}
	if abs == string(filepath.Separator) {
		return fmt.Errorf("cannot use %q as project directory; pass --project or configure the MCP client cwd", abs)
	}
	return nil
}
