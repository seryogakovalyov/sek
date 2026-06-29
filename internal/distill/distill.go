package distill

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/seryogakovalyov/sek/internal/llm"
	"github.com/seryogakovalyov/sek/internal/models"
	"github.com/seryogakovalyov/sek/internal/redact"
	"github.com/seryogakovalyov/sek/internal/store"
)

const (
	dedupThreshold = 0.95
	promoteAfter   = 3
)

type Pipeline struct {
	llm      llm.Provider
	embedder llm.Embedder
	model    string
	store    store.KnowledgeStore
	obsCount int
}

func NewPipeline(llm llm.Provider, embedder llm.Embedder, model string, s store.KnowledgeStore) *Pipeline {
	return &Pipeline{llm: llm, embedder: embedder, model: model, store: s}
}

func (p *Pipeline) Process(ctx context.Context, events []models.Event) error {
	for _, event := range events {
		obs, err := p.distillEvent(ctx, event)
		if err != nil {
			return fmt.Errorf("distill event %s: %w", event.ID, err)
		}
		if obs == nil {
			continue
		}

		obs.EventType = event.Type
		obs.Importance = models.EventImportance(event.Type)

		saved, err := p.embedAndSaveDedup(ctx, obs)
		if err != nil {
			return fmt.Errorf("save observation: %w", err)
		}
		if !saved {
			continue
		}

		p.obsCount++
		if p.obsCount%promoteAfter == 0 {
			if err := p.promote(ctx); err != nil {
				log.Printf("promote warning: %v", err)
			}
		}
	}
	return nil
}

func (p *Pipeline) embedAndSaveDedup(ctx context.Context, k *models.Knowledge) (bool, error) {
	embeddings, err := p.embedder.Embed(ctx, []string{k.Content})
	if err == nil && len(embeddings) > 0 {
		k.Embedding = embeddings[0]

		dups, err := p.store.FindSimilar(ctx, k.Embedding, dedupThreshold, 1)
		if err == nil && len(dups) > 0 {
			existing := dups[0]
			merged := mergeSourceIDs(existing.SourceIDs, k.SourceIDs)
			if err := p.store.UpdateSourceIDs(ctx, existing.ID, merged); err != nil {
				log.Printf("dedup merge source_ids warning: %v", err)
			}
			log.Printf("dedup: skipped observation %s, merged into existing %s (score: %.3f)", k.ID, existing.ID, dups[0].Score)
			return false, nil
		}
	} else if err != nil {
		log.Printf("embed warning: %v", err)
	}

	return true, p.store.Save(ctx, k)
}

func mergeSourceIDs(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	result := make([]string, 0, len(a)+len(b))
	for _, id := range a {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	for _, id := range b {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func (p *Pipeline) promote(ctx context.Context) error {
	observations, err := p.store.List(ctx, models.LevelObservation, 50)
	if err != nil {
		return fmt.Errorf("list observations: %w", err)
	}
	if len(observations) < promoteAfter {
		return nil
	}

	lessons, err := p.store.List(ctx, models.LevelLesson, 50)
	if err != nil {
		return fmt.Errorf("list lessons: %w", err)
	}

	usedObs := usedSourceIDs(lessons)
	unpromoted := filterUnpromoted(observations, usedObs)

	clusters := clusterBySimilarity(unpromoted, 0.7)
	for _, cluster := range clusters {
		if len(cluster) < promoteAfter {
			continue
		}
		if err := p.composeLesson(ctx, cluster); err != nil {
			log.Printf("compose lesson warning: %v", err)
		}
	}

	patterns, err := p.store.List(ctx, models.LevelPattern, 50)
	if err != nil {
		return fmt.Errorf("list patterns: %w", err)
	}

	usedLessons := usedSourceIDs(patterns)
	unpromotedLessons := filterUnpromoted(lessons, usedLessons)

	lessonClusters := clusterBySimilarity(unpromotedLessons, 0.7)
	for _, cluster := range lessonClusters {
		if len(cluster) < promoteAfter {
			continue
		}
		if err := p.composePattern(ctx, cluster); err != nil {
			log.Printf("compose pattern warning: %v", err)
		}
	}

	return nil
}

func (p *Pipeline) distillEvent(ctx context.Context, event models.Event) (*models.Knowledge, error) {
	resp, err := p.llm.Chat(ctx, llm.ChatRequest{
		Model: p.model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: `Extract one concise, reusable engineering observation from this event.

Preserve concrete technical details that make the lesson actionable:
- file paths, config keys/sections, commands, flags, tool names, model names, API names, error messages, ports, and version names
- the root cause or decision rationale when present
- if the event contains an error, preserve the exact error text or the most distinctive error substring verbatim
- if the event contains a command that fixed or verified the issue, preserve the exact command or the essential command + flags verbatim

Do not over-generalize. A future engineer should be able to repeat the fix or avoid the same mistake without reading the original event.
Return only the observation text, 1-3 sentences, nothing else.`},
			{Role: llm.RoleUser, Content: fmt.Sprintf("Event type: %s\nSource: %s\nContent: %s", event.Type, event.Source, redact.Secrets(event.Content))},
		},
		Temperature: 0.3,
	})
	if err != nil {
		return nil, err
	}
	if resp.Content == "" {
		return nil, nil
	}
	return &models.Knowledge{
		ID:        fmt.Sprintf("obs-%s", event.ID),
		Level:     models.LevelObservation,
		CreatedAt: time.Now(),
		Content:   redact.Secrets(resp.Content),
		SourceIDs: []string{event.ID},
	}, nil
}

type centroid struct {
	embedding []float32
	indices   []int
}

func usedSourceIDs(items []models.Knowledge) map[string]bool {
	used := make(map[string]bool)
	for _, item := range items {
		for _, id := range item.SourceIDs {
			used[id] = true
		}
	}
	return used
}

func filterUnpromoted(items []models.Knowledge, used map[string]bool) []models.Knowledge {
	var result []models.Knowledge
	for _, item := range items {
		if !used[item.ID] {
			result = append(result, item)
		}
	}
	return result
}

func clusterBySimilarity(items []models.Knowledge, threshold float64) [][]models.Knowledge {
	if len(items) == 0 {
		return nil
	}

	var centroids []centroid

outer:
	for i, item := range items {
		if len(item.Embedding) == 0 {
			continue
		}
		for ci := range centroids {
			sim := cosineSimilarity(item.Embedding, centroids[ci].embedding)
			if sim >= threshold {
				centroids[ci].indices = append(centroids[ci].indices, i)
				recomputeCentroid(&centroids[ci], items)
				continue outer
			}
		}
		centroids = append(centroids, centroid{
			embedding: item.Embedding,
			indices:   []int{i},
		})
	}

	var clusters [][]models.Knowledge
	for _, c := range centroids {
		if len(c.indices) < 3 {
			continue
		}
		cluster := make([]models.Knowledge, len(c.indices))
		for j, idx := range c.indices {
			cluster[j] = items[idx]
		}
		clusters = append(clusters, cluster)
	}
	return clusters
}

func recomputeCentroid(c *centroid, items []models.Knowledge) {
	if len(c.indices) == 0 {
		return
	}
	dim := len(items[c.indices[0]].Embedding)
	if dim == 0 {
		return
	}
	sum := make([]float64, dim)
	for _, idx := range c.indices {
		for d := 0; d < dim && d < len(items[idx].Embedding); d++ {
			sum[d] += float64(items[idx].Embedding[d])
		}
	}
	avg := make([]float32, dim)
	for d := 0; d < dim; d++ {
		avg[d] = float32(sum[d] / float64(len(c.indices)))
	}
	c.embedding = avg
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
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

func (p *Pipeline) composeLesson(ctx context.Context, cluster []models.Knowledge) error {
	content := ""
	for i, obs := range cluster {
		content += fmt.Sprintf("%d. %s\n", i+1, obs.Content)
	}

	resp, err := p.llm.Chat(ctx, llm.ChatRequest{
		Model: p.model,
		Messages: []llm.Message{
			{
				Role:    llm.RoleSystem,
				Content: "You are distilling engineering observations into a compact lesson. Given several related observations from a software project, compose them into a single concise, actionable lesson. A lesson captures a reusable insight: what to do, why, and when. Return only the lesson text, nothing else.",
			},
			{
				Role:    llm.RoleUser,
				Content: fmt.Sprintf("Compose these related observations into a single lesson:\n\n%s", content),
			},
		},
		Temperature: 0.3,
	})
	if err != nil {
		return fmt.Errorf("compose lesson llm: %w", err)
	}
	if resp.Content == "" {
		return nil
	}

	sourceIDs := make([]string, len(cluster))
	for i, obs := range cluster {
		sourceIDs[i] = obs.ID
	}

	lesson := &models.Knowledge{
		ID:         fmt.Sprintf("lesson-%d", time.Now().UnixNano()),
		Level:      models.LevelLesson,
		CreatedAt:  time.Now(),
		Content:    redact.Secrets(resp.Content),
		SourceIDs:  sourceIDs,
		Importance: models.ImportanceNormal,
	}

	embeddings, err := p.embedder.Embed(ctx, []string{lesson.Content})
	if err == nil && len(embeddings) > 0 {
		lesson.Embedding = embeddings[0]
	} else if err != nil {
		log.Printf("embed lesson warning: %v", err)
	}

	return p.store.Save(ctx, lesson)
}

func (p *Pipeline) composePattern(ctx context.Context, cluster []models.Knowledge) error {
	content := ""
	for i, lesson := range cluster {
		content += fmt.Sprintf("%d. %s\n", i+1, lesson.Content)
	}

	resp, err := p.llm.Chat(ctx, llm.ChatRequest{
		Model: p.model,
		Messages: []llm.Message{
			{
				Role:    llm.RoleSystem,
				Content: "You are distilling engineering lessons into a high-level pattern. Given several related lessons from a software project, compose them into a single architectural or process pattern. A pattern describes a recurring approach: context, problem, solution, and consequences. Return only the pattern text, nothing else.",
			},
			{
				Role:    llm.RoleUser,
				Content: fmt.Sprintf("Compose these related lessons into a single pattern:\n\n%s", content),
			},
		},
		Temperature: 0.3,
	})
	if err != nil {
		return fmt.Errorf("compose pattern llm: %w", err)
	}
	if resp.Content == "" {
		return nil
	}

	sourceIDs := make([]string, len(cluster))
	for i, l := range cluster {
		sourceIDs[i] = l.ID
	}

	pattern := &models.Knowledge{
		ID:         fmt.Sprintf("pattern-%d", time.Now().UnixNano()),
		Level:      models.LevelPattern,
		CreatedAt:  time.Now(),
		Content:    redact.Secrets(resp.Content),
		SourceIDs:  sourceIDs,
		Importance: models.ImportanceHigh,
	}

	embeddings, err := p.embedder.Embed(ctx, []string{pattern.Content})
	if err == nil && len(embeddings) > 0 {
		pattern.Embedding = embeddings[0]
	} else if err != nil {
		log.Printf("embed pattern warning: %v", err)
	}

	return p.store.Save(ctx, pattern)
}

func SessionDigest(ctx context.Context, st store.Store, provider llm.Provider, embedder llm.Embedder, modelName string, serverSession string) {
	if ctx.Err() != nil || serverSession == "" {
		return
	}

	events, err := st.EventsByServerSession(ctx, serverSession, 100)
	if err != nil {
		log.Printf("session digest: events by server session: %v", err)
		return
	}
	if len(events) < 3 {
		log.Printf("session digest: only %d events for session %s, skipping", len(events), serverSession)
		return
	}

	if ctx.Err() != nil {
		return
	}
	makeDigest(ctx, st, provider, embedder, modelName, events)
}

func makeDigest(ctx context.Context, st store.Store, provider llm.Provider, embedder llm.Embedder, modelName string, events []models.Event) {
	content := ""
	for i, e := range events {
		content += fmt.Sprintf("%d. [%s] %s: %s\n", i+1, e.Type, e.Source, truncateString(redact.Secrets(e.Content), 200))
	}

	resp, err := provider.Chat(ctx, llm.ChatRequest{
		Model: modelName,
		Messages: []llm.Message{
			{
				Role: llm.RoleSystem,
				Content: "Summarize the following engineering session events into a compact, actionable lesson. " +
					"A lesson captures a reusable insight: context, what was learned, and why it matters. " +
					"Focus on decisions made, problems encountered, and solutions found. " +
					"Return only the lesson text, nothing else.",
			},
			{
				Role:    llm.RoleUser,
				Content: fmt.Sprintf("Session events:\n\n%s", content),
			},
		},
		Temperature: 0.3,
	})
	if err != nil {
		log.Printf("session digest: llm: %v", err)
		return
	}
	if resp.Content == "" {
		return
	}

	sourceIDs := make([]string, len(events))
	for i, e := range events {
		sourceIDs[i] = e.ID
	}

	obs := &models.Knowledge{
		ID:         fmt.Sprintf("digest-%d", time.Now().UnixNano()),
		Level:      models.LevelLesson,
		CreatedAt:  time.Now(),
		Content:    redact.Secrets(resp.Content),
		SourceIDs:  sourceIDs,
		EventType:  models.EventDecision,
		Importance: models.ImportanceHigh,
	}

	embeddings, err := embedder.Embed(ctx, []string{obs.Content})
	if err == nil && len(embeddings) > 0 {
		obs.Embedding = embeddings[0]
	}

	if err := st.Save(ctx, obs); err != nil {
		log.Printf("session digest: save: %v", err)
		return
	}
	log.Printf("session digest: saved %s (%d events)", obs.ID, len(events))
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
