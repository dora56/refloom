package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbedSendsSingleInput(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("path = %s, want /api/embed", r.URL.Path)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if got := req["model"]; got != "test-model" {
			t.Fatalf("model = %v, want test-model", got)
		}
		if got := req["input"]; got != "hello" {
			t.Fatalf("input = %v, want hello", got)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float64{{1, 2, 3}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-model")
	client.HTTPClient = server.Client()

	got, err := client.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("embedding = %v, want [1 2 3]", got)
	}
}

func TestEmbedBatchSendsArrayInput(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.Model != "test-model" {
			t.Fatalf("model = %s, want test-model", req.Model)
		}
		if len(req.Input) != 2 || req.Input[0] != "a" || req.Input[1] != "b" {
			t.Fatalf("input = %v, want [a b]", req.Input)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float64{{1, 2}, {3, 4}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-model")
	client.HTTPClient = server.Client()

	got, err := client.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("embeddings len = %d, want 2", len(got))
	}
	if got[0][0] != 1 || got[1][1] != 4 {
		t.Fatalf("embeddings = %v, want [[1 2] [3 4]]", got)
	}
}

func TestEmbedBatchEmptyInput(t *testing.T) {
	client := NewClient("http://localhost:11434", "test-model")

	got, err := client.EmbedBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if got != nil {
		t.Fatalf("EmbedBatch(nil) = %v, want nil", got)
	}
}
