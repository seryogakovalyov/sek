# SEK — Project Experience Runtime

SEK даёт AI-агентам постоянную память проекта. Агент сохраняет инженерный опыт между сессиями, дистиллирует его в reusable observations/lessons/patterns и использует при следующих задачах через MCP.

SEK не заменяет coding agent, LLM provider или внешнюю RAG-систему. Это локальный experience runtime: небольшой Go-бинарник, SQLite-файл рядом с проектом и MCP-инструменты для capture/retrieval.

> Полное видение проекта: [VISION.md](VISION.md).

## Возможности

- MCP server через stdio по умолчанию и Streamable HTTP через `--http`.
- Захват событий агента через `capture_event`.
- Дистилляция событий в observations через LLM.
- Promotion: observations → lessons → patterns.
- Vector search по embeddings с fallback на keyword search.
- Source trace: вывод `source_ids`, причины retrieval и scoring metadata.
- Secret redaction перед сохранением events/knowledge.
- Retrieval telemetry и feedback loop через `report_usage`.
- `sekctl` для просмотра, поиска, GC и обслуживания локальной базы.

## Статус

Проект в активной разработке. Основной сценарий уже рабочий:

1. агент вызывает `query_experience` перед проектной задачей;
2. агент вызывает `capture_event` после важного решения, ошибки или фикса;
3. SEK сохраняет raw event, дистиллирует observation и индексирует embedding;
4. при завершении stdio-сессии SEK создаёт session digest;
5. следующие сессии получают релевантный опыт из `.sek/store.db`.

## Установка

Требования:

- Go 1.22+;
- LLM endpoint для chat completions;
- embeddings endpoint для vector search, если нужен semantic retrieval.

Сборка:

```bash
go build -o sekd ./cmd/sekd
go build -o sekctl ./cmd/sekctl
```

Проверка:

```bash
go test ./...
./sekctl status
```

В sandbox или read-only окружениях Go cache может требовать явный путь:

```bash
GOCACHE=/tmp/sek-go-cache go test ./...
```

## Быстрый старт

Запустить MCP server для текущего проекта:

```bash
./sekd --llm-key "$OPENAI_API_KEY"
```

Запустить с явным project path:

```bash
./sekd --project /path/to/project --llm-key "$OPENAI_API_KEY"
```

Запустить с OpenAI-compatible локальным endpoint:

```bash
./sekd \
  --llm-key none \
  --llm-provider openai \
  --llm-base-url http://localhost:8000/v1 \
  --llm-model Qwen3.6-35B-A3B-Unsloth
```

Для llama.cpp embeddings нужны флаги на стороне llama.cpp:

```bash
--embeddings --pooling mean
```

## Конфигурация

`sekd` читает `.sek/config.json`, если файл существует. CLI-флаги переопределяют значения из конфига.

| Флаг | По умолчанию | Описание |
|---|---|---|
| `--project` | `cwd` | Директория проекта; `_global` включает global store |
| `--data-dir` | `~/.sek` | Директория global store |
| `--config` | `.sek/config.json` | Путь к config file |
| `--http` | empty | Streamable HTTP address, например `:9090`; без флага используется stdio |
| `--stdio` | `false` | Принудительно использовать stdio, даже если config содержит `mcp.http_addr` |
| `--llm-provider` | `openai` | `openai` или `anthropic` |
| `--llm-model` | `gpt-4o` | Model name для chat completions и embeddings |
| `--llm-key` | `SEK_LLM_KEY` | API key; для локальных серверов можно использовать `none` |
| `--llm-base-url` | empty | Custom API base URL |

## Режимы хранения

SEK использует один SQLite-файл как один context. Внутреннего `project_id` нет.

### Per-project store

Режим по умолчанию:

```bash
./sekd --project /path/to/project
```

Данные хранятся в:

```text
/path/to/project/.sek/store.db
```

Это основной режим для coding agents: память физически лежит рядом с проектом, легко переносится, удаляется и бэкапится.

### Global store

```bash
./sekd --project _global
```

Данные хранятся в:

```text
~/.sek/store.db
```

Global store — одна общая память машины без разделения на проекты. Он подходит для локальных LLM-настроек, системных gotchas, общих предпочтений и reusable-паттернов.

## Подключение к агентам

SEK по умолчанию использует текущую рабочую директорию MCP-процесса как проект. Это удобно, если агент запускает MCP server из корня репозитория. Если агент не гарантирует `cwd` или запускает subprocess из системной директории, передавайте явный `--project /path/to/project`.

Не используйте `--project .`, если MCP client не задаёт `cwd` в корень проекта. `.` будет интерпретирован относительно процесса агента и может указывать не туда. Для защиты `sekd` отказывается использовать `/` как project directory и просит передать `--project` или настроить `cwd`.

Рекомендации:

- project-local MCP config или client `cwd` в корень проекта: можно использовать `--project .`;
- user-level/global MCP config без гарантированного `cwd`: используйте absolute `--project /path/to/project`;
- общая память машины: используйте `--project _global`.

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

## MCP tools

| Tool | Назначение |
|---|---|
| `capture_event` | Сохранить важное событие и запустить дистилляцию в observation |
| `query_experience` | Найти релевантный опыт по текущей задаче |
| `report_usage` | Сообщить, что найденный knowledge entry реально помог |
| `list_knowledge` | Показать сохранённые knowledge entries |

### `capture_event`

Параметры:

| Параметр | Обязательный | Описание |
|---|---|---|
| `event_type` | да | `request`, `response`, `tool_usage`, `failure`, `decision`, `implementation_choice`, `successful_fix` |
| `source` | да | Источник события: `codex`, `opencode`, `cursor`, `claude-code` |
| `content` | да | Подробное описание события: команды, файлы, ошибки, rationale |
| `session_id` | нет | Пользовательский session id; если пустой, используется server session |

Писать стоит только reusable опыт: ошибки, фиксы, решения, важные команды, discovered conventions. Не нужно писать каждый промежуточный tool call.

### `query_experience`

Параметры:

| Параметр | Обязательный | Описание |
|---|---|---|
| `task` | да | Вопрос или описание задачи |
| `max_tokens` | нет | Максимальный размер ответа |
| `max_entries` | нет | Максимальное число записей |

Ответ содержит formatted knowledge entries и `retrieval_id` для `report_usage`.

### `report_usage`

Параметры:

| Параметр | Обязательный | Описание |
|---|---|---|
| `retrieval_id` | да | ID из ответа `query_experience` |
| `knowledge_id` | да | ID knowledge entry, который был использован |

`report_usage` обновляет telemetry и увеличивает `usage_count`, который участвует в scoring.

### `list_knowledge`

Параметры:

| Параметр | Обязательный | Описание |
|---|---|---|
| `level` | нет | `observation`, `lesson`, `pattern` |
| `limit` | нет | Максимальное число записей |

## CLI

`sekctl` управляет store напрямую. Базовые команды не требуют LLM; `query` требует LLM endpoint.

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

sekctl query "как здесь настроен CI?" --llm-key "$KEY"
sekctl list --project _global
```

`sekctl list` и `sekctl log` берут последние N записей, но печатают их в хронологическом порядке: старые сверху, новые снизу.

| Команда | Основные флаги | Описание |
|---|---|---|
| `init` | `--project` | Создать `.sek/` и store |
| `status` | `--project` | Показать счётчики и размер БД |
| `list` | `--project`, `--level`, `--limit` | Показать knowledge entries |
| `log` | `--project`, `--limit` | Показать raw events |
| `query` | `--project`, `--llm-*`, `--max-tokens`, `--max-entries` | Найти опыт через reuse engine |
| `rm <id>` | `--project` | Удалить knowledge entry |
| `gc` | `--project`, `--older-than`, `--before`, `--dry-run` | Удалить старые entries, retrieval logs и orphan-derived knowledge |
| `prune` | `--project`, `--force` | Удалить все events и knowledge из store |

## Архитектура

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

На shutdown stdio-сессии SEK собирает события текущего `server_session` и сохраняет session digest уровня `lesson`, если событий достаточно.

### Retrieval

`query_experience` сначала пытается получить embedding задачи и выполнить vector search. Если embeddings недоступны, используется keyword search. Результаты ранжируются с учётом:

- cosine similarity;
- recency boost;
- importance по `event_type`;
- usage boost из `report_usage`.

## Стек

- Go — два небольших бинарника: `sekd` и `sekctl`.
- `modernc.org/sqlite` — pure Go SQLite без CGo.
- `mark3labs/mcp-go` — MCP protocol и transports.
- OpenAI-compatible API или Anthropic API — chat completions.
- OpenAI-compatible embeddings endpoint — semantic retrieval.

## Разработка

```bash
go test ./...
go vet ./...
go build -o sekd ./cmd/sekd
go build -o sekctl ./cmd/sekctl
```

Кросс-компиляция:

```bash
GOOS=linux GOARCH=arm64 go build -o sekd-linux-arm64 ./cmd/sekd
GOOS=darwin GOARCH=amd64 go build -o sekd-darwin-amd64 ./cmd/sekd
```

Локальные артефакты `sekd`, `sekctl` и `.sek/` не коммитятся.

## Roadmap

### Phase 1 — Core MVP ✅

- [x] Event store на SQLite
- [x] LLM abstraction: OpenAI-compatible и Anthropic
- [x] MCP server через stdio
- [x] Дистилляция event → observation
- [x] Embeddings через `/v1/embeddings`
- [x] Vector search с cosine similarity
- [x] Keyword fallback

### Phase 2 — Качество дистилляции ✅

- [x] Уровни знаний: observation → lesson → pattern
- [x] Semantic deduplication: cosine > 0.95 + merge `source_ids`
- [x] Scoring: similarity + recency + importance
- [x] Автоматическое обнаружение patterns через clustering + LLM composition

### Phase 3 — Интеграция и управление ✅

- [x] `sekctl`: `init`, `status`, `list`, `log`, `query`, `rm`, `gc`, `prune`
- [x] Session digest на shutdown
- [x] `.sek/config.json` + CLI overrides
- [x] Global store: `--project _global`
- [x] Streamable HTTP transport: `--http :9090`
- [x] GC по TTL: `sekctl gc --older-than 720h`

### Phase 4 — Качество памяти и feedback loop

- [x] Secret redaction перед сохранением events/knowledge
- [x] Golden evals для distillation prompt
- [x] Source trace для knowledge entries
- [x] `sekctl gc --before <timestamp>` + orphan cleanup
- [x] Retrieval telemetry через `retrieval_log`
- [x] Feedback loop через `report_usage` и `usage_count`
- [ ] Knowledge lifecycle: `supersedes`, `conflicts_with`, `deprecated_at`
- [ ] Автоматическая очистка устаревших observations
- [ ] `sekctl --help` с code 0
- [ ] MCP resources для read-only knowledge
- [ ] Experience diff между сессиями
- [ ] Экспорт/импорт знаний между store

### Phase 5 — Экосистема

- [ ] Go SDK
- [ ] Plugin API для custom distillation stages
- [ ] GitHub Action
- [ ] VS Code extension
- [ ] Retrieval benchmarks: recall@k
- [ ] `sek doctor`: диагностика LLM/MCP setup
- [ ] Project onboarding helper для AGENTS.md и MCP snippet
