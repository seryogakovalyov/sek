# Experience Runtime

You have access to SEK via MCP tools (`capture_event`, `query_experience`, `report_usage`, `list_knowledge`).
All tools write to a single store context — no `project_id` parameter needed.
SEK remembers what you learn across sessions. Use it deliberately.

Before starting any project-specific task, call `query_experience`.

Before your final response, check whether the task produced reusable project experience.
If you fixed a bug, made an architectural decision, discovered a project convention, chose a library/pattern, or found a
reusable gotcha, call `capture_event` even if the user did not explicitly ask.

When using knowledge returned by `query_experience`, call `report_usage`.

Do not capture routine file edits or every tool call. Capture only reusable lessons with concrete paths, commands, errors,
and rationale.
