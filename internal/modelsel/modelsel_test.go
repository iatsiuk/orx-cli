package modelsel

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"orx/internal/testutil"
)

func TestFetchModels(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		resp := ModelsResponse{
			Data: []APIModel{
				{ID: "openai/gpt-4o", Name: "GPT-4o", Architecture: Architecture{Modality: "text->text"}},
				{ID: "openai/dall-e-3", Name: "DALL-E 3", Architecture: Architecture{Modality: "text->image"}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	models, err := fetchModelsWithURL(context.Background(), "test-token", server.URL, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
}

func TestFetchModelsUnauthorized(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := fetchModelsWithURL(context.Background(), "bad-token", server.URL, false, nil)
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
}

func TestFetchModelsTimeout(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// block forever - context should cancel
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := fetchModelsWithURL(ctx, "token", server.URL, false, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestFilterTextModels(t *testing.T) {
	t.Parallel()

	models := []APIModel{
		{ID: "openai/gpt-4o", Architecture: Architecture{Modality: "text->text"}},
		{ID: "openai/dall-e-3", Architecture: Architecture{Modality: "text->image"}},
		{ID: "anthropic/claude-3", Architecture: Architecture{Modality: "text->text"}},
		{ID: "stability/sdxl", Architecture: Architecture{Modality: "text->image"}},
		{ID: "google/gemini-pro", Architecture: Architecture{Modality: "text+image->text"}},
		{ID: "openai/whisper", Architecture: Architecture{Modality: "audio->text"}},
	}

	filtered := filterTextModels(models)

	if len(filtered) != 3 {
		t.Errorf("expected 3 text models, got %d", len(filtered))
	}

	expected := map[string]bool{
		"openai/gpt-4o":      true,
		"anthropic/claude-3": true,
		"google/gemini-pro":  true,
	}
	for _, m := range filtered {
		if !expected[m.ID] {
			t.Errorf("unexpected model in result: %s", m.ID)
		}
	}
}

func TestCanProcessText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		modality string
		want     bool
	}{
		{"text->text", true},
		{"text+image->text", true},
		{"text+image+audio->text", true},
		{"text->image", false},
		{"text->audio", false},
		{"audio->text", false},
		{"image->text", false},
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		got := canProcessText(tt.modality)
		if got != tt.want {
			t.Errorf("canProcessText(%q) = %v, want %v", tt.modality, got, tt.want)
		}
	}
}

func TestSortByProvider(t *testing.T) {
	t.Parallel()

	models := []APIModel{
		{ID: "openai/gpt-4o"},
		{ID: "anthropic/claude-3"},
		{ID: "google/gemini"},
		{ID: "anthropic/claude-2"},
	}

	sorted := sortByProvider(models)

	expected := []string{
		"anthropic/claude-2",
		"anthropic/claude-3",
		"google/gemini",
		"openai/gpt-4o",
	}

	for i, m := range sorted {
		if m.ID != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], m.ID)
		}
	}
}

func TestFormatPricing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"0.000003", "$3.00 per 1M tokens"},
		{"0.0000025", "$2.50 per 1M tokens"},
		{"0", "FREE"},
		{"", "FREE"},
		{"0.00000000001", "$0.00 per 1M tokens"},
		{"0.000015", "$15.00 per 1M tokens"},
		{"0.0000001", "$0.10 per 1M tokens"},
	}

	for _, tt := range tests {
		got := formatPricing(tt.input)
		if got != tt.want {
			t.Errorf("formatPricing(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatContextLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int
		want  string
	}{
		{128000, "128,000 tokens"},
		{1000000, "1,000,000 tokens"},
		{4096, "4,096 tokens"},
		{100, "100 tokens"},
		{0, "Unknown"},
	}

	for _, tt := range tests {
		got := formatContextLength(tt.input)
		if got != tt.want {
			t.Errorf("formatContextLength(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
