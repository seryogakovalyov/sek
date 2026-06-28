package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/anomalyco/sek/internal/models"
	"github.com/anomalyco/sek/internal/redact"
	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db *sql.DB
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
		project_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		server_session TEXT DEFAULT '',
		timestamp TEXT NOT NULL,
		type TEXT NOT NULL,
		source TEXT NOT NULL,
		content TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_events_project ON events(project_id, timestamp);

	CREATE TABLE IF NOT EXISTS knowledge (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		level TEXT NOT NULL,
		created_at TEXT NOT NULL,
		content TEXT NOT NULL,
		source_ids TEXT DEFAULT '[]',
		embedding BLOB,
		event_type TEXT DEFAULT '',
		importance REAL DEFAULT 0.5
	);
	CREATE INDEX IF NOT EXISTS idx_knowledge_project ON knowledge(project_id, level);
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
	_, err = db.Exec(`ALTER TABLE events ADD COLUMN server_session TEXT DEFAULT ''`)
	if err != nil {
		// column may already exist
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_ssession ON events(server_session)`)
	if err != nil {
		// ignore
	}
	return nil
}

func (s *sqliteStore) Append(ctx context.Context, event *models.Event) error {
	event.Content = redact.Secrets(event.Content)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (id, project_id, session_id, server_session, timestamp, type, source, content) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.ProjectID, event.SessionID, event.ServerSession, event.Timestamp.Format(time.RFC3339Nano), string(event.Type), event.Source, event.Content,
	)
	return err
}

func (s *sqliteStore) Query(ctx context.Context, projectID string, limit int) ([]models.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, session_id, server_session, timestamp, type, source, content FROM events WHERE project_id = ? ORDER BY timestamp DESC LIMIT ?`,
		projectID, limit,
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
		`INSERT INTO knowledge (id, project_id, level, created_at, content, source_ids, embedding, event_type, importance) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.ProjectID, string(k.Level), k.CreatedAt.Format(time.RFC3339Nano), k.Content, string(sourceIDs), embBytes, string(k.EventType), float64(k.Importance),
	)
	return err
}

func (s *sqliteStore) Search(ctx context.Context, projectID string, query string, limit int) ([]models.Knowledge, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, level, created_at, content, source_ids, embedding, event_type, importance FROM knowledge WHERE project_id = ? AND content LIKE '%' || ? || '%' ORDER BY created_at DESC LIMIT ?`,
		projectID, query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKnowledge(rows)
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
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Level, &ts, &k.Content, &srcIDs, &embBytes, &evType, &importance); err != nil {
			return nil, err
		}
		k.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		json.Unmarshal([]byte(srcIDs), &k.SourceIDs)
		k.EventType = models.EventType(evType)
		k.Importance = models.Importance(importance)
		if len(embBytes) > 0 {
			k.Embedding = decodeEmbedding(embBytes)
		}
		knowledge = append(knowledge, k)
	}
	return knowledge, rows.Err()
}

func (s *sqliteStore) FindSimilar(ctx context.Context, projectID string, embedding []float32, threshold float64, limit int) ([]models.Knowledge, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, level, created_at, content, source_ids, embedding, event_type, importance FROM knowledge WHERE project_id = ? AND level = 'observation' AND embedding IS NOT NULL`,
		projectID,
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

func (s *sqliteStore) SearchSimilar(ctx context.Context, projectID string, embedding []float32, limit int) ([]models.Knowledge, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, level, created_at, content, source_ids, embedding, event_type, importance FROM knowledge WHERE project_id = ?`,
		projectID,
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

func (s *sqliteStore) List(ctx context.Context, projectID string, level models.KnowledgeLevel, limit int) ([]models.Knowledge, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if level == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, project_id, level, created_at, content, source_ids, embedding, event_type, importance FROM knowledge WHERE project_id = ? ORDER BY created_at DESC LIMIT ?`,
			projectID, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, project_id, level, created_at, content, source_ids, embedding, event_type, importance FROM knowledge WHERE project_id = ? AND level = ? ORDER BY created_at DESC LIMIT ?`,
			projectID, string(level), limit,
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

func (s *sqliteStore) ClearProject(ctx context.Context, projectID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM knowledge WHERE project_id = ?`, projectID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM events WHERE project_id = ?`, projectID)
	return err
}

func (s *sqliteStore) Stats(ctx context.Context, projectID string) (*ProjectStats, error) {
	var stats ProjectStats
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM knowledge WHERE project_id = ?`, projectID).Scan(&stats.KnowledgeCount)
	if err != nil {
		return nil, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE project_id = ?`, projectID).Scan(&stats.EventCount)
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
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.SessionID, &e.ServerSession, &ts, &e.Type, &e.Source, &e.Content); err != nil {
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
		`SELECT id, project_id, session_id, server_session, timestamp, type, source, content FROM events WHERE session_id = ? ORDER BY timestamp ASC LIMIT ?`,
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
		`SELECT id, project_id, session_id, server_session, timestamp, type, source, content FROM events WHERE server_session = ? ORDER BY timestamp ASC LIMIT ?`,
		serverSession, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *sqliteStore) UnobservedEvents(ctx context.Context, projectID string, limit int) ([]models.Event, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if projectID == "" || projectID == "*" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT e.id, e.project_id, e.session_id, e.server_session, e.timestamp, e.type, e.source, e.content
			FROM events e
			WHERE NOT EXISTS (
				SELECT 1 FROM knowledge k
				WHERE k.source_ids LIKE '%' || e.id || '%'
			)
			ORDER BY e.timestamp DESC
			LIMIT ?`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT e.id, e.project_id, e.session_id, e.server_session, e.timestamp, e.type, e.source, e.content
			FROM events e
			WHERE e.project_id = ?
			  AND NOT EXISTS (
				SELECT 1 FROM knowledge k
				WHERE k.project_id = e.project_id
				  AND k.source_ids LIKE '%' || e.id || '%'
			  )
			ORDER BY e.timestamp DESC
			LIMIT ?`, projectID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *sqliteStore) GC(ctx context.Context, projectID string, before string) (*GCResult, error) {
	var result GCResult

	if projectID == "" || projectID == "*" {
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
	} else {
		del, err := s.db.ExecContext(ctx, `DELETE FROM knowledge WHERE project_id = ? AND created_at < ?`, projectID, before)
		if err != nil {
			return nil, err
		}
		result.KnowledgeDeleted, _ = del.RowsAffected()

		del, err = s.db.ExecContext(ctx, `DELETE FROM events WHERE project_id = ? AND timestamp < ?`, projectID, before)
		if err != nil {
			return nil, err
		}
		result.EventsDeleted, _ = del.RowsAffected()
	}

	return &result, nil
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
