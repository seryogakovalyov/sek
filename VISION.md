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

---

## Experience Lifecycle

```
Interaction

↓

Events

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

---

## Responsibilities

SEK has four responsibilities.

### 1. Capture

Capture useful execution traces.

Examples:

* requests
* responses
* tool usage
* failures
* decisions
* implementation choices
* successful fixes

---

### 2. Distill

Convert raw execution traces into reusable observations.

Examples:

* "Anthropic system prompt must be sent separately."

* "This project uses user-level systemd services."

* "Provider interface should receive context.Context."

---

### 3. Organize

Maintain durable experience in a local store.

Experience remains durable.

The prompt remains small.

Memory is complete.

Context is selective.

---

### 4. Reuse

When an agent starts working on a task,

SEK provides only the experience that is relevant for the current problem and fits the available context budget.

The agent receives previous engineering knowledge instead of rediscovering it.

---

## Design Principles

SEK should never replace the coding agent.

SEK should never replace the LLM provider.

SEK should never become another orchestration framework.

SEK exists only to provide durable experience.

---

## Modules

A module defines the type of memory SEK is organizing.

The current built-in module is `engineering`.

It uses:

* knowledge levels: `observation`, `lesson`, `pattern`
* event types: `request`, `response`, `tool_usage`, `failure`, `decision`, `implementation_choice`, `successful_fix`

Modules are vocabulary, not partitioning. They should not force agents to pass `project_id` or `namespace` through MCP calls.

The storage rule stays simple:

> one SQLite store = one context

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

---

## Long-Term Goal

A coding agent should gradually develop engineering experience.

Not by retraining.

Not by fine-tuning.

But by continuously accumulating, organizing and reusing what it has already learned while working on the project.

That accumulated experience is SEK.
