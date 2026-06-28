# SEK — Project Experience Runtime

SEK даёт AI-агентам постоянную project memory. Агент накапливает инженерный опыт между сессиями и использует его, когда нужно, вместо того чтобы переоткрывать заново.

- Не очередной coding agent.
- Не LLM provider.
- Не RAG-система.
- **Experience runtime** — то, чего не хватает всем остальным.

> Полное видение проекта — [VISION.md](VISION.md).

## Как это работает

### Жизненный цикл данных

```
Интеракция агента
       ↓
  Capture: сырое событие + server_session (авто-тег)
       ↓
  Distill: LLM → observation + embed → SQLite
       ↓                                   
  Promote: каждые 3 obs → lesson → pattern (через LLM)
       ↓
  ...сессия продолжается или завершается...
       ↓
  Session Digest (на shutdown): все события сессии → LLM → lesson
  SQLite: .sek/store.db — events + knowledge + embeddings
```

### Как работает поиск

1. Агент пишет задачу в `query_experience`
2. SEK превращает задачу в вектор (через тот же LLM endpoint, `/v1/embeddings`)
3. Ищет в SQLite все observations с векторами
4. Считает cosine similarity (косинусная близость) между вектором задачи и каждым observation-ом
5. Сортирует по убыванию похожести, возвращает top-K
6. Если векторный поиск недоступен (нет `/v1/embeddings`) — падает на keyword search (Oder By created_at)

Если словесного совпадения нет, но смысл близкий — поиск всё равно найдёт. Например, запрос "как подключить локальную модель" найдёт observation про `--pooling mean`, даже если эти слова не упоминаются.

### Работает ли между сессиями

Да. Все данные хранятся в `.sek/store.db` в той же директории проекта. SQLite — это файл. Он не очищается между сессиями. Пока проект один и тот же — опыт копится вечно.

Каждая новая сессия агента видит всё, что накоплено предыдущими сессиями. MCP сервер должен быть настроен на один и тот же `--project` (путь к директории проекта). `project_id` больше не нужен — все записи пишутся в единый контекст проекта.

### Когда запускается MCP сервер

SEK работает как **MCP subprocess**. Он не висит в фоне, не слушает порт, не жрёт ресурсы.

Как это происходит:
1. Агент (Claude Code, Cursor, opencode) стартует
2. Агент читает свой конфиг (например, `~/.claude.json`), находит секцию `mcpServers.sek`
3. Агент запускает `sekd` как подпроцесс — **однократно**, при своём старте
4. `sekd` висит в памяти, общаясь с агентом через stdin/stdout по JSON-RPC
5. Когда агент завершается (Ctrl+C, нормальный exit, краш) — `sekd` получает EOF на stdin
6. `sekd` запускает **session digest**: собирает все события этой сессии (по `server_session`, которым автоматически тегируется каждый capture_event), делает один LLM-вызов, сохраняет компактный **lesson** сессии
7. `sekd` завершается

**Session digest**: если в сессии было ≥3 событий, при завершении `sekd` делает один LLM-вызов, упаковывая их в одну запись уровня `lesson`. Lesson имеет `importance=0.8` и `source_ids` на все события сессии, поэтому при поиске в следующих сессиях он будет показываться выше отдельных observations. Таймаут — 10 секунд.

**Потребление ресурсов:**
- RAM: ~15-25MB (включая Go runtime + SQLite)
- CPU: 0% в простое, ~1-5 секунд на дистилляцию события
- Диск: размер `.sek/store.db` (~1KB на observation + embedding)
- Сеть: только к LLM endpoint (никаких внешних сервисов)

MCP работает через **stdio** (по умолчанию) или **Streamable HTTP** (с флагом `--http :9090`). stdio не занимает портов и не требует сети; HTTP режим нужен для удалённого доступа или интеграции с веб-интерфейсами. Если HTTP-сессия завершается по таймауту неактивности, digest не запускается — для HTTP это предусмотрено через ручной вызов или ожидание следующей сессии.

## Быстрый старт

```bash
# собрать оба бинарника
go build -o sekd ./cmd/sekd/
go build -o sekctl ./cmd/sekctl/

# запустить MCP-сервер (stdio) с OpenAI
./sekd --llm-key $OPENAI_API_KEY --project /path/to/project

# с локальным llama.cpp
./sekd --llm-key none --llm-provider openai --llm-base-url http://localhost:8000/v1
```

### Флаги

| Флаг | По умолчанию | Описание |
|---|---|---|---|
| `--llm-key` | `SEK_LLM_KEY` env | API key для LLM (дистилляция + embeddings) |
| `--llm-provider` | `openai` | `openai` (OpenAI-compatible, llama.cpp, vLLM, ...) или `anthropic` |
| `--llm-model` | `gpt-4o` | Имя модели для chat completions |
| `--llm-base-url` | — | Кастомный endpoint (обязателен для локальных серверов) |
| `--project` | `cwd` | Директория проекта (там создаётся `.sek/`); `_global` для глобального стора |
| `--data-dir` | `~/.sek` | Директория данных (используется при `--project _global`) |
| `--http` | — | Streamable HTTP адрес (напр. `:9090`); без флага — stdio |
| `--config` | `.sek/config.json` | Путь к конфиг-файлу; если есть — флаги переопределяют его поля |

### Подключение в агентах

**Claude Code** (`~/.claude.json`):
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

**Cursor** → Settings → MCP → Add:
- Name: `SEK`
- Type: `command`
- Command: `/path/to/sekd --llm-key $KEY --project /path/to/project`

**Opencode** (`~/.config/opencode/opencode.jsonc`):
```jsonc
"mcp": {
  "sekd": {
    "type": "local",
    "command": ["/path/to/sekd", "--llm-key", "$KEY", "--project", "/path/to/project"],
    "enabled": true
  }
}
```

**Важно:** если используешь локальный llama.cpp без ключа:
```bash
# Замени $KEY на "none", а --llm-model на имя твоей модели
./sekd --llm-key none --llm-provider openai --llm-base-url http://localhost:8000/v1 --llm-model Qwen3.6-35B-A3B-Unsloth
```

А для поддержки векторного поиска в llama.cpp нужен флаг `--embeddings --pooling mean`.

## MCP Инструменты

| Инструмент | Назначение | Возвращает |
|---|---|---|
| `capture_event` | Записать событие + дистиллировать в observation | ID события + ошибка дистилляции (если была) |
| `query_experience` | Найти релевантный опыт по задаче (векторный поиск) | Knowledge (obs/lesson/pattern) с score похожести; `retrieval_id` для последующего `report_usage` |
| `report_usage` | Сообщить, что выданный knowledge был реально использован | confirmation |
| `list_knowledge` | Посмотреть всё накопленное знание проекта | Flat список, сортировка по дате |

### Параметры инструментов

**capture_event:**

| Параметр | Обязательный | Описание |
|---|---|---|
| `event_type` | да | `request`, `response`, `tool_usage`, `failure`, `decision`, `implementation_choice`, `successful_fix` |
| `source` | да | Кто создал событие (напр. `opencode`, `claude-code`, `cursor`) |
| `content` | да | Текст события (чем детальнее, тем лучше observation) |
| `session_id` | нет | ID сессии для группировки событий (если не указан, SEK автоматически тегирует своим `server_session` для digest на shutdown) |

**query_experience:**

| Параметр | Обязательный | Описание |
|---|---|---|
| `task` | да | Описание задачи или вопрос |
| `max_tokens` | нет | Максимум токенов в ответе (по умолч. 2000) |
| `max_entries` | нет | Максимум записей (по умолч. 10) |

**list_knowledge:**

| Параметр | Обязательный | Описание |
|---|---|---|
| `level` | нет | Фильтр по уровню: `observation`, `lesson`, `pattern` |
| `limit` | нет | Сколько записей вернуть (по умолч. 20) |

**report_usage:**

| Параметр | Обязательный | Описание |
|---|---|---|
| `retrieval_id` | да | ID из `query_experience` (в поле `retrieval_id` в ответе) |
| `knowledge_id` | да | ID знание, которое агент реально использовал |

## Архитектура

```
                 ┌──────────────────────────┐
                │     MCP Client            │
                │  (Claude Code, Cursor,    │
                │   opencode, любой HTTP)   │
                └─────┬──────────┬─────────┘
                stdio /          \ Streamable HTTP
                     v            v
              ┌──────────┐  ┌──────────┐
              │  stdio   │  │   HTTP   │  ← --http :9090
              │ transport│  │transport │
              └─────┬────┘  └─────┬────┘
                    │             │
              ┌─────▼─────────────▼──────┐
              │     SEK MCP Server        │
              │                           │
              │  ┌───────────────────┐    │
              │  │   Capture Svc     │────+── auto server_session tag
              │  └───────┬───────────┘    │
              │          │               │
              │  ┌───────▼───────────┐   │
              │  │   Distill Pipe    │   │  ← LLM (chat completions)
              │  │  • LLM → obs      │   │
              │  │  • dedup (>95%)   │   │
              │  │  • promote        │   │
              │  │    (obs→lesson,   │   │
              │  │     lesson→pat)   │   │
              │  └───────┬───────────┘   │
              │          │               │
              │  ┌───────▼───────────┐   │
              │  │   Embedder        │   │  ← LLM (/v1/embeddings)
              │  │  (vector)         │   │
              │  └───────┬───────────┘   │
              │          │               │
              │  ┌───────▼───────────┐   │
              │  │   Reuse Engine    │   │  ← cosine similarity
              │  │  • vector search  │   │     + recency
              │  │  • score adjust   │   │     + importance
              │  │  • trim by ctx    │   │
              │  └───────┬───────────┘   │
              │          │               │
              │  ┌───────▼───────────┐   │
              │  │   SQLite Store    │   │
              │  │ events +          │   │
              │  │ knowledge +       │   │
              │  │ embeddings        │   │
              │  └───────┬───────────┘   │
              │          │               │
              │  ┌───────▼───────────┐   │
              │  │ Session Digest    │   │  ← shutdown: LLM → lesson
              │  │ (после закрытия   │   │     (все события сессии)
              │  │  stdin или HTTP)  │   │
              │  └───────────────────┘   │
              └──────────────────────────┘
```

### Стек

- **Go** — один бинарник (18MB), быстрый старт (<100ms), кроссплатформа
- **modernc.org/sqlite** — pure Go SQLite (без CGo, без зависимостей)
- **mark3labs/mcp-go** — MCP protocol (stdio, JSON-RPC)
- **OpenAI-compatible / Anthropic API** — LLM для дистилляции и embeddings

### Почему такой выбор

| Компонент | Альтернативы | Почему нет |
|---|---|---|
| **Go** | Python, Rust, TypeScript | Python — медленный старт, нет single binary. Rust — дольше компиляция, меньше MCP-экосистема |
| **SQLite** | PostgreSQL, Redis, MongoDB | Zero infra, один файл на проект, не нужен Docker |
| **Вектора в SQLite** | Pinecone, Weaviate, Qdrant | Данные не уходят с машины, не нужен интернет |
| **MCP stdio** | HTTP, WebSocket | Не нужен порт, не нужен сетевой доступ, запускается вместе с агентом |

## CLI: sekctl

`sekctl` — утилита для управления базой знаний напрямую, без MCP. Не требует LLM для базовых операций (кроме `query`).

```bash
# инициализировать проект
sekctl init

# посмотреть статистику
sekctl status

# список знаний
sekctl list
sekctl list --level lesson
sekctl list --limit 50

# список событий
sekctl log --limit 10

# удалить запись
sekctl rm obs-abc123

# GC: удалить записи старше 30 дней
sekctl gc --older-than 720h
sekctl gc --older-than 30d --dry-run  # посмотреть без удаления
sekctl gc --all                       # все проекты в global store

# очистить всё (с подтверждением)
sekctl prune
sekctl prune --force

# поиск опыта (требует LLM)
sekctl query "как здесь настроен CI?" --llm-key $KEY

# глобальный опыт (общий для всех проектов)
sekctl list --project _global
```

Все флаги:
| Команда | Флаги | Описание |
|---------|-------|----------|
| `init` | `--project` | Создать `.sek/` и БД |
| `list` | `--project`, `--level`, `--limit` | Список знаний в таблице |
| `log` | `--project`, `--limit` | Список событий |
| `rm <id>` | `--project` | Удалить знание по ID |
| `gc` | `--project`, `--older-than`, `--dry-run`, `--all` | GC по TTL |
| `status` | `--project` | Статистика (счётчики, размер БД) |
| `prune` | `--project`, `--force` | Удалить все данные проекта |
| `query <task>` | `--project`, `--llm-*`, `--max-tokens`, `--max-entries` | Поиск опыта (как MCP tool) |

## Roadmap

### Phase 1 — Core MVP ✅
- [x] Event Store (SQLite, append-only)
- [x] LLM abstraction (OpenAI + Anthropic)
- [x] MCP сервер (stdio, 3 инструмента)
- [x] Дистилляция: event → observation через LLM
- [x] Embeddings через /v1/embeddings (local или OpenAI)
- [x] Векторный поиск с cosine similarity
- [x] Fallback: keyword search если embedder недоступен

### Phase 2 — Качество дистилляции ✅
- [x] Уровни знаний: Observation → Lesson → Pattern (композиция связанных observations → LLM → lesson, lessons → LLM → pattern)
- [x] Дедупликация наблюдений (semantic dedup — cosine >0.95, merge source_ids)
- [x] Scoring: cosine × (1 + recencyBoost + importanceBoost), где recency = экспоненциальный decay за 30 дней, importance = от event_type
- [x] Автоматическое детектирование паттернов (кросс-сессионная кластеризация по embedding + LLM композиция)

### Phase 3 — Интеграция и управление ✅
- [x] `sekctl` CLI: `init`, `list`, `log`, `rm`, `status`, `prune`, `query`, `gc`
- [x] `status` — статистика (счётчики, размер БД)
- [x] **Session digest** — автоматический lesson на shutdown для каждой сессии
- [x] Config file (`.sek/config.json`) загружается и мержится с CLI-флагами
- [x] Глобальный опыт: `--project _global` + `--data-dir ~/.sek`
- [x] Streamable HTTP transport: `--http :9090` через mcp-go `NewStreamableHTTPServer`
- [x] `sek gc --older-than 720h` — очистка старых записей по TTL

### Phase 4 — Pro-фичи
- [x] Secret redaction перед сохранением events/knowledge: ключи, токены, пароли, приватные URL
- [x] Golden evals для дистилляции: fixture events → expected observations, чтобы prompt не терял пути, команды, config keys и tool names
- [x] Source trace: показывать для каждого lesson/pattern исходные events/observations и почему запись попала в ответ
- [x] `sekctl gc --before <timestamp>`: абсолютный cutoff + удаление orphan-derived lessons/patterns, которые ссылаются на удалённые source_ids
- [x] Retrieval telemetry: сохранять какие knowledge entries были выданы на запрос и какие из них агент реально использовал (таблица `retrieval_log`, MCP-инструмент `report_usage`)
- [x] Feedback loop: `report_usage` инкрементит `usage_count` на knowledge → `usageBoost` в scoring (+0.05 per use, max +0.3)
- [ ] Knowledge lifecycle: `supersedes`, `conflicts_with`, `deprecated_at` для устаревших или заменённых решений
- [ ] Автоматическая очистка устаревших наблюдений (TTL + relevance decay)
- [ ] `sekctl --help`: top-level help должен печатать usage и выходить с code 0
- [ ] MCP resources: knowledge как read-only ресурсы, а не только инструменты
- [ ] Experience diff: показывать что изменилось между сессиями
- [ ] Экспорт/импорт знаний между проектами

### Phase 5 — Экосистема
- [ ] Go SDK для программной интеграции
- [ ] Plugin-система для кастомных стадий дистилляции
- [ ] GitHub Action для CI/CD
- [ ] VSCode extension (просмотр опыта проекта прямо в IDE)
- [ ] Бенчмарки качества retrieval (recall@k)
- [ ] `sek doctor`: диагностика локальной LLM/MCP setup — chat endpoint, `/v1/embeddings`, model name, `--pooling mean`, config path, store path
- [ ] Project onboarding helper: сгенерировать AGENTS.md/MCP snippet для нового проекта и проверить, что агент вызывает `query_experience`

## Разработка

```bash
go build ./cmd/sekd/     # собрать MCP-сервер
go build ./cmd/sekctl/   # собрать CLI
go test ./...            # запустить тесты
go vet ./...             # статический анализ
```

Проект на чистом Go, без CGo. Компилируется под любую платформу:
```bash
GOOS=linux GOARCH=arm64 go build -o sekd-linux-arm64 ./cmd/sekd/
GOOS=darwin GOARCH=amd64 go build -o sekd-darwin-amd64 ./cmd/sekd/
```
