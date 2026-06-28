package llm

import "context"

type Embedder interface {
	Embed(ctx context.Context, input []string) ([][]float32, error)
}
