# SEK — Project Experience Runtime

## Vision

SEK is not another coding agent.

SEK is not another LLM provider.

SEK is not another RAG system.

SEK is an **experience runtime** that gives existing coding agents persistent project memory.

Its purpose is simple:

> Allow AI coding agents to accumulate, preserve, search and reuse engineering experience across many independent interactions.

The agent should gradually become better at working on a project, not because its model changes, but because its accumulated project experience becomes available whenever it is useful.

---

## Core Idea

Every interaction between an agent and a project produces experience.

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

Project Experience
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

Maintain project-local experience.

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

SEK exists only to provide durable project experience.

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

A coding agent should gradually develop project-specific engineering experience.

Not by retraining.

Not by fine-tuning.

But by continuously accumulating, organizing and reusing what it has already learned while working on the project.

That accumulated experience is SEK.
