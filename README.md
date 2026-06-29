# SEK — Experience Runtime

SEK gives AI agents persistent experience. Its default use case is engineering memory for coding agents: it preserves decisions, fixes, patterns, and project conventions across sessions, distills them into reusable observations/lessons/patterns, and surfaces them on future tasks via MCP.

SEK is not a coding agent, LLM provider, or external RAG system. It is a local experience runtime: a small Go binary, a SQLite store, and MCP tools for capture and retrieval. Project-local memory is one deployment mode, not the core abstraction.

> Full vision: [VISION.md](VISION.md).

## Features

- MCP server via stdio by default and Streamable HTTP via `--http`.
- Agent event capture via `capture_event`.
- Event distillation into observations via LLM.
- Promotion: observations → lessons → patterns.
- Vector search with keyword fallback.
- Source trace: optional output with `source_ids`, relevance reasons, and scoring metadata.
- Secret redaction before saving events/knowledge.
- Retrieval telemetry and feedback loop via `report_usage`.
- `sekctl` for browsing, searching, GC, and store maintenance.

## Status

Under active development. The core workflow is functional:

1. agent calls `query_experience` before a project task;
2. agent calls `capture_event` after an important decision, error, or fix;
3. SEK saves the raw event, distills an observation, and indexes its embedding;
4. on stdio session shutdown, SEK creates a session digest;
5. future sessions receive relevant experience from `.sek/store.db`.

## Installation

Requirements:

- Go 1.22+;
- LLM endpoint for chat completions;
- embeddings endpoint for vector search (optional, keyword fallback works without it).

Build:

```bash
go build -o sekd ./cmd/sekd
go build -o sekctl ./cmd/sekctl
```

Verify:

```bash
go test ./...
./sekctl status
```

In sandbox or read-only environments, Go cache may need an explicit path:

```bash
GOCACHE=/tmp/sek-go-cache go test ./...
```

## Quick Start

Run the MCP server for the current project:

```bash
./sekd --llm-key "$OPENAI_API_KEY"
```

Run with an explicit project path:

```bash
./sekd --project /path/to/project --llm-key "$OPENAI_API_KEY"
```

Run with a local OpenAI-compatible endpoint:

```bash
./sekd \
  --llm-key none \
  --llm-provider openai \
  --llm-base-url http://localhost:8000/v1 \
  --llm-model Qwen3.6-35B-A3B-Unsloth
```

For llama.cpp embeddings, add these flags on the llama.cpp side:

```bash
--embeddings --pooling mean
```

## Configuration

`sekd` reads `.sek/config.json` if it exists. CLI flags override config values.

| Flag | Default | Description |
|---|---|---|
| `--project` | `cwd` | Project directory (default: current directory) |
| `--global` | `false` | Use global store in `~/.sek/store.db` |
| `--store` | empty | Explicit store path (overrides `--project` and `--global`) |
| `--data-dir` | `~/.sek` | Global store data directory |
| `--config` | `.sek/config.json` | Config file path |
| `--http` | empty | Streamable HTTP address, e.g. `:9090`; defaults to stdio |
| `--stdio` | `false` | Force stdio even if config sets `mcp.http_addr` |
| `--llm-provider` | `openai` | `openai` or `anthropic` |
| `--llm-model` | `gpt-4o` | Model name for chat completions and embeddings |
| `--llm-key` | `SEK_LLM_KEY` | API key; use `none` for local servers |
| `--llm-base-url` | empty | Custom API base URL |

## Storage Modes

SEK uses a single SQLite file as one context. There is no internal `project_id`.

### Per-project store

Default mode:

```bash
./sekd --project /path/to/project
```

Data lives in:

```text
/path/to/project/.sek/store.db
```

This is the primary mode for coding agents: memory lives alongside the project, easy to move, delete, and back up.

### Global store

```bash
./sekd --global
```

Data lives in:

```text
~/.sek/store.db
```

A single machine-wide memory pool with no project partitioning. Suitable for local LLM setup notes, system gotchas, shared preferences, and reusable patterns.

## Connecting to Agents

SEK uses the MCP process's working directory as the project by default. This works well when the agent launches the MCP server from the repo root. If the agent does not guarantee `cwd` or spawns a subprocess from a system directory, pass an explicit `--project /path/to/project`.

Do not use `--project .` unless the MCP client sets `cwd` to the project root — `.` resolves relative to the agent's process, which may point elsewhere. For safety, `sekd` refuses to use `/` as a project directory and asks for `--project` or a proper `cwd`.

Recommendations:

- project-local MCP config or client `cwd` set to project root: use `--project .`;
- user-level/global MCP config without guaranteed `cwd`: use absolute `--project /path/to/project`;
- machine-wide shared memory: use `--global`.

### Project Instructions

For stable capture behavior, add an `AGENTS.md` file to the project or merge its SEK section into your existing agent instructions. Tool descriptions help, but project-level instructions are more likely to shape the agent workflow.

Recommended rules:

- call `query_experience` before context-specific tasks;
- before the final response, call `capture_event` if the task produced reusable experience;
- call `report_usage` after applying a returned knowledge entry;
- capture concrete paths, commands, errors, root causes, and rationale;
- skip routine edits and noisy per-tool-call logging.

### Claude Code

```json
{
  "mcpServers": {
    "sek": {
      "command": "/path/to/sekd",
      "args": ["--llm-key", "$KEY", "--project", "/path/to/project"]
    }
  }
}
```

### Cursor

Settings → MCP → Add:

- Name: `SEK`
- Type: `command`
- Command: `/path/to/sekd --llm-key $KEY --project /path/to/project`

### Opencode

```jsonc
"mcp": {
  "sekd": {
    "type": "local",
    "command": ["/path/to/sekd", "--llm-key", "$KEY", "--project", "/path/to/project"],
    "enabled": true
  }
}
```

## MCP Tools

| Tool | Purpose |
|---|---|
| `capture_event` | Save a notable event and trigger distillation into an observation |
| `query_experience` | Find relevant experience for the current task |
| `report_usage` | Report that a retrieved knowledge entry was actually useful |
| `list_knowledge` | List stored knowledge entries |

### `capture_event`

Parameters:

| Parameter | Required | Description |
|---|---|---|
| `event_type` | yes | `request`, `response`, `tool_usage`, `failure`, `decision`, `implementation_choice`, `successful_fix` |
| `source` | yes | Event source: `codex`, `opencode`, `cursor`, `claude-code` |
| `content` | yes | Detailed event description: commands, files, errors, rationale |
| `session_id` | no | Custom session ID; empty uses server session |

Capture only reusable experience: errors, fixes, decisions, important commands, discovered conventions. Do not capture every intermediate tool call.

### `query_experience`

Parameters:

| Parameter | Required | Description |
|---|---|---|
| `task` | yes | Question or task description |
| `max_tokens` | no | Maximum response size, default `1000` |
| `max_entries` | no | Maximum number of entries, default `5` |
| `include_trace` | no | Include source trace and score breakdown, default `false` |

The response includes compact formatted knowledge entries and a `retrieval_id` for `report_usage`. Set `include_trace=true` when debugging retrieval relevance.

### `report_usage`

Parameters:

| Parameter | Required | Description |
|---|---|---|
| `retrieval_id` | yes | ID from the `query_experience` response |
| `knowledge_id` | yes | ID of the knowledge entry that was used |

`report_usage` updates telemetry and increments `usage_count`, which factors into scoring.

### `list_knowledge`

Parameters:

| Parameter | Required | Description |
|---|---|---|
| `level` | no | `observation`, `lesson`, `pattern` |
| `limit` | no | Maximum number of entries |

## CLI

`sekctl` operates the store directly. Basic commands do not need an LLM; `query` requires an LLM endpoint.

```bash
sekctl init
sekctl status

sekctl list
sekctl list --level lesson
sekctl list --limit 50

sekctl log --limit 10
sekctl rm obs-abc123

sekctl gc --older-than 720h
sekctl gc --before 2026-06-28T21:48:00+03:00 --dry-run

sekctl prune
sekctl prune --force

sekctl query "how is CI configured here?" --llm-key "$KEY"
sekctl list --global
```

`sekctl list` and `sekctl log` fetch the last N entries but print them in chronological order: oldest first, newest last.

| Command | Key flags | Description |
|---|---|---|
| `init` | `--project`, `--global`, `--store` | Create `.sek/` and store |
| `status` | `--project`, `--global`, `--store` | Show counters and DB size |
| `list` | `--project`, `--global`, `--store`, `--level`, `--limit` | Show knowledge entries |
| `log` | `--project`, `--global`, `--store`, `--limit` | Show raw events |
| `query` | `--project`, `--global`, `--store`, `--llm-*`, `--max-tokens`, `--max-entries`, `--trace` | Find experience via reuse engine |
| `rm <id>` | `--project`, `--global`, `--store` | Delete a knowledge entry |
| `gc` | `--project`, `--global`, `--store`, `--older-than`, `--before`, `--dry-run` | Delete old entries, retrieval logs, and orphan-derived knowledge |
| `prune` | `--project`, `--global`, `--store`, `--force` | Delete all events and knowledge from store |

## Architecture

SEK is a generic experience runtime. The current built-in module is `engineering`: reusable coding-agent experience with the default knowledge levels (`observation`, `lesson`, `pattern`) and event types (`request`, `response`, `tool_usage`, `failure`, `decision`, `implementation_choice`, `successful_fix`).

Modules describe the type of memory and its vocabulary. They do not introduce public `namespace` or `project_id` parameters, and they do not change the storage schema. A SQLite store remains one context.

### Module Routing Principles

Module routing answers one question:

> What kind of memory is this observation?

This is different from where the event came from. A web UI, MCP client, CLI command, or agent name is a source/channel. The module is determined by the meaning of the distilled knowledge.

The routing layer is intentionally not part of the public MCP API yet. Agents should not pass `module`, `namespace`, or `project_id` in tool calls. The next step is to test whether the distillation model can classify observations consistently before adding schema or retrieval changes.

The test fixture for this lives in `internal/distill/testdata/golden_module_routing.json`.

Run deterministic prompt/parser tests with `go test ./...`. To run the same routing cases against a real model, opt in explicitly:

```bash
SEK_MODULE_ROUTE_EVAL=1 \
SEK_LLM_PROVIDER=openai \
SEK_LLM_KEY=none \
SEK_LLM_BASE_URL=http://localhost:8000/v1 \
SEK_LLM_MODEL=Qwen3.6-35B-A3B-Unsloth \
go test ./internal/distill -run TestModuleRouteGoldenCasesWithRealModel
```

Initial module candidates:

| Module | Use for |
|---|---|
| `engineering` | Code, architecture, bugs, tests, build systems, APIs, repository conventions |
| `local-ai` | Local model serving, llama.cpp/vLLM, embeddings endpoints, GPU/runtime setup, model quirks |
| `agent-behavior` | How Codex, Claude Code, Cursor, Opencode, or other agents use instructions and tools |
| `personal` | Durable user preferences, working style, recurring constraints |
| `company` | Team/company process, release policy, ownership, communication norms |

Examples for future golden evals:

| Observation | Expected module | Why |
|---|---|---|
| `Codex MCP may start sekd from /, so --global must not validate project cwd.` | `engineering` | It is a storage/runtime bug fix in the codebase |
| `llama.cpp embeddings require --embeddings --pooling mean for SEK vector search.` | `local-ai` | It is local model serving setup |
| `Claude Code captures events reliably after AGENTS.md includes an explicit final-response checkpoint.` | `agent-behavior` | It describes agent behavior under instructions |
| `The user prefers concise recommendations before implementation when tradeoffs are unclear.` | `personal` | It is a durable user working preference |
| `The AI initiatives group wants repo announcements framed as open-source runtime feedback requests.` | `company` | It is a communication/process convention |

For now, the built-in `engineering` module remains the default. If routing confidence is low, prefer `engineering` over inventing a new module. Store source/channel separately from module when that data becomes useful.

Current state:

- module routing runs in shadow mode after an observation is saved;
- routing decisions are stored as telemetry in `module_route_log`;
- `knowledge` does not store `module` yet;
- retrieval is not module-aware yet;
- MCP tools do not accept `module`, and should stay that way until routing quality is proven.

```text
MCP client
  ├─ stdio
  └─ Streamable HTTP (--http :9090)
        ↓
SEK MCP server
  ├─ capture service
  ├─ distill pipeline
  │   ├─ LLM chat completion → observation
  │   ├─ semantic deduplication
  │   └─ promotion: observation → lesson → pattern
  ├─ embedder → /v1/embeddings
  ├─ reuse engine
  │   ├─ vector search
  │   ├─ keyword fallback
  │   └─ scoring: similarity + recency + importance + usage
  ├─ SQLite store: events, knowledge, retrieval_log
  └─ session digest on shutdown
```

### Data lifecycle

```text
agent event
  → capture_event
  → events
  → distill
  → knowledge observation
  → embed
  → retrieval through query_experience
  → report_usage feedback
```

On stdio session shutdown, SEK collects the current `server_session` events and generates a session digest at the `lesson` level if enough events exist.

### Retrieval

`query_experience` first attempts to embed the task and run a vector search. If embeddings are unavailable, it falls back to keyword search. Results are ranked by:

- cosine similarity;
- recency boost;
- importance by `event_type`;
- usage boost from `report_usage`.

## Stack

- Go — two small binaries: `sekd` and `sekctl`.
- `modernc.org/sqlite` — pure Go SQLite, no CGo.
- `mark3labs/mcp-go` — MCP protocol and transports.
- OpenAI-compatible or Anthropic API — chat completions.
- OpenAI-compatible embeddings endpoint — semantic retrieval.

## Development

```bash
go test ./...
go vet ./...
go build -o sekd ./cmd/sekd
go build -o sekctl ./cmd/sekctl
```

Cross-compilation:

```bash
GOOS=linux GOARCH=arm64 go build -o sekd-linux-arm64 ./cmd/sekd
GOOS=darwin GOARCH=amd64 go build -o sekd-darwin-amd64 ./cmd/sekd
```

Local artifacts `sekd`, `sekctl`, and `.sek/` are not committed.

## Roadmap

### Phase 1 — Core MVP ✅

- [x] SQLite event store
- [x] LLM abstraction: OpenAI-compatible and Anthropic
- [x] MCP server via stdio
- [x] Distillation: event → observation
- [x] Embeddings via `/v1/embeddings`
- [x] Vector search with cosine similarity
- [x] Keyword fallback

### Phase 2 — Distillation Quality ✅

- [x] Knowledge levels: observation → lesson → pattern
- [x] Semantic deduplication: cosine > 0.95 + merge `source_ids`
- [x] Scoring: similarity + recency + importance
- [x] Automatic pattern discovery via clustering + LLM composition

### Phase 3 — Integration & Management ✅

- [x] `sekctl`: `init`, `status`, `list`, `log`, `query`, `rm`, `gc`, `prune`
- [x] Session digest on shutdown
- [x] `.sek/config.json` + CLI overrides
- [x] Global store: `--global`
- [x] Streamable HTTP transport: `--http :9090`
- [x] GC by TTL: `sekctl gc --older-than 720h`

### Phase 4 — Memory Quality & Feedback Loop

- [x] Secret redaction before saving events/knowledge
- [x] Golden evals for distillation prompt
- [x] Source trace for knowledge entries
- [x] `sekctl gc --before <timestamp>` + orphan cleanup
- [x] Retrieval telemetry via `retrieval_log`
- [x] Feedback loop via `report_usage` and `usage_count`
- [ ] Knowledge lifecycle: `supersedes`, `conflicts_with`, `deprecated_at`
- [ ] Automatic cleanup of stale observations
- [ ] `sekctl --help` with exit code 0
- [ ] MCP resources for read-only knowledge
- [ ] Experience diff between sessions
- [ ] Knowledge export/import between stores
- [ ] Module-aware lifecycle rules

### Phase 5 — Ecosystem

- [ ] Go SDK
- [ ] Plugin API for custom distillation stages
- [ ] GitHub Action
- [ ] VS Code extension
- [ ] Retrieval benchmarks: recall@k
- [ ] `sek doctor`: LLM/MCP setup diagnostics
- [ ] Project onboarding helper for AGENTS.md and MCP snippet
