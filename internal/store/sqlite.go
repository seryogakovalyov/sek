package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/seryogakovalyov/sek/internal/models"
	"github.com/seryogakovalyov/sek/internal/redact"
	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db *sql.DB
}

type retrievalResultEntry struct {
	ID        string                `json:"id"`
	Score     float64               `json:"score"`
	Breakdown models.ScoreBreakdown `json:"score_breakdown,omitempty"`
}

func NewSQLite(path string) (Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &sqliteStore{db: db}, nil
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		server_session TEXT DEFAULT '',
		timestamp TEXT NOT NULL,
		type TEXT NOT NULL,
		source TEXT NOT NULL,
		content TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS knowledge (
		id TEXT PRIMARY KEY,
		level TEXT NOT NULL,
		created_at TEXT NOT NULL,
		content TEXT NOT NULL,
		source_ids TEXT DEFAULT '[]',
		embedding BLOB,
		event_type TEXT DEFAULT '',
		importance REAL DEFAULT 0.5
	);
	`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE knowledge ADD COLUMN event_type TEXT DEFAULT ''`)
	if err != nil {
		// column may already exist
	}
	_, err = db.Exec(`ALTER TABLE knowledge ADD COLUMN importance REAL DEFAULT 0.5`)
	if err != nil {
		// column may already exist
	}
	_, err = db.Exec(`ALTER TABLE knowledge ADD COLUMN usage_count INTEGER DEFAULT 0`)
	if err != nil {
		// column may already exist
	}
	_, err = db.Exec(`ALTER TABLE events ADD COLUMN server_session TEXT DEFAULT ''`)
	if err != nil {
		// column may already exist
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp)`)
	if err != nil {
		// ignore
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_ssession ON events(server_session)`)
	if err != nil {
		// ignore
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_knowledge_level ON knowledge(level)`)
	if err != nil {
		// ignore
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS retrieval_log (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		timestamp TEXT NOT NULL,
		task TEXT NOT NULL,
		results TEXT DEFAULT '[]',
		used_ids TEXT DEFAULT '[]'
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_retrieval_timestamp ON retrieval_log(timestamp)`)
	if err != nil {
		// ignore
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS module_route_log (
		id TEXT PRIMARY KEY,
		knowledge_id TEXT NOT NULL,
		timestamp TEXT NOT NULL,
		module TEXT NOT NULL,
		confidence REAL DEFAULT 0,
		reason TEXT DEFAULT ''
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_module_route_timestamp ON module_route_log(timestamp)`)
	if err != nil {
		// ignore
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_module_route_knowledge ON module_route_log(knowledge_id)`)
	if err != nil {
		// ignore
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS session_log (
		id TEXT PRIMARY KEY,
		started_at TEXT NOT NULL,
		ended_at TEXT DEFAULT '',
		project_dir TEXT NOT NULL,
		status TEXT NOT NULL,
		start_snapshot TEXT DEFAULT '{}',
		end_snapshot TEXT DEFAULT '{}'
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_session_log_started ON session_log(started_at)`)
	if err != nil {
		// ignore
	}

	return nil
}

func (s *sqliteStore) StartSession(ctx context.Context, session *models.SessionLog) error {
	startSnapshot, err := json.Marshal(session.StartSnapshot)
	if err != nil {
		return err
	}
	status := session.Status
	if status == "" {
		status = "running"
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO session_log (id, started_at, ended_at, project_dir, status, start_snapshot, end_snapshot) VALUES (?, ?, ?, ?, ?, ?, COALESCE((SELECT end_snapshot FROM session_log WHERE id = ?), '{}'))`,
		session.ID, session.StartedAt.Format(time.RFC3339Nano), "", session.ProjectDir, status, string(startSnapshot), session.ID,
	)
	return err
}

func (s *sqliteStore) FinishSession(ctx context.Context, sessionID string, endedAt string, snapshot *models.GitSnapshot) error {
	endSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE session_log SET ended_at = ?, status = ?, end_snapshot = ? WHERE id = ?`,
		endedAt, "finished", string(endSnapshot), sessionID,
	)
	return err
}

func (s *sqliteStore) RecoverOpenSessions(ctx context.Context, projectDir string, endedAt string, snapshot *models.GitSnapshot) error {
	endSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE session_log SET ended_at = ?, status = ?, end_snapshot = ? WHERE project_dir = ? AND status = ? AND ended_at = ''`,
		endedAt, "interrupted", string(endSnapshot), projectDir, "running",
	)
	return err
}

func (s *sqliteStore) GetSession(ctx context.Context, sessionID string) (*models.SessionLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, started_at, ended_at, project_dir, status, start_snapshot, end_snapshot FROM session_log WHERE id = ? LIMIT 1`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions, err := scanSessions(rows)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, sql.ErrNoRows
	}
	return &sessions[0], nil
}

func (s *sqliteStore) ListSessions(ctx context.Context, limit int) ([]models.SessionLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, started_at, ended_at, project_dir, status, start_snapshot, end_snapshot FROM session_log ORDER BY started_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func scanSessions(rows *sql.Rows) ([]models.SessionLog, error) {
	var sessions []models.SessionLog
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *session)
	}
	return sessions, rows.Err()
}

type sessionScanner interface {
	Scan(dest ...any) error
}

func scanSession(row sessionScanner) (*models.SessionLog, error) {
	var session models.SessionLog
	var startedAt string
	var endedAt string
	var startSnapshot string
	var endSnapshot string
	if err := row.Scan(&session.ID, &startedAt, &endedAt, &session.ProjectDir, &session.Status, &startSnapshot, &endSnapshot); err != nil {
		return nil, err
	}
	session.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt)
	if endedAt != "" {
		session.EndedAt, _ = time.Parse(time.RFC3339Nano, endedAt)
	}
	if startSnapshot != "" && startSnapshot != "{}" {
		var snapshot models.GitSnapshot
		if err := json.Unmarshal([]byte(startSnapshot), &snapshot); err != nil {
			return nil, err
		}
		session.StartSnapshot = &snapshot
	}
	if endSnapshot != "" && endSnapshot != "{}" {
		var snapshot models.GitSnapshot
		if err := json.Unmarshal([]byte(endSnapshot), &snapshot); err != nil {
			return nil, err
		}
		session.EndSnapshot = &snapshot
	}
	return &session, nil
}

func (s *sqliteStore) Append(ctx context.Context, event *models.Event) error {
	event.Content = redact.Secrets(event.Content)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (id, session_id, server_session, timestamp, type, source, content) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.SessionID, event.ServerSession, event.Timestamp.Format(time.RFC3339Nano), string(event.Type), event.Source, event.Content,
	)
	return err
}

func (s *sqliteStore) GetEvent(ctx context.Context, id string) (*models.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, server_session, timestamp, type, source, content FROM events WHERE id = ? LIMIT 1`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, sql.ErrNoRows
	}
	return &events[0], nil
}

func (s *sqliteStore) Query(ctx context.Context, limit int) ([]models.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, server_session, timestamp, type, source, content FROM events ORDER BY timestamp DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *sqliteStore) Save(ctx context.Context, k *models.Knowledge) error {
	k.Content = redact.Secrets(k.Content)
	sourceIDs, _ := json.Marshal(k.SourceIDs)
	var embBytes []byte
	if len(k.Embedding) > 0 {
		embBytes = encodeEmbedding(k.Embedding)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO knowledge (id, level, created_at, content, source_ids, embedding, event_type, importance, usage_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		k.ID, string(k.Level), k.CreatedAt.Format(time.RFC3339Nano), k.Content, string(sourceIDs), embBytes, string(k.EventType), float64(k.Importance), k.UsageCount,
	)
	return err
}

func (s *sqliteStore) GetKnowledge(ctx context.Context, id string) (*models.Knowledge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, level, created_at, content, source_ids, embedding, event_type, importance, COALESCE(usage_count, 0) FROM knowledge WHERE id = ? LIMIT 1`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	knowledge, err := scanKnowledge(rows)
	if err != nil {
		return nil, err
	}
	if len(knowledge) == 0 {
		return nil, sql.ErrNoRows
	}
	return &knowledge[0], nil
}

func (s *sqliteStore) Search(ctx context.Context, query string, limit int) ([]models.Knowledge, error) {
	if limit <= 0 {
		limit = 50
	}
	tokens := searchTokens(query)
	if len(tokens) == 0 {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, level, created_at, content, source_ids, embedding, event_type, importance, COALESCE(usage_count, 0) FROM knowledge`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	all, err := scanKnowledge(rows)
	if err != nil {
		return nil, err
	}

	matches := make([]models.Knowledge, 0, len(all))
	for _, k := range all {
		score := keywordScore(k, tokens)
		if score <= 0 {
			continue
		}
		k.Score = score
		matches = append(matches, k)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].CreatedAt.After(matches[j].CreatedAt)
		}
		return matches[i].Score > matches[j].Score
	})

	if limit > len(matches) {
		limit = len(matches)
	}
	return matches[:limit], nil
}

func searchTokens(query string) []string {
	seen := make(map[string]bool)
	var tokens []string
	for _, token := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == '/')
	}) {
		token = strings.Trim(token, "-_./")
		if len(token) < 2 || seen[token] {
			continue
		}
		seen[token] = true
		tokens = append(tokens, token)
	}
	return tokens
}

func keywordScore(k models.Knowledge, tokens []string) float64 {
	haystack := strings.ToLower(k.ID + " " + k.Content + " " + strings.Join(k.SourceIDs, " "))
	matches := 0
	for _, token := range tokens {
		if strings.Contains(haystack, token) {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}
	fraction := float64(matches) / float64(len(tokens))
	return 0.25 + 0.75*fraction
}

func scanKnowledge(rows *sql.Rows) ([]models.Knowledge, error) {
	var knowledge []models.Knowledge
	for rows.Next() {
		var k models.Knowledge
		var ts string
		var srcIDs string
		var embBytes []byte
		var evType string
		var importance float64
		var usageCount int
		if err := rows.Scan(&k.ID, &k.Level, &ts, &k.Content, &srcIDs, &embBytes, &evType, &importance, &usageCount); err != nil {
			return nil, err
		}
		k.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		json.Unmarshal([]byte(srcIDs), &k.SourceIDs)
		k.EventType = models.EventType(evType)
		k.Importance = models.Importance(importance)
		k.UsageCount = usageCount
		if len(embBytes) > 0 {
			k.Embedding = decodeEmbedding(embBytes)
		}
		knowledge = append(knowledge, k)
	}
	return knowledge, rows.Err()
}

func (s *sqliteStore) FindSimilar(ctx context.Context, embedding []float32, threshold float64, limit int) ([]models.Knowledge, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, level, created_at, content, source_ids, embedding, event_type, importance, COALESCE(usage_count, 0) FROM knowledge WHERE level = 'observation' AND embedding IS NOT NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	all, err := scanKnowledge(rows)
	if err != nil {
		return nil, err
	}

	type scored struct {
		knowledge models.Knowledge
		score     float64
	}

	var scoredList []scored
	for _, k := range all {
		score := cosineSimilarity(embedding, k.Embedding)
		if math.IsNaN(score) {
			score = 0
		}
		if score >= threshold {
			scoredList = append(scoredList, scored{k, score})
		}
	}

	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})

	if limit > len(scoredList) {
		limit = len(scoredList)
	}
	result := make([]models.Knowledge, limit)
	for i := 0; i < limit; i++ {
		result[i] = scoredList[i].knowledge
		result[i].Score = scoredList[i].score
	}
	return result, nil
}

func (s *sqliteStore) SearchSimilar(ctx context.Context, embedding []float32, limit int) ([]models.Knowledge, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, level, created_at, content, source_ids, embedding, event_type, importance, COALESCE(usage_count, 0) FROM knowledge`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	all, err := scanKnowledge(rows)
	if err != nil {
		return nil, err
	}

	type scored struct {
		knowledge models.Knowledge
		score     float64
	}

	var scoredList []scored
	for _, k := range all {
		var score float64
		if len(k.Embedding) > 0 {
			score = cosineSimilarity(embedding, k.Embedding)
			if math.IsNaN(score) {
				score = 0
			}
		} else {
			score = 0.05 // low default for records without embedding
		}
		scoredList = append(scoredList, scored{k, score})
	}

	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})

	if limit > len(scoredList) {
		limit = len(scoredList)
	}

	result := make([]models.Knowledge, limit)
	for i := 0; i < limit; i++ {
		result[i] = scoredList[i].knowledge
		result[i].Score = scoredList[i].score
	}
	return result, nil
}

func (s *sqliteStore) UpdateSourceIDs(ctx context.Context, id string, sourceIDs []string) error {
	data, _ := json.Marshal(sourceIDs)
	_, err := s.db.ExecContext(ctx, `UPDATE knowledge SET source_ids = ? WHERE id = ?`, string(data), id)
	return err
}

func (s *sqliteStore) List(ctx context.Context, level models.KnowledgeLevel, limit int) ([]models.Knowledge, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if level == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, level, created_at, content, source_ids, embedding, event_type, importance, COALESCE(usage_count, 0) FROM knowledge ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, level, created_at, content, source_ids, embedding, event_type, importance, COALESCE(usage_count, 0) FROM knowledge WHERE level = ? ORDER BY created_at DESC LIMIT ?`,
			string(level), limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanKnowledge(rows)
}

func (s *sqliteStore) DeleteKnowledge(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM knowledge WHERE id = ?`, id)
	return err
}

func (s *sqliteStore) Clear(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM knowledge`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM events`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM retrieval_log`)
	return err
}

func (s *sqliteStore) Stats(ctx context.Context) (*StoreStats, error) {
	var stats StoreStats
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM knowledge`).Scan(&stats.KnowledgeCount)
	if err != nil {
		return nil, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&stats.EventCount)
	if err != nil {
		return nil, err
	}
	var pageCount, pageSize int
	s.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pageCount)
	s.db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize)
	stats.DBSizeBytes = int64(pageCount) * int64(pageSize)
	return &stats, nil
}

func scanEvents(rows *sql.Rows) ([]models.Event, error) {
	var events []models.Event
	for rows.Next() {
		var e models.Event
		var ts string
		if err := rows.Scan(&e.ID, &e.SessionID, &e.ServerSession, &ts, &e.Type, &e.Source, &e.Content); err != nil {
			return nil, err
		}
		e.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *sqliteStore) EventsBySession(ctx context.Context, sessionID string, limit int) ([]models.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, server_session, timestamp, type, source, content FROM events WHERE session_id = ? ORDER BY timestamp ASC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *sqliteStore) EventsByServerSession(ctx context.Context, serverSession string, limit int) ([]models.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, server_session, timestamp, type, source, content FROM events WHERE server_session = ? ORDER BY timestamp ASC LIMIT ?`,
		serverSession, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *sqliteStore) UnobservedEvents(ctx context.Context, limit int) ([]models.Event, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.session_id, e.server_session, e.timestamp, e.type, e.source, e.content
		FROM events e
		WHERE NOT EXISTS (
			SELECT 1 FROM knowledge k
			WHERE k.source_ids LIKE '%' || e.id || '%'
		)
		ORDER BY e.timestamp DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *sqliteStore) GC(ctx context.Context, before string) (*GCResult, error) {
	var result GCResult

	del, err := s.db.ExecContext(ctx, `DELETE FROM knowledge WHERE created_at < ?`, before)
	if err != nil {
		return nil, err
	}
	result.KnowledgeDeleted, _ = del.RowsAffected()

	del, err = s.db.ExecContext(ctx, `DELETE FROM events WHERE timestamp < ?`, before)
	if err != nil {
		return nil, err
	}
	result.EventsDeleted, _ = del.RowsAffected()

	del, err = s.db.ExecContext(ctx, `DELETE FROM retrieval_log WHERE timestamp < ?`, before)
	if err != nil {
		return nil, err
	}
	result.RetrievalDeleted, _ = del.RowsAffected()

	orphans, err := s.deleteOrphanDerived(ctx)
	if err != nil {
		return nil, err
	}
	result.OrphansDeleted = orphans

	return &result, nil
}

func (s *sqliteStore) deleteOrphanDerived(ctx context.Context) (int64, error) {
	idRows, err := s.db.QueryContext(ctx, `SELECT id FROM knowledge`)
	if err != nil {
		return 0, fmt.Errorf("query existing ids: %w", err)
	}
	defer idRows.Close()

	existingIDs := make(map[string]struct{})
	for idRows.Next() {
		var id string
		if err := idRows.Scan(&id); err != nil {
			return 0, fmt.Errorf("scan id: %w", err)
		}
		existingIDs[id] = struct{}{}
	}
	if err := idRows.Err(); err != nil {
		return 0, fmt.Errorf("iterate ids: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, source_ids FROM knowledge WHERE level IN ('lesson', 'pattern')`,
	)
	if err != nil {
		return 0, fmt.Errorf("query derived: %w", err)
	}
	defer rows.Close()

	var orphanIDs []string
	for rows.Next() {
		var id, srcIDsStr string
		if err := rows.Scan(&id, &srcIDsStr); err != nil {
			return 0, fmt.Errorf("scan derived: %w", err)
		}

		var sourceIDs []string
		if err := json.Unmarshal([]byte(srcIDsStr), &sourceIDs); err != nil || len(sourceIDs) == 0 {
			continue
		}

		allGone := true
		for _, sid := range sourceIDs {
			if _, exists := existingIDs[sid]; exists {
				allGone = false
				break
			}
		}
		if allGone {
			orphanIDs = append(orphanIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate derived: %w", err)
	}

	if len(orphanIDs) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(orphanIDs))
	args := make([]any, len(orphanIDs))
	for i, id := range orphanIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `DELETE FROM knowledge WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete orphans: %w", err)
	}

	n, _ := result.RowsAffected()
	return n, nil
}

func (s *sqliteStore) LogRetrieval(ctx context.Context, log *models.RetrievalLog) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO retrieval_log (id, session_id, timestamp, task, results, used_ids) VALUES (?, ?, ?, ?, ?, ?)`,
		log.ID, log.SessionID, log.Timestamp.Format(time.RFC3339Nano), log.Task, log.Results, log.UsedIDs,
	)
	return err
}

func (s *sqliteStore) UsageSummary(ctx context.Context) (*models.UsageSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT used_ids FROM retrieval_log`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summary models.UsageSummary
	for rows.Next() {
		var usedIDs string
		if err := rows.Scan(&usedIDs); err != nil {
			return nil, err
		}
		used := countUsedIDs(usedIDs)
		summary.Retrievals++
		if used > 0 {
			summary.UsedRetrievals++
			summary.UsedMarks += used
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(usage_count), 0) FROM knowledge WHERE COALESCE(usage_count, 0) > 0`,
	).Scan(&summary.KnowledgeWithUse, &summary.TotalUsageCount)
	if err != nil {
		return nil, err
	}

	return &summary, nil
}

func (s *sqliteStore) UsageBySession(ctx context.Context, limit int) ([]models.SessionUsage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, timestamp, used_ids FROM retrieval_log ORDER BY timestamp DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bySession := make(map[string]*models.SessionUsage)
	order := make([]string, 0)
	for rows.Next() {
		var sessionID, ts, usedIDs string
		if err := rows.Scan(&sessionID, &ts, &usedIDs); err != nil {
			return nil, err
		}
		item, ok := bySession[sessionID]
		if !ok {
			item = &models.SessionUsage{SessionID: sessionID}
			bySession[sessionID] = item
			order = append(order, sessionID)
		}
		item.Retrievals++
		used := countUsedIDs(usedIDs)
		if used > 0 {
			item.UsedRetrievals++
			item.UsedMarks += used
		}
		if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil && parsed.After(item.LastSeen) {
			item.LastSeen = parsed
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]models.SessionUsage, 0, len(order))
	for _, sessionID := range order {
		result = append(result, *bySession[sessionID])
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (s *sqliteStore) ListRetrievals(ctx context.Context, sessionID string, unusedOnly bool, limit int) ([]models.RetrievalLog, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `SELECT id, session_id, timestamp, task, results, used_ids FROM retrieval_log`
	args := make([]any, 0, 2)
	where := make([]string, 0, 2)
	if sessionID != "" {
		where = append(where, `session_id = ?`)
		args = append(args, sessionID)
	}
	if unusedOnly {
		where = append(where, `(used_ids IS NULL OR used_ids = '[]')`)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY timestamp DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.RetrievalLog
	for rows.Next() {
		var item models.RetrievalLog
		var ts string
		if err := rows.Scan(&item.ID, &item.SessionID, &ts, &item.Task, &item.Results, &item.UsedIDs); err != nil {
			return nil, err
		}
		item.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		logs = append(logs, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *sqliteStore) TopUsedKnowledge(ctx context.Context, limit int) ([]models.Knowledge, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, level, created_at, content, source_ids, embedding, event_type, importance, COALESCE(usage_count, 0)
		FROM knowledge
		WHERE COALESCE(usage_count, 0) > 0
		ORDER BY COALESCE(usage_count, 0) DESC, created_at DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKnowledge(rows)
}

func countUsedIDs(raw string) int {
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return 0
	}
	return len(ids)
}

func (s *sqliteStore) MarkRetrievalUsed(ctx context.Context, id string, knowledgeID string) (bool, error) {
	var results, usedIDs string
	err := s.db.QueryRowContext(ctx, `SELECT results, used_ids FROM retrieval_log WHERE id = ?`, id).Scan(&results, &usedIDs)
	if err != nil {
		return false, fmt.Errorf("retrieval log not found: %w", err)
	}

	var returned []retrievalResultEntry
	if err := json.Unmarshal([]byte(results), &returned); err != nil {
		return false, fmt.Errorf("parse retrieval results: %w", err)
	}
	found := false
	for _, result := range returned {
		if result.ID == knowledgeID {
			found = true
			break
		}
	}
	if !found {
		return false, fmt.Errorf("knowledge %s was not returned by retrieval %s", knowledgeID, id)
	}

	var ids []string
	if err := json.Unmarshal([]byte(usedIDs), &ids); err != nil {
		return false, fmt.Errorf("parse used ids: %w", err)
	}

	for _, existing := range ids {
		if existing == knowledgeID {
			return false, nil
		}
	}
	ids = append(ids, knowledgeID)

	data, _ := json.Marshal(ids)
	_, err = s.db.ExecContext(ctx, `UPDATE retrieval_log SET used_ids = ? WHERE id = ?`, string(data), id)
	return true, err
}

func (s *sqliteStore) IncrementUsageCount(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE knowledge SET usage_count = COALESCE(usage_count, 0) + 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("knowledge not found: %s", id)
	}
	return err
}

func (s *sqliteStore) LogModuleRoute(ctx context.Context, log *models.ModuleRouteLog) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO module_route_log (id, knowledge_id, timestamp, module, confidence, reason) VALUES (?, ?, ?, ?, ?, ?)`,
		log.ID, log.KnowledgeID, log.Timestamp.Format(time.RFC3339Nano), log.Module, log.Confidence, log.Reason,
	)
	return err
}

func (s *sqliteStore) ListModuleRoutes(ctx context.Context, limit int) ([]models.ModuleRouteLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, knowledge_id, timestamp, module, confidence, reason FROM module_route_log ORDER BY timestamp DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.ModuleRouteLog
	for rows.Next() {
		var item models.ModuleRouteLog
		var ts string
		if err := rows.Scan(&item.ID, &item.KnowledgeID, &ts, &item.Module, &item.Confidence, &item.Reason); err != nil {
			return nil, err
		}
		item.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		logs = append(logs, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

func encodeEmbedding(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func decodeEmbedding(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
