# SEK — Experience Runtime

## Vision

SEK is not another coding agent.

SEK is not another LLM provider.

SEK is not another RAG system.

SEK is an **experience runtime** that gives existing agents persistent reusable memory.

Its purpose is simple:

> Allow AI agents to accumulate, preserve, search and reuse experience across many independent interactions.

The default SEK module is engineering memory for coding agents. A coding agent should gradually become better at working on a codebase, not because its model changes, but because its accumulated experience becomes available whenever it is useful.

---

## Core Idea

Every interaction between an agent and a context can produce experience.

Normally this experience disappears forever.

SEK treats every interaction as a potential source of reusable engineering knowledge.

Instead of storing conversations, SEK stores experience.

SEK should collect experience from multiple observation surfaces:

* **active memory** — explicit MCP tool calls such as `capture_event`;
* **passive observation** — session/runtime signals such as git snapshots;
* **future adapters** — proxies, wrappers, hooks, or SDK integrations when they provide useful signals.

These surfaces are collectors. They should feed the same event pipeline instead of becoming separate memory systems.

---

## Experience Lifecycle

```
Interaction

↓

Collectors

↓

Events / session artifacts

↓

Observation

↓

Lessons

↓

Patterns

↓

Reusable Experience
```

Raw execution traces are not the final product.

They are transformed into increasingly compact and reusable knowledge.

Session artifacts are not knowledge by themselves. A changed-file list or diff stat can prove that work happened, but it cannot reliably explain why the work mattered. They are evidence for future analysis, not automatic memory.

---

## Responsibilities

SEK has five responsibilities.

### 1. Capture

Capture useful execution traces and session signals.

Examples:

* requests
* responses
* tool usage
* failures
* decisions
* implementation choices
* successful fixes
* git snapshots
* changed files
* retrieval usage

The highest-quality captures usually include intent. Passive signals improve coverage when an agent forgets to write memory, but they must be analyzed carefully before becoming knowledge.

---

### 2. Observe

Track the runtime context without requiring a separate daemon.

The embedded session manager records lightweight session state such as:

* session id
* project directory
* git HEAD
* dirty status
* changed files
* diff stat
* interrupted sessions

This is intentionally lightweight. Full patch capture, file watching, and automatic memory extraction are future layers, not part of the first observer.

---

### 3. Distill

Convert raw execution traces into reusable observations.

Examples:

* "Anthropic system prompt must be sent separately."

* "This project uses user-level systemd services."

* "Provider interface should receive context.Context."

---

### 4. Organize

Maintain durable experience in a local store.

Experience remains durable.

The prompt remains small.

Memory is complete.

Context is selective.

---

### 5. Reuse

When an agent starts working on a task,

SEK provides only the experience that is relevant for the current problem and fits the available context budget.

The agent receives previous engineering knowledge instead of rediscovering it.

---

## Design Principles

SEK should never replace the coding agent.

SEK should never replace the LLM provider.

SEK should never become another orchestration framework.

SEK should not require users to launch a separate wrapper for the normal case.

SEK exists to provide durable experience through a small runtime and explicit integrations.

---

## Runtime Layers

SEK is built as a runtime pipeline, not as one transport.

```
Collectors
  ├─ MCP tools
  ├─ SessionManager
  └─ future proxy/wrapper/hook adapters
        ↓
Event and artifact pipeline
        ↓
Redaction
        ↓
Distillation
        ↓
Module routing
        ↓
Knowledge store
        ↓
Retrieval and feedback
```

Current state:

* MCP tools provide active memory.
* SessionManager records lightweight git snapshots in `session_log`.
* SessionManager does not yet store full patches or generate knowledge from diffs.
* Retrieval telemetry records what knowledge was returned and what was later marked useful.

---

## Modules

A module defines the type of memory SEK is organizing.

The current built-in default is `engineering`, with shadow routing for additional module candidates.

It uses:

* knowledge levels: `observation`, `lesson`, `pattern`
* event types: `request`, `response`, `tool_usage`, `failure`, `decision`, `implementation_choice`, `successful_fix`

Modules are vocabulary, not partitioning. They should not force agents to pass `project_id` or `namespace` through MCP calls.

The storage rule stays simple:

> one SQLite store = one context

Module routing should be inferred from content, not supplied by agents. Agents should focus on capturing reusable experience; SEK decides how that experience is organized.

Current module routing status:

* routing runs in shadow mode after observations are created;
* routing decisions are stored as telemetry;
* `knowledge` does not store `module` yet;
* retrieval is not module-aware yet;
* lifecycle rules are not module-aware yet.

---

## Transport

The transport layer is an implementation detail.

Possible transports include:

* MCP
* HTTP proxy
* SDK
* CLI integration

The transport may change.

The experience model should not.

The default runtime is one `sekd` process. It can run over stdio for MCP clients or Streamable HTTP for clients that prefer a long-lived server. The session manager should be reusable by both modes.

Wrappers and LLM API proxies may become useful collectors, but they should remain optional adapters, not the foundation of SEK.

---

## Current Boundaries

SEK should be explicit about what it does not do yet.

Current limitations:

* It does not automatically infer reusable knowledge from git diffs.
* It does not store full patch artifacts in `session_log`.
* It does not make retrieval module-aware.
* It does not apply module-aware lifecycle rules.
* It does not replace agent instructions such as `AGENTS.md`; those remain useful for active memory quality.

Near-term architectural focus:

1. keep active MCP memory reliable;
2. make passive session observation useful without storing noisy transcripts;
3. validate module routing before changing retrieval or storage schema;
4. add lifecycle only after knowledge conflicts become visible;
5. prefer inspectable candidates before automatic memory extraction.

---

## Long-Term Goal

A coding agent should gradually develop engineering experience.

Not by retraining.

Not by fine-tuning.

But by continuously accumulating, organizing and reusing what it has already learned while working on the project.

That accumulated experience is SEK.
