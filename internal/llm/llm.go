// Package llm is a minimal, dependency-free client for the three AI
// providers the weekly recap supports (Anthropic, OpenAI, Gemini). Each
// provider gets its own small HTTP client rather than pulling in an SDK,
// matching the rest of this codebase's approach to external APIs (see
// internal/sleeper and internal/discord).
package llm

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Config selects and authenticates a provider.
type Config struct {
	// Provider is one of "anthropic", "openai", or "gemini".
	Provider string
	APIKey   string
	Model    string
}

// Client generates text from a system prompt (the assistant's persona and
// instructions) and a user prompt (the data/request for this call).
type Client interface {
	Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// requestTimeout is generous because writing a newsletter-length recap can
// take a while, especially from slower/larger models.
const requestTimeout = 90 * time.Second

// New builds a Client for cfg.Provider. Callers that already validate
// Provider up front (see config.loadLLMConfig) should never see the error
// case in practice.
func New(cfg Config) (Client, error) {
	httpClient := &http.Client{Timeout: requestTimeout}
	switch cfg.Provider {
	case "anthropic":
		return &anthropicClient{apiKey: cfg.APIKey, model: cfg.Model, httpClient: httpClient}, nil
	case "openai":
		return &openAIClient{apiKey: cfg.APIKey, model: cfg.Model, httpClient: httpClient}, nil
	case "gemini":
		return &geminiClient{apiKey: cfg.APIKey, model: cfg.Model, httpClient: httpClient}, nil
	default:
		return nil, fmt.Errorf("llm: unknown provider %q (want anthropic, openai, or gemini)", cfg.Provider)
	}
}
