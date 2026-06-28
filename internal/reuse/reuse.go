package reuse

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/anomalyco/sek/internal/llm"
	"github.com/anomalyco/sek/internal/models"
	"github.com/anomalyco/sek/internal/store"
)

const (
	recencyHalfLife  = 30 * 24 * time.Hour
	maxRecencyBoost  = 0.3
	maxUsageBoost    = 0.3
	usageBoostPerUse = 0.05
)

type Engine struct {
	llm      llm.Provider
	embedder llm.Embedder
	store    store.KnowledgeStore
}

func NewEngine(llm llm.Provider, embedder llm.Embedder, s store.KnowledgeStore) *Engine {
	return &Engine{llm: llm, embedder: embedder, store: s}
}

func (e *Engine) Query(ctx context.Context, req models.ReuseRequest) (*models.ReuseResult, error) {
	var knowledge []models.Knowledge
	limit := req.Budget.MaxEntries
	if limit <= 0 {
		limit = 10
	}

	// Phase 1: vector search
	embeddings, embedErr := e.embedder.Embed(ctx, []string{req.Task})
	if embedErr == nil && len(embeddings) > 0 {
		k, err := e.store.SearchSimilar(ctx, embeddings[0], limit)
		if err == nil {
			knowledge = k
		}
	}

	// Phase 2: supplement with keyword results if we don't have enough
	if len(knowledge) < limit {
		needed := limit - len(knowledge)
		k, err := e.store.Search(ctx, req.Task, needed)
		if err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}

		// merge, dedup by ID
		seen := make(map[string]bool, len(knowledge))
		for _, kn := range knowledge {
			seen[kn.ID] = true
		}
		for _, kn := range k {
			if !seen[kn.ID] {
				kn.Score = 0.1 // low default score for keyword hits
				knowledge = append(knowledge, kn)
				seen[kn.ID] = true
			}
		}
	}

	knowledge = applyScoreAdjustments(knowledge)
	// re-sort after merge
	for i := 0; i < len(knowledge); i++ {
		for j := i + 1; j < len(knowledge); j++ {
			if knowledge[j].Score > knowledge[i].Score {
				knowledge[i], knowledge[j] = knowledge[j], knowledge[i]
			}
		}
	}

	if req.Budget.MaxTokens > 0 {
		knowledge = truncateByTokens(knowledge, req.Budget.MaxTokens)
	}

	totalTokens := 0
	for _, k := range knowledge {
		totalTokens += len(k.Content) / 4
	}

	return &models.ReuseResult{
		Knowledge:   knowledge,
		TotalTokens: totalTokens,
	}, nil
}

func applyScoreAdjustments(knowledge []models.Knowledge) []models.Knowledge {
	now := time.Now()
	result := make([]models.Knowledge, len(knowledge))
	for i, k := range knowledge {
		recencyBoost := recencyFactor(k.CreatedAt, now)
		importanceBoost := float64(k.Importance)
		usageBoost := math.Min(float64(k.UsageCount)*usageBoostPerUse, maxUsageBoost)
		k.Score = k.Score * (1 + recencyBoost + importanceBoost + usageBoost)
		result[i] = k
	}

	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Score > result[i].Score {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

func recencyFactor(createdAt time.Time, now time.Time) float64 {
	age := now.Sub(createdAt)
	if age <= 0 {
		return maxRecencyBoost
	}
	boost := maxRecencyBoost * math.Exp(-float64(age)/float64(recencyHalfLife))
	if boost < 0 {
		return 0
	}
	return boost
}

func truncateByTokens(knowledge []models.Knowledge, maxTokens int) []models.Knowledge {
	tokens := 0
	for i, k := range knowledge {
		tokens += len(k.Content) / 4
		if tokens > maxTokens {
			return knowledge[:i]
		}
	}
	return knowledge
}
