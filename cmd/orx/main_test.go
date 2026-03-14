package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

func TestExtractPreSelected_EnabledModels(t *testing.T) {
	t.Parallel()

	models := []config.Model{
		{Model: "provider/model-a", Enabled: true},
		{Model: "provider/model-b", Enabled: false},
		{Model: "provider/model-c", Enabled: true},
	}

	result := extractPreSelected(models)

	if len(result) != 2 {
		t.Fatalf("expected 2 pre-selected, got %d", len(result))
	}
	if result[0] != "provider/model-a" {
		t.Errorf("expected model-a, got %q", result[0])
	}
	if result[1] != "provider/model-c" {
		t.Errorf("expected model-c, got %q", result[1])
	}
}

func TestExtractPreSelected_DisabledModelsExcluded(t *testing.T) {
	t.Parallel()

	models := []config.Model{
		{Model: "provider/model-a", Enabled: false},
		{Model: "provider/model-b", Enabled: false},
	}

	result := extractPreSelected(models)

	if len(result) != 0 {
		t.Errorf("expected no pre-selected models, got %d: %v", len(result), result)
	}
}

func TestExtractPreSelected_NilInput(t *testing.T) {
	t.Parallel()

	result := extractPreSelected(nil)

	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
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
	byID := make(map[string]config.SelectedModel, len(result))
	for _, m := range result {
		byID[m.ID] = m
	}
	if a, ok := byID["provider/model-a"]; !ok || !a.Enabled {
		t.Errorf("model-a should be enabled: %+v", a)
	}
	b, ok := byID["provider/model-b"]
	if !ok || b.Enabled {
		t.Errorf("model-b should be disabled: %+v", b)
	}
	if b.Name != "Model B" {
		t.Errorf("model-b should preserve name, got %q", b.Name)
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
	byID := make(map[string]config.SelectedModel, len(result))
	for _, m := range result {
		byID[m.ID] = m
	}
	if nm, ok := byID["provider/new-model"]; !ok || !nm.Enabled {
		t.Errorf("new-model should be enabled: %+v", nm)
	}
	if om, ok := byID["provider/old-model"]; !ok || om.Enabled {
		t.Errorf("old-model should be disabled: %+v", om)
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

func TestMergeDisabledModels_PreservesExistingParams(t *testing.T) {
	t.Parallel()

	temp := float64(0.7)
	existing := []config.Model{
		{Name: "Model A", Model: "provider/model-a", Temperature: &temp},
	}
	selected := []config.SelectedModel{}

	result := mergeDisabledModels(existing, selected)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].ExistingParams == nil {
		t.Fatal("expected ExistingParams to be set for disabled model")
	}
	if result[0].ExistingParams.Temperature == nil || *result[0].ExistingParams.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", result[0].ExistingParams.Temperature)
	}
}

func TestMergeDisabledModels_PreservesReasoningInExistingParams(t *testing.T) {
	t.Parallel()

	existing := []config.Model{
		{Name: "Model R", Model: "provider/model-r", Reasoning: &config.ReasoningConfig{Effort: "high"}},
	}
	selected := []config.SelectedModel{}

	result := mergeDisabledModels(existing, selected)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].ExistingParams == nil {
		t.Fatal("expected ExistingParams to be set")
	}
	if result[0].ExistingParams.Reasoning == nil || result[0].ExistingParams.Reasoning.Effort != "high" {
		t.Errorf("expected reasoning effort 'high', got %+v", result[0].ExistingParams.Reasoning)
	}
}

func TestMergeDisabledModels_EnabledModelGetsExistingParams(t *testing.T) {
	t.Parallel()

	temp := float64(0.5)
	existing := []config.Model{
		{Name: "Model A", Model: "provider/model-a", Temperature: &temp},
	}
	selected := []config.SelectedModel{
		{ID: "provider/model-a", Name: "Model A", Enabled: true},
	}

	result := mergeDisabledModels(existing, selected)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].ExistingParams == nil {
		t.Fatal("expected ExistingParams to be set for enabled model with existing config")
	}
	if result[0].ExistingParams.Temperature == nil || *result[0].ExistingParams.Temperature != 0.5 {
		t.Errorf("expected temperature 0.5, got %v", result[0].ExistingParams.Temperature)
	}
}

func TestMergeDisabledModels_NewModelHasNilExistingParams(t *testing.T) {
	t.Parallel()

	existing := []config.Model{
		{Name: "Model A", Model: "provider/model-a"},
	}
	selected := []config.SelectedModel{
		{ID: "provider/model-new", Name: "New Model", Enabled: true},
	}

	result := mergeDisabledModels(existing, selected)

	// model-new was not in existing config
	byID := make(map[string]config.SelectedModel, len(result))
	for _, m := range result {
		byID[m.ID] = m
	}
	if nm, ok := byID["provider/model-new"]; !ok {
		t.Fatal("expected model-new in result")
	} else if nm.ExistingParams != nil {
		t.Errorf("expected nil ExistingParams for new model, got %+v", nm.ExistingParams)
	}
}

func TestMergeDisabledModels_EmptySelected(t *testing.T) {
	t.Parallel()

	existing := []config.Model{
		{Name: "Model A", Model: "provider/model-a"},
		{Name: "Model B", Model: "provider/model-b"},
	}

	result := mergeDisabledModels(existing, []config.SelectedModel{})

	if len(result) != 2 {
		t.Fatalf("expected 2 disabled results, got %d", len(result))
	}
	for _, m := range result {
		if m.Enabled {
			t.Errorf("expected model %s to be disabled", m.ID)
		}
	}
}

//nolint:cyclop // integration test with multiple assertions
func newKeyInfoJSON(label string, usage, daily, weekly, monthly float64, limit, remaining *float64) string {
	limitStr := "null"
	remainingStr := "null"
	if limit != nil {
		limitStr = fmt.Sprintf("%.2f", *limit)
	}
	if remaining != nil {
		remainingStr = fmt.Sprintf("%.2f", *remaining)
	}
	return fmt.Sprintf(`{"data":{"label":%q,"usage":%.2f,"usage_daily":%.2f,"usage_weekly":%.2f,"usage_monthly":%.2f,"limit":%s,"limit_remaining":%s,"is_free_tier":false}}`,
		label, usage, daily, weekly, monthly, limitStr, remainingStr)
}

func TestUsageCmd_MissingToken(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	cmd := newRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{"usage"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !errors.Is(err, ErrTokenRequired) {
		t.Errorf("expected ErrTokenRequired, got: %v", err)
	}
}

func TestUsageCmd_Success(t *testing.T) {
	t.Parallel()

	usage := 2.5
	daily := 0.3
	weekly := 1.2
	monthly := 2.5
	limit := 10.0
	remaining := 7.5
	body := newKeyInfoJSON("my-key", usage, daily, weekly, monthly, &limit, &remaining)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/key" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	cmd := newRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{"usage", "--token", "test-token", "--base-url", server.URL})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "my-key") {
		t.Errorf("output should contain label 'my-key', got:\n%s", out)
	}
	if !strings.Contains(out, "$2.50") {
		t.Errorf("output should contain total usage $2.50, got:\n%s", out)
	}
	if !strings.Contains(out, "$0.30") {
		t.Errorf("output should contain daily $0.30, got:\n%s", out)
	}
	if !strings.Contains(out, "$1.20") {
		t.Errorf("output should contain weekly $1.20, got:\n%s", out)
	}
}

func TestUsageCmd_APIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"usage", "--token", "bad-token", "--base-url", server.URL})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for API error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestUsageCmd_WithLimit(t *testing.T) {
	t.Parallel()

	limit := 10.0
	remaining := 7.5
	body := newKeyInfoJSON("key-with-limit", 2.5, 0.3, 1.2, 2.5, &limit, &remaining)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	cmd := newRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{"usage", "--token", "test-token", "--base-url", server.URL})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Limit") {
		t.Errorf("output should contain 'Limit', got:\n%s", out)
	}
	if !strings.Contains(out, "$10.00") {
		t.Errorf("output should contain '$10.00', got:\n%s", out)
	}
	if !strings.Contains(out, "Remaining") {
		t.Errorf("output should contain 'Remaining', got:\n%s", out)
	}
	if !strings.Contains(out, "$7.50") {
		t.Errorf("output should contain '$7.50', got:\n%s", out)
	}
}

func TestUsageCmd_NoLimit(t *testing.T) {
	t.Parallel()

	body := newKeyInfoJSON("free-key", 0.0, 0.0, 0.0, 0.0, nil, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	cmd := newRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{"usage", "--token", "test-token", "--base-url", server.URL})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if strings.Contains(out, "Limit:") {
		t.Errorf("output should not contain 'Limit:' when no limit set, got:\n%s", out)
	}
	if strings.Contains(out, "Remaining:") {
		t.Errorf("output should not contain 'Remaining:' when no limit set, got:\n%s", out)
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
