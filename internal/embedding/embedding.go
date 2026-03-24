package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client calls the Ollama embedding API.
type Client struct {
	BaseURL    string
	Model      string
	MaxRetries int
}

// NewClient creates a new embedding client.
func NewClient(baseURL, model string) *Client {
	return &Client{BaseURL: baseURL, Model: model, MaxRetries: 3}
}

type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// Embed generates an embedding for the given text, with retry on transient failures.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	delays := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}

	var lastErr error
	for attempt := range c.MaxRetries {
		result, err := c.embedOnce(ctx, text)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt < c.MaxRetries-1 {
			delay := delays[attempt]
			slog.Warn("embedding retry", "attempt", attempt+1, "delay", delay, "error", err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return nil, fmt.Errorf("embedding failed after %d attempts: %w", c.MaxRetries, lastErr)
}

func (c *Client) embedOnce(ctx context.Context, text string) ([]float32, error) {
	reqBody := embeddingRequest{Model: c.Model, Input: text}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		slog.Error("ollama error response", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("ollama returned %d", resp.StatusCode)
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("empty embedding returned")
	}

	f64 := result.Embeddings[0]
	f32 := make([]float32, len(f64))
	for i, v := range f64 {
		f32[i] = float32(v)
	}
	return f32, nil
}

// CheckHealth verifies Ollama is running and the model is available.
func (c *Client) CheckHealth(ctx context.Context) error {
	slog.Debug("checking ollama health", "url", c.BaseURL, "model", c.Model)

	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama not reachable at %s: %w\nStart it with: ollama serve", c.BaseURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return fmt.Errorf("decode tags: %w", err)
	}

	for _, m := range tags.Models {
		if m.Name == c.Model || m.Name == c.Model+":latest" {
			slog.Debug("ollama health OK", "model", m.Name)
			return nil
		}
	}
	return fmt.Errorf("model %q not found. Pull it with: ollama pull %s", c.Model, c.Model)
}
