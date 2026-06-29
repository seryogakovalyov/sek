package reuse

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/seryogakovalyov/sek/internal/llm"
	"github.com/seryogakovalyov/sek/internal/models"
)

type goldenRetrievalCase struct {
	Name               string            `json:"name"`
	Task               string            `json:"task"`
	MaxEntries         int               `json:"max_entries"`
	VectorResults      []goldenKnowledge `json:"vector_results"`
	KeywordResults     []goldenKnowledge `json:"keyword_results"`
	ExpectedTop        string            `json:"expected_top"`
	ExpectedMatchTypes []string          `json:"expected_match_types,omitempty"`
}

type goldenKnowledge struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	Importance float64 `json:"importance"`
	AgeDays    int     `json:"age_days"`
}

func TestGoldenRetrievalEvals(t *testing.T) {
	data, err := os.ReadFile("testdata/golden_retrieval.json")
	if err != nil {
		t.Fatalf("read golden retrieval fixture: %v", err)
	}

	var cases []goldenRetrievalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parse golden retrieval fixture: %v", err)
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			store := &goldenStore{
				vectorResults:  toKnowledge(tc.VectorResults),
				keywordResults: toKnowledge(tc.KeywordResults),
			}
			engine := NewEngine(noopProvider{}, fixedEmbedder{}, store)

			result, err := engine.Query(context.Background(), models.ReuseRequest{
				Task: tc.Task,
				Budget: models.ContextBudget{
					MaxEntries: tc.MaxEntries,
				},
			})
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if len(result.Knowledge) == 0 {
				t.Fatal("expected knowledge results")
			}
			if got := result.Knowledge[0].ID; got != tc.ExpectedTop {
				t.Fatalf("expected top %s, got %s", tc.ExpectedTop, got)
			}
			if len(tc.ExpectedMatchTypes) > 0 {
				if got := result.Knowledge[0].Breakdown.MatchTypes; !reflect.DeepEqual(got, tc.ExpectedMatchTypes) {
					t.Fatalf("expected match types %v, got %v", tc.ExpectedMatchTypes, got)
				}
			}
			if result.Knowledge[0].Breakdown.FinalScore <= 0 {
				t.Fatalf("expected final score breakdown, got %#v", result.Knowledge[0].Breakdown)
			}
		})
	}
}

func toKnowledge(items []goldenKnowledge) []models.Knowledge {
	now := time.Now()
	result := make([]models.Knowledge, len(items))
	for i, item := range items {
		result[i] = models.Knowledge{
			ID:         item.ID,
			Level:      models.LevelLesson,
			CreatedAt:  now.Add(-time.Duration(item.AgeDays) * 24 * time.Hour),
			Content:    item.Content,
			Score:      item.Score,
			Importance: models.Importance(item.Importance),
		}
	}
	return result
}

type goldenStore struct {
	vectorResults  []models.Knowledge
	keywordResults []models.Knowledge
}

func (s *goldenStore) Save(context.Context, *models.Knowledge) error {
	return nil
}

func (s *goldenStore) Search(context.Context, string, int) ([]models.Knowledge, error) {
	return append([]models.Knowledge(nil), s.keywordResults...), nil
}

func (s *goldenStore) SearchSimilar(context.Context, []float32, int) ([]models.Knowledge, error) {
	return append([]models.Knowledge(nil), s.vectorResults...), nil
}

func (s *goldenStore) FindSimilar(context.Context, []float32, float64, int) ([]models.Knowledge, error) {
	return nil, nil
}

func (s *goldenStore) UpdateSourceIDs(context.Context, string, []string) error {
	return nil
}

func (s *goldenStore) List(context.Context, models.KnowledgeLevel, int) ([]models.Knowledge, error) {
	return nil, nil
}

type fixedEmbedder struct{}

func (fixedEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1, 0}}, nil
}

type noopProvider struct{}

func (noopProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{}, nil
}
