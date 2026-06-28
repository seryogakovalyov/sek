package store

import (
	"context"

	"github.com/anomalyco/sek/internal/models"
)

type EventStore interface {
	Append(ctx context.Context, event *models.Event) error
	Query(ctx context.Context, limit int) ([]models.Event, error)
	UnobservedEvents(ctx context.Context, limit int) ([]models.Event, error)
	EventsBySession(ctx context.Context, sessionID string, limit int) ([]models.Event, error)
	EventsByServerSession(ctx context.Context, serverSession string, limit int) ([]models.Event, error)
}

type KnowledgeStore interface {
	Save(ctx context.Context, k *models.Knowledge) error
	Search(ctx context.Context, query string, limit int) ([]models.Knowledge, error)
	SearchSimilar(ctx context.Context, embedding []float32, limit int) ([]models.Knowledge, error)
	FindSimilar(ctx context.Context, embedding []float32, threshold float64, limit int) ([]models.Knowledge, error)
	UpdateSourceIDs(ctx context.Context, id string, sourceIDs []string) error
	List(ctx context.Context, level models.KnowledgeLevel, limit int) ([]models.Knowledge, error)
}

type ProjectStats struct {
	KnowledgeCount int
	EventCount     int
	DBSizeBytes    int64
}

type GCResult struct {
	KnowledgeDeleted int64
	EventsDeleted    int64
	OrphansDeleted   int64
}

type Store interface {
	EventStore
	KnowledgeStore
	DeleteKnowledge(ctx context.Context, id string) error
	Clear(ctx context.Context) error
	Stats(ctx context.Context) (*ProjectStats, error)
	GC(ctx context.Context, before string) (*GCResult, error)
	LogRetrieval(ctx context.Context, log *models.RetrievalLog) error
	MarkRetrievalUsed(ctx context.Context, id string, knowledgeID string) error
	IncrementUsageCount(ctx context.Context, id string) error
	Close() error
}
