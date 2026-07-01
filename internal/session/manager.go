package session

import (
	"context"
	"log"
	"time"

	"github.com/seryogakovalyov/sek/internal/distill"
	"github.com/seryogakovalyov/sek/internal/llm"
	"github.com/seryogakovalyov/sek/internal/models"
	"github.com/seryogakovalyov/sek/internal/store"
)

type Manager struct {
	store      store.Store
	provider   llm.Provider
	embedder   llm.Embedder
	modelName  string
	sessionID  string
	projectDir string
}

type Options struct {
	Store      store.Store
	Provider   llm.Provider
	Embedder   llm.Embedder
	ModelName  string
	SessionID  string
	ProjectDir string
}

func NewManager(opts Options) *Manager {
	return &Manager{
		store:      opts.Store,
		provider:   opts.Provider,
		embedder:   opts.Embedder,
		modelName:  opts.ModelName,
		sessionID:  opts.SessionID,
		projectDir: opts.ProjectDir,
	}
}

func (m *Manager) Start(ctx context.Context) {
	if m == nil || m.store == nil || m.sessionID == "" {
		return
	}
	snapshotCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	snapshot := CaptureGitSnapshot(snapshotCtx, m.projectDir)
	now := time.Now()
	if err := m.store.RecoverOpenSessions(ctx, m.projectDir, now.Format(time.RFC3339Nano), snapshot); err != nil {
		log.Printf("session manager: recover open sessions: %v", err)
	}
	if err := m.store.StartSession(ctx, &models.SessionLog{
		ID:            m.sessionID,
		StartedAt:     now,
		ProjectDir:    m.projectDir,
		Status:        "running",
		StartSnapshot: snapshot,
	}); err != nil {
		log.Printf("session manager: start: %v", err)
	}
}

func (m *Manager) Finish(ctx context.Context) {
	if m == nil || m.store == nil || m.sessionID == "" {
		return
	}
	snapshotCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	snapshot := CaptureGitSnapshot(snapshotCtx, m.projectDir)
	if err := m.store.FinishSession(ctx, m.sessionID, time.Now().Format(time.RFC3339Nano), snapshot); err != nil {
		log.Printf("session manager: finish: %v", err)
	}

	if ctx.Err() != nil {
		return
	}
	log.Println("session ended, running session digest...")
	distill.SessionDigest(ctx, m.store, m.provider, m.embedder, m.modelName, m.sessionID)
}

func (m *Manager) SessionID() string {
	if m == nil {
		return ""
	}
	return m.sessionID
}
