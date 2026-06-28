package capture

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/anomalyco/sek/internal/models"
	"github.com/anomalyco/sek/internal/redact"
	"github.com/anomalyco/sek/internal/store"
)

type Service struct {
	store store.EventStore
}

func NewService(s store.EventStore) *Service {
	return &Service{store: s}
}

func (s *Service) Capture(ctx context.Context, sessionID, serverSession string, eventType models.EventType, source, content string) (*models.Event, error) {
	event := &models.Event{
		ID:            uuid.New().String(),
		SessionID:     sessionID,
		ServerSession: serverSession,
		Timestamp:     time.Now(),
		Type:          eventType,
		Source:        source,
		Content:       redact.Secrets(content),
	}
	return event, s.store.Append(ctx, event)
}
