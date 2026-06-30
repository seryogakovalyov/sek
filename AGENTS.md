# SEK: Experience Runtime

You have access to SEK via MCP tools (`capture_event`, `query_experience`, `report_usage`, `list_knowledge`).
All tools write to a single store context — no `project_id` parameter needed.
SEK remembers what you learn across sessions. Use it deliberately.

## Memory policy

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

## When to call `capture_event`

Call AFTER something notable happens:

Before completing a task, check whether it produced reusable experience.
If yes, call `capture_event`, even if the user did not explicitly ask you to remember it.

| Trigger | What to put in `content` |
|---|---|
| **Bug/error encountered** | Error message, what you tried, what caused it |
| **Bug fixed** | Root cause, how you fixed it, relevant file paths |
| **Design decision** | Options considered, why you chose this one |
| **Library/pattern chosen** | What, why, any gotchas discovered |
| **Context convention discovered** | e.g. "tests live in tests/ dir, use pytest fixtures" |
| **Local AI setup discovered** | Model server, embedding endpoint, runtime/GPU flags, model-specific gotchas |
| **Agent behavior discovered** | How a coding agent reacts to instructions, tools, MCP, or project files |
| **Team/process convention discovered** | Release, review, communication, ownership, or workflow norms |
| **Durable working preference discovered** | Stable user/team work preference that changes future task handling |
| **Novel tool usage** | Exact command, what it accomplished |

Do NOT call for:
- Routine "I wrote file X" — too trivial
- Every tool call in a sequence — wait for the meaningful outcome
- Obvious boilerplate patterns (unless they have a non-obvious twist)
- Sensitive, private, or short-lived personal details unless the user explicitly asks to remember them
- Guesses about people or organizations that are not directly useful for future work
- Conversation transcripts or summaries that do not preserve reusable lessons

### Content format

Be concrete. Include:
- File paths
- Error messages
- Command examples
- The "why" behind decisions

Bad: `Fixed the test`
Good: `Fixed TestFoo — was failing because func init() ran before mock setup. Moved mock init to TestMain. File: foo_test.go`

## When to call `query_experience`

**ALWAYS call before answering any question related to the current store context.** Do NOT answer from your own training data — SEK has the actual accumulated experience.

1. **User asks "как / how to / why / что лучше / which"** → query_experience immediately
2. **User asks about a file, library, pattern or decision** → query_experience
3. **User reports an error or bug** → query_experience(paste error)
4. **New task may benefit from prior experience** → query_experience(task description)
5. **Before making any decision** → query_experience(options, tradeoffs)

If you skip the tool call and guess, you will give wrong context-specific answers.

## When to call `report_usage`

Call AFTER you use knowledge returned by `query_experience` — this teaches SEK which results were actually useful, improving future scoring.

1. **You applied the knowledge** → call `report_usage(retrieval_id, knowledge_id)` with the ID from the query_experience response
2. **You did NOT use it** → don't call — unused entries naturally decay via scoring

## Session digest (automatic)

When the session ends, SEK automatically creates a summary lesson from all events.
You don't need to call anything extra — just make sure you `capture_event` for key moments during the session.
