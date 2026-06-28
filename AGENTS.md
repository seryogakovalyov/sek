# SEK: Project Experience Runtime

You have access to SEK via MCP tools (`capture_event`, `query_experience`, `report_usage`, `list_knowledge`).
SEK remembers what you learn across sessions. Use it deliberately.

## When to call `capture_event`

Call AFTER something notable happens:

| Trigger | What to put in `content` |
|---|---|
| **Bug/error encountered** | Error message, what you tried, what caused it |
| **Bug fixed** | Root cause, how you fixed it, relevant file paths |
| **Design decision** | Options considered, why you chose this one |
| **Library/pattern chosen** | What, why, any gotchas discovered |
| **Project convention discovered** | e.g. "tests live in tests/ dir, use pytest fixtures" |
| **Novel tool usage** | Exact command, what it accomplished |

Do NOT call for:
- Routine "I wrote file X" — too trivial
- Every tool call in a sequence — wait for the meaningful outcome
- Obvious boilerplate patterns (unless they have a non-obvious twist)

### Content format

Be concrete. Include:
- File paths
- Error messages
- Command examples
- The "why" behind decisions

Bad: `Fixed the test`
Good: `Fixed TestFoo — was failing because func init() ran before mock setup. Moved mock init to TestMain. File: foo_test.go`

## When to call `query_experience`

**ALWAYS call before answering any project-related question.** Do NOT answer from your own training data — SEK has the actual project experience.

1. **User asks "как / how to / why / что лучше / which"** → query_experience immediately
2. **User asks about a file, library, pattern or decision** → query_experience
3. **User reports an error or bug** → query_experience(paste error)
4. **New task starts** → query_experience(task description) — check for prior art
5. **Before making any decision** → query_experience(options, tradeoffs)

If you skip the tool call and guess, you will give wrong project-specific answers.

## When to call `report_usage`

Call AFTER you use knowledge returned by `query_experience` — this teaches SEK which results were actually useful, improving future scoring.

1. **You applied the knowledge** → call `report_usage(retrieval_id, knowledge_id)` with the ID from the query_experience response
2. **You did NOT use it** → don't call — unused entries naturally decay via scoring

## Session digest (automatic)

When the session ends, SEK automatically creates a summary lesson from all events.
You don't need to call anything extra — just make sure you `capture_event` for key moments during the session.
