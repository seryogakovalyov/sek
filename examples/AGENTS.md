# Experience Runtime

You have access to SEK via MCP tools (`capture_event`, `query_experience`, `report_usage`, `list_knowledge`).
All tools write to a single store context — no `project_id` parameter needed.
SEK remembers what you learn across sessions. Use it deliberately.

SEK automatically classifies captured experience into knowledge modules.
Focus on capturing concrete reusable experience, not choosing categories.
Do not include category labels or routing hints in captured content.

SEK is not a conversation log. Capture reusable experience, not transcripts.
Capture only information that is likely to save future work.

Reusable experience may include:

- Code, architecture, bugs, tests, build systems, APIs, and repository conventions
- Local model serving, llama.cpp/vLLM, embeddings endpoints, GPU/runtime setup, and model quirks
- How coding agents use instructions, tools, MCP, and project files
- Durable working preferences, working style, and recurring constraints
- Team process, release policy, ownership, communication, and workflow norms

A good capture should answer:

> What should a future agent know so it does not repeat this work or make the same mistake?

Before starting a task that may benefit from prior experience, call `query_experience`.

Before completing a task, check whether it produced reusable experience.
If you fixed a bug, made an architectural decision, discovered a project convention, chose a library/pattern, or found a
reusable gotcha, call `capture_event` even if the user did not explicitly ask.

When using knowledge returned by `query_experience`, call `report_usage`.

Do not capture routine file edits or every tool call. Capture only reusable lessons with concrete paths, commands, errors,
and rationale. Do not capture sensitive, private, or short-lived personal details unless the user explicitly asks you to
remember them. Do not capture conversation transcripts or summaries that do not preserve reusable lessons.
