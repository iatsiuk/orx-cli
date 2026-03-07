package modelsel

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"orx/internal/config"
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

func TestPreSelectModels(t *testing.T) {
	t.Parallel()

	models := []APIModel{
		{ID: "openai/gpt-4o", Name: "GPT-4o"},
		{ID: "anthropic/claude-3", Name: "Claude 3"},
		{ID: "google/gemini-pro", Name: "Gemini Pro"},
	}

	preSelected := []string{"openai/gpt-4o", "google/gemini-pro"}
	app := newTuiApp(models, preSelected)

	if !app.selected["openai/gpt-4o"] {
		t.Error("expected openai/gpt-4o to be pre-selected")
	}
	if !app.selected["google/gemini-pro"] {
		t.Error("expected google/gemini-pro to be pre-selected")
	}
	if app.selected["anthropic/claude-3"] {
		t.Error("expected anthropic/claude-3 to NOT be pre-selected")
	}
	if len(app.selected) != 2 {
		t.Errorf("expected 2 selected models, got %d", len(app.selected))
	}
}

func TestPreSelectModels_NonExistent(t *testing.T) {
	t.Parallel()

	models := []APIModel{
		{ID: "openai/gpt-4o", Name: "GPT-4o"},
	}

	preSelected := []string{"openai/gpt-4o", "nonexistent/model"}
	app := newTuiApp(models, preSelected)

	if !app.selected["openai/gpt-4o"] {
		t.Error("expected openai/gpt-4o to be pre-selected")
	}
	if app.selected["nonexistent/model"] {
		t.Error("expected nonexistent/model to NOT be in selected map")
	}
	if len(app.selected) != 1 {
		t.Errorf("expected 1 selected model, got %d", len(app.selected))
	}
}

func TestNextEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		current string
		want    string
	}{
		{"", "none"},
		{"none", "minimal"},
		{"minimal", "low"},
		{"low", "medium"},
		{"medium", "high"},
		{"high", "xhigh"},
		{"xhigh", ""},
	}

	for _, tt := range tests {
		got := nextEffort(tt.current)
		if got != tt.want {
			t.Errorf("nextEffort(%q) = %q, want %q", tt.current, got, tt.want)
		}
	}
}

func TestSupportsReasoning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		params []string
		want   bool
	}{
		{[]string{"reasoning", "temperature"}, true},
		{[]string{"temperature", "top_p"}, false},
		{[]string{"reasoning"}, true},
		{[]string{}, false},
		{nil, false},
	}

	for _, tt := range tests {
		got := supportsReasoning(tt.params)
		if got != tt.want {
			t.Errorf("supportsReasoning(%v) = %v, want %v", tt.params, got, tt.want)
		}
	}
}

func TestFilterReasoningSelectedModels(t *testing.T) {
	t.Parallel()

	models := []config.SelectedModel{
		{ID: "openai/o3", SupportedParameters: []string{"reasoning", "temperature"}},
		{ID: "openai/gpt-4o", SupportedParameters: []string{"temperature", "top_p"}},
		{ID: "anthropic/claude-opus", SupportedParameters: []string{"reasoning"}},
		{ID: "google/gemini", SupportedParameters: []string{"temperature"}},
	}

	filtered := filterReasoningSelectedModels(models)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 reasoning models, got %d", len(filtered))
	}
	if filtered[0].ID != "openai/o3" {
		t.Errorf("expected filtered[0] = openai/o3, got %s", filtered[0].ID)
	}
	if filtered[1].ID != "anthropic/claude-opus" {
		t.Errorf("expected filtered[1] = anthropic/claude-opus, got %s", filtered[1].ID)
	}
}

func TestFilterReasoningSelectedModels_Empty(t *testing.T) {
	t.Parallel()

	models := []config.SelectedModel{
		{ID: "openai/gpt-4o", SupportedParameters: []string{"temperature"}},
	}

	filtered := filterReasoningSelectedModels(models)

	if len(filtered) != 0 {
		t.Errorf("expected 0 reasoning models, got %d", len(filtered))
	}
}
