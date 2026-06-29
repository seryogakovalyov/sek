package reuse

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/anomalyco/sek/internal/llm"
	"github.com/anomalyco/sek/internal/models"
	"github.com/anomalyco/sek/internal/store"
)

const (
	recencyHalfLife       = 30 * 24 * time.Hour
	maxRecencyBoost       = 0.10
	maxUsageBoost         = 0.20
	usageBoostPerUse      = 0.04
	importanceBoostFactor = 0.19
	hybridCandidateFactor = 3
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
	limit := req.Budget.MaxEntries
	if limit <= 0 {
		limit = 10
	}
	candidateLimit := limit * hybridCandidateFactor

	var vectorResults []models.Knowledge
	embeddings, embedErr := e.embedder.Embed(ctx, []string{req.Task})
	if embedErr == nil && len(embeddings) > 0 {
		k, err := e.store.SearchSimilar(ctx, embeddings[0], candidateLimit)
		if err == nil {
			vectorResults = k
		}
	}

	keywordResults, err := e.store.Search(ctx, req.Task, candidateLimit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	knowledge := mergeHybridResults(vectorResults, keywordResults)
	knowledge = applyScoreAdjustments(knowledge)
	if limit < len(knowledge) {
		knowledge = knowledge[:limit]
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

func mergeHybridResults(vectorResults, keywordResults []models.Knowledge) []models.Knowledge {
	merged := make(map[string]models.Knowledge, len(vectorResults)+len(keywordResults))

	for _, k := range vectorResults {
		k.Breakdown.VectorScore = k.Score
		k.Breakdown.MatchTypes = appendMatchType(k.Breakdown.MatchTypes, "vector")
		merged[k.ID] = k
	}

	for _, k := range keywordResults {
		existing, ok := merged[k.ID]
		if !ok {
			k.Breakdown.KeywordScore = k.Score
			k.Breakdown.MatchTypes = appendMatchType(k.Breakdown.MatchTypes, "keyword")
			merged[k.ID] = k
			continue
		}
		if k.Score > existing.Breakdown.KeywordScore {
			existing.Breakdown.KeywordScore = k.Score
		}
		existing.Breakdown.MatchTypes = appendMatchType(existing.Breakdown.MatchTypes, "keyword")
		if existing.Content == "" {
			existing.Content = k.Content
		}
		merged[k.ID] = existing
	}

	result := make([]models.Knowledge, 0, len(merged))
	for _, k := range merged {
		base := math.Max(k.Breakdown.VectorScore, k.Breakdown.KeywordScore)
		if k.Breakdown.VectorScore > 0 && k.Breakdown.KeywordScore > 0 {
			base = math.Min(1, base+0.1*math.Min(k.Breakdown.VectorScore, k.Breakdown.KeywordScore))
		}
		k.Score = base
		result = append(result, k)
	}
	return result
}

func appendMatchType(types []string, matchType string) []string {
	for _, existing := range types {
		if existing == matchType {
			return types
		}
	}
	return append(types, matchType)
}

func applyScoreAdjustments(knowledge []models.Knowledge) []models.Knowledge {
	now := time.Now()
	result := make([]models.Knowledge, len(knowledge))
	for i, k := range knowledge {
		baseScore := k.Score
		recencyBoost := recencyFactor(k.CreatedAt, now)
		importanceBoost := float64(k.Importance) * importanceBoostFactor
		usageBoost := math.Min(float64(k.UsageCount)*usageBoostPerUse, maxUsageBoost)
		k.Score = baseScore * (1 + recencyBoost + importanceBoost + usageBoost)
		k.Breakdown.BaseScore = baseScore
		k.Breakdown.RecencyBoost = recencyBoost
		k.Breakdown.ImportanceBoost = importanceBoost
		k.Breakdown.UsageBoost = usageBoost
		k.Breakdown.FinalScore = k.Score
		result[i] = k
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].CreatedAt.After(result[j].CreatedAt)
		}
		return result[i].Score > result[j].Score
	})

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
