package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orx/internal/client"
	"orx/internal/config"
	"orx/internal/runner"
)

func TestRootCmd_MissingToken(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "") // clear env var

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "orx.json")
	if err := os.WriteFile(cfgPath, []byte(`{"models":[{"name":"t","model":"m","enabled":true}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	promptPath := filepath.Join(tmpDir, "prompt.txt")
	if err := os.WriteFile(promptPath, []byte("test prompt"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"-c", cfgPath, "-p", promptPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !errors.Is(err, ErrTokenRequired) {
		t.Errorf("expected ErrTokenRequired, got: %v", err)
	}
}

func TestRootCmd_TokenFromEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-token-from-env")

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "orx.json")
	// config with no enabled models to trigger early error (after token validation)
	if err := os.WriteFile(cfgPath, []byte(`{"models":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	promptPath := filepath.Join(tmpDir, "prompt.txt")
	if err := os.WriteFile(promptPath, []byte("test prompt"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"-c", cfgPath, "-p", promptPath})

	err := cmd.Execute()
	// should fail with "no enabled models", not "token required"
	if err == nil || strings.Contains(err.Error(), "token") {
		t.Errorf("token from env should be accepted, got: %v", err)
	}
}

func TestRootCmd_ConfigNotFound(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-token")

	tmpDir := t.TempDir()
	promptPath := filepath.Join(tmpDir, "prompt.txt")
	if err := os.WriteFile(promptPath, []byte("test prompt"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"-c", "/nonexistent/config.json", "-p", promptPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRootCmd_PromptFileNotFound(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-token")

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "orx.json")
	if err := os.WriteFile(cfgPath, []byte(`{"models":[{"name":"t","model":"m","enabled":true}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"-c", cfgPath, "-p", "/nonexistent/prompt.txt"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing prompt file")
	}
	if !strings.Contains(err.Error(), "read prompt") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRootCmd_EmptyPromptFile(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-token")

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "orx.json")
	if err := os.WriteFile(cfgPath, []byte(`{"models":[{"name":"t","model":"m","enabled":true}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	promptPath := filepath.Join(tmpDir, "prompt.txt")
	if err := os.WriteFile(promptPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"-c", cfgPath, "-p", promptPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
	if !errors.Is(err, ErrEmptyPrompt) {
		t.Errorf("expected ErrEmptyPrompt, got: %v", err)
	}
}

func TestInitCmd_CreatesConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "orx.json")

	stderr := &bytes.Buffer{}

	cmd := newRootCmd()
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"init", "-o", outPath, "--template"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Error("config file should be created")
	}

	if !strings.Contains(stderr.String(), outPath) {
		t.Error("should print created file path to stderr")
	}
}

func TestInitCmd_CreatesNestedDirs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "nested", "dir", "orx.json")

	cmd := newRootCmd()
	cmd.SetArgs([]string{"init", "-o", outPath, "--template"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Error("config file should be created in nested directory")
	}
}

func TestInitCmd_ExistingFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           string
		expectOverwrite bool
		expectMessage   string
	}{
		{"abort on n", "n\n", false, "Aborted"},
		{"overwrite on y", "y\n", true, "Configuration file created"},
		{"overwrite on yes", "yes\n", true, "Configuration file created"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			outPath := filepath.Join(tmpDir, "orx.json")

			originalContent := []byte(`{"existing": true}`)
			if err := os.WriteFile(outPath, originalContent, 0o600); err != nil {
				t.Fatal(err)
			}

			stdin := strings.NewReader(tt.input)
			stderr := &bytes.Buffer{}

			cmd := newRootCmd()
			cmd.SetIn(stdin)
			cmd.SetErr(stderr)
			cmd.SetArgs([]string{"init", "-o", outPath, "--template"})

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			content, _ := os.ReadFile(outPath)
			wasOverwritten := !bytes.Equal(content, originalContent)

			if wasOverwritten != tt.expectOverwrite {
				t.Errorf("overwrite=%v, want %v", wasOverwritten, tt.expectOverwrite)
			}

			if !strings.Contains(stderr.String(), tt.expectMessage) {
				t.Errorf("stderr %q should contain %q", stderr.String(), tt.expectMessage)
			}

			if tt.expectOverwrite {
				cfg, err := config.Load(outPath)
				if err != nil {
					t.Fatalf("new config should be valid: %v", err)
				}
				if len(cfg.Models) == 0 {
					t.Error("expected models in generated config")
				}
			}
		})
	}
}

func TestReadPrompt_FromStdin(t *testing.T) {
	t.Parallel()

	input := "prompt from stdin"
	stdin := strings.NewReader(input)

	result, err := readPrompt(stdin, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestReadPrompt_EmptyStdin(t *testing.T) {
	t.Parallel()

	stdin := strings.NewReader("")

	_, err := readPrompt(stdin, "")
	if err == nil {
		t.Fatal("expected error for empty stdin")
	}
	if !errors.Is(err, ErrEmptyPrompt) {
		t.Errorf("expected ErrEmptyPrompt, got: %v", err)
	}
}

func TestReadPrompt_FileOverridesStdin(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "prompt.txt")
	fileContent := "prompt from file"
	if err := os.WriteFile(filePath, []byte(fileContent), 0o600); err != nil {
		t.Fatal(err)
	}

	stdin := strings.NewReader("should be ignored")

	result, err := readPrompt(stdin, filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != fileContent {
		t.Errorf("expected %q, got %q", fileContent, result)
	}
}

func TestMergeDisabledModels_DeselectedModel(t *testing.T) {
	t.Parallel()

	existing := []config.Model{
		{Name: "Model A", Model: "provider/model-a"},
		{Name: "Model B", Model: "provider/model-b"},
	}
	selected := []config.SelectedModel{
		{ID: "provider/model-a", Name: "Model A", Enabled: true},
	}

	result := mergeDisabledModels(existing, selected)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	// model-a stays enabled
	if result[0].ID != "provider/model-a" || !result[0].Enabled {
		t.Errorf("model-a should be enabled: %+v", result[0])
	}
	// model-b deselected -> disabled
	if result[1].ID != "provider/model-b" || result[1].Enabled {
		t.Errorf("model-b should be disabled: %+v", result[1])
	}
	if result[1].Name != "Model B" {
		t.Errorf("model-b should preserve name, got %q", result[1].Name)
	}
}

func TestMergeDisabledModels_NotInAPI(t *testing.T) {
	t.Parallel()

	existing := []config.Model{
		{Name: "Old Model", Model: "provider/old-model"},
	}
	// TUI didn't show old-model (not in API), so it's not in selected
	selected := []config.SelectedModel{
		{ID: "provider/new-model", Name: "New Model", Enabled: true},
	}

	result := mergeDisabledModels(existing, selected)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].ID != "provider/new-model" || !result[0].Enabled {
		t.Errorf("new-model should be enabled: %+v", result[0])
	}
	if result[1].ID != "provider/old-model" || result[1].Enabled {
		t.Errorf("old-model should be disabled: %+v", result[1])
	}
}

func TestMergeDisabledModels_SelectedNotDuplicated(t *testing.T) {
	t.Parallel()

	existing := []config.Model{
		{Name: "Model A", Model: "provider/model-a"},
	}
	selected := []config.SelectedModel{
		{ID: "provider/model-a", Name: "Model A", Enabled: true},
	}

	result := mergeDisabledModels(existing, selected)

	if len(result) != 1 {
		t.Fatalf("selected model should not be duplicated, got %d results", len(result))
	}
	if !result[0].Enabled {
		t.Error("model-a should stay enabled")
	}
}

func TestMergeDisabledModels_NoExisting(t *testing.T) {
	t.Parallel()

	selected := []config.SelectedModel{
		{ID: "provider/model-a", Name: "Model A", Enabled: true},
	}

	result := mergeDisabledModels(nil, selected)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].ID != "provider/model-a" || !result[0].Enabled {
		t.Errorf("unexpected result: %+v", result[0])
	}
}

//nolint:cyclop // integration test with multiple assertions
func TestIntegration_FullFlow(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("invalid auth header: %s", r.Header.Get("Authorization"))
		}

		var req client.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		cost := 0.001
		_ = json.NewEncoder(w).Encode(client.Response{
			ID: "gen-123",
			Choices: []client.Choice{{
				Message: client.ChoiceMessage{Content: "test response for " + req.Model},
			}},
			Usage: &client.Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
				Cost:             &cost,
			},
		})
	}))
	defer server.Close()

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "model-a", Model: "test/model-a", Enabled: true},
			{Name: "model-b", Model: "test/model-b", Enabled: true},
		},
	}

	cl := client.New("test-api-key", false, nil, client.WithBaseURL(server.URL))
	r := runner.New(cfg.EnabledModels(), cl, t.TempDir())

	output, err := r.Run(t.Context(), "You are a test assistant", "integration test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Successful != 2 {
		t.Errorf("expected 2 successful, got %d", output.Successful)
	}
	if output.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", output.Failed)
	}
	if len(output.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(output.Results))
	}

	for _, res := range output.Results {
		if res.Status != "success" {
			t.Errorf("result %s: expected success, got %s", res.Name, res.Status)
		}
		if !strings.Contains(res.Content, "test response") {
			t.Errorf("result %s: unexpected content %q", res.Name, res.Content)
		}
	}

	if output.TotalCost < 0.001 {
		t.Errorf("expected total cost >= 0.001, got %f", output.TotalCost)
	}
}
