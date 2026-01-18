package indexer

import (
	"context"
	"fmt"
	"os"

	"github.com/sashabaranov/go-openai"
	"github.com/user/xhub/internal/config"
)

// Embedder generates text embeddings
type Embedder struct {
	cfg    *config.Config
	client *openai.Client
}

func NewEmbedder(cfg *config.Config) (*Embedder, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}

	client := openai.NewClient(apiKey)

	return &Embedder{
		cfg:    cfg,
		client: client,
	}, nil
}

// Embed generates embeddings for text
func (e *Embedder) Embed(text string) ([]float32, error) {
	// Truncate text if too long (8191 tokens max for text-embedding-3-small)
	const maxChars = 30000
	if len(text) > maxChars {
		text = text[:maxChars]
	}

	model := e.cfg.Embeddings.Model
	if model == "" {
		model = "text-embedding-3-small"
	}

	resp, err := e.client.CreateEmbeddings(context.Background(), openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(model),
		Input: []string{text},
	})

	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return resp.Data[0].Embedding, nil
}

// EmbedBatch generates embeddings for multiple texts
func (e *Embedder) EmbedBatch(texts []string) ([][]float32, error) {
	// OpenAI supports batching
	const maxChars = 30000
	truncated := make([]string, len(texts))
	for i, t := range texts {
		if len(t) > maxChars {
			truncated[i] = t[:maxChars]
		} else {
			truncated[i] = t
		}
	}

	model := e.cfg.Embeddings.Model
	if model == "" {
		model = "text-embedding-3-small"
	}

	resp, err := e.client.CreateEmbeddings(context.Background(), openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(model),
		Input: truncated,
	})

	if err != nil {
		return nil, err
	}

	embeddings := make([][]float32, len(texts))
	for _, data := range resp.Data {
		if data.Index < len(embeddings) {
			embeddings[data.Index] = data.Embedding
		}
	}

	return embeddings, nil
}
