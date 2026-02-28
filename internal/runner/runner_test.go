package runner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orx/internal/client"
	"orx/internal/config"
	"orx/internal/testutil"
)

func TestGenerateUUID(t *testing.T) {
	t.Parallel()

	id := generateUUID()

	pattern := `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	if !regexp.MustCompile(pattern).MatchString(id) {
		t.Errorf("UUID %q does not match expected format", id)
	}
}

func TestSaveOutput_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	result := &client.Result{
		Name:    "Claude Sonnet",
		Model:   "anthropic/claude-3.5-sonnet",
		Status:  "success",
		Content: "hello world",
	}

	path, err := saveOutput(dir, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// check filename pattern
	pattern := `orx-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.json$`
	if !regexp.MustCompile(pattern).MatchString(path) {
		t.Errorf("path %q does not match expected pattern", path)
	}

	// check permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600 permissions, got %o", info.Mode().Perm())
	}

	// check content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var saved savedOutput
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if saved.Name != "Claude Sonnet" {
		t.Errorf("name = %q, want %q", saved.Name, "Claude Sonnet")
	}
	if saved.Model != "anthropic/claude-3.5-sonnet" {
		t.Errorf("model = %q, want %q", saved.Model, "anthropic/claude-3.5-sonnet")
	}
	if saved.Content != "hello world" {
		t.Errorf("content = %q, want %q", saved.Content, "hello world")
	}
}

func TestSaveOutput_InvalidDir(t *testing.T) {
	t.Parallel()

	result := &client.Result{
		Name:    "test",
		Model:   "test/model",
		Status:  "success",
		Content: "test",
	}

	_, err := saveOutput("/nonexistent/dir/path", result)
	if err == nil {
		t.Fatal("expected error for invalid directory")
	}
}

func TestGenerateUUID_Unique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	for range 100 {
		id := generateUUID()
		if seen[id] {
			t.Fatalf("duplicate UUID: %s", id)
		}
		seen[id] = true
	}
}

func TestNew_Options(t *testing.T) {
	t.Parallel()

	cl := client.New("token", false, nil)
	models := []config.Model{{Name: "m", Model: "test/m", Enabled: true}}

	// default: no progress, default timeout
	r := New(models, cl, "/tmp")
	if r.progressOut != nil {
		t.Error("expected nil progressOut by default")
	}
	if r.saveDir != "/tmp" {
		t.Errorf("expected saveDir /tmp, got %q", r.saveDir)
	}
	if r.timeout != defaultTimeout {
		t.Errorf("expected default timeout %v, got %v", defaultTimeout, r.timeout)
	}

	// with options
	var buf strings.Builder
	r = New(models, cl, "/tmp", WithTimeout(5*time.Second), WithProgressOut(&buf))
	if r.timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", r.timeout)
	}
	if r.progressOut == nil {
		t.Error("expected non-nil progressOut")
	}
	if r.saveDir != "/tmp" {
		t.Errorf("expected /tmp saveDir, got %q", r.saveDir)
	}
}

func TestRun_Parallel(t *testing.T) {
	t.Parallel()

	var activeRequests atomic.Int32
	var maxConcurrent atomic.Int32

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		current := activeRequests.Add(1)
		defer activeRequests.Add(-1)

		for {
			old := maxConcurrent.Load()
			if current <= old || maxConcurrent.CompareAndSwap(old, current) {
				break
			}
		}

		time.Sleep(50 * time.Millisecond)

		_ = json.NewEncoder(w).Encode(client.Response{
			Choices: []client.Choice{{
				Message: client.ChoiceMessage{Content: "ok"},
			}},
		})
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
			{Name: "m2", Model: "test/m2", Enabled: true},
			{Name: "m3", Model: "test/m3", Enabled: true},
		},
	}

	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, t.TempDir())

	output, err := r.Run(context.Background(), "", "test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if maxConcurrent.Load() < 2 {
		t.Error("requests were not executed in parallel")
	}

	if output.Successful != 3 {
		t.Errorf("expected 3 successful, got %d", output.Successful)
	}
}

func TestRun_PartialFailure(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 2 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("error"))
			return
		}
		_ = json.NewEncoder(w).Encode(client.Response{
			Choices: []client.Choice{{Message: client.ChoiceMessage{Content: "ok"}}},
		})
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
			{Name: "m2", Model: "test/m2", Enabled: true},
		},
	}

	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, t.TempDir())

	output, err := r.Run(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Successful != 1 || output.Failed != 1 {
		t.Errorf("expected 1 success and 1 failure, got %d/%d", output.Successful, output.Failed)
	}
}

func TestRun_Timeout(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
		},
	}

	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, t.TempDir(), WithTimeout(100*time.Millisecond))

	output, err := r.Run(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Failed != 1 {
		t.Error("expected timeout failure")
	}
}

func TestRun_NoEnabledModels(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: false},
		},
	}

	cl := client.New("token", false, nil)
	r := New(cfg.EnabledModels(), cl, t.TempDir())

	_, err := r.Run(context.Background(), "", "test")
	if !errors.Is(err, config.ErrNoEnabledModels) {
		t.Errorf("expected ErrNoEnabledModels, got %v", err)
	}
}

func TestRun_AllFailed(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("error"))
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
			{Name: "m2", Model: "test/m2", Enabled: true},
		},
	}

	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, t.TempDir())

	output, err := r.Run(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Successful != 0 {
		t.Errorf("expected 0 successful, got %d", output.Successful)
	}
	if output.Failed != 2 {
		t.Errorf("expected 2 failed, got %d", output.Failed)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
		},
	}

	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	output, err := r.Run(ctx, "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Failed != 1 {
		t.Error("expected failure on context cancellation")
	}
	if !strings.Contains(output.Results[0].Error, "context canceled") {
		t.Errorf("expected context canceled in error, got %q", output.Results[0].Error)
	}
}

func TestRun_ResultsOrder(t *testing.T) {
	t.Parallel()

	delays := map[string]time.Duration{
		"m1": 100 * time.Millisecond,
		"m2": 10 * time.Millisecond,
		"m3": 50 * time.Millisecond,
	}

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req client.Request
		_ = json.NewDecoder(r.Body).Decode(&req)

		for name, delay := range delays {
			if strings.Contains(req.Model, name) {
				time.Sleep(delay)
				break
			}
		}

		_ = json.NewEncoder(w).Encode(client.Response{
			Choices: []client.Choice{{Message: client.ChoiceMessage{Content: req.Model}}},
		})
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
			{Name: "m2", Model: "test/m2", Enabled: true},
			{Name: "m3", Model: "test/m3", Enabled: true},
		},
	}

	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, t.TempDir())

	output, _ := r.Run(context.Background(), "", "test")

	// results should be in config order regardless of completion order
	if output.Results[0].Name != "m1" || output.Results[1].Name != "m2" || output.Results[2].Name != "m3" {
		t.Error("results not in config order")
	}
}

func TestRun_TotalCost(t *testing.T) {
	t.Parallel()

	var requestNum atomic.Int32
	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		num := requestNum.Add(1)
		var cost float64
		switch num {
		case 1:
			cost = 0.001
		case 2:
			cost = 0.002
		case 3:
			cost = 0.003
		}
		_ = json.NewEncoder(w).Encode(client.Response{
			ID: "gen",
			Choices: []client.Choice{{
				Message: client.ChoiceMessage{Content: "response"},
			}},
			Usage: &client.Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
				Cost:             &cost,
			},
		})
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
			{Name: "m2", Model: "test/m2", Enabled: true},
			{Name: "m3", Model: "test/m3", Enabled: true},
		},
	}

	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, t.TempDir())

	output, err := r.Run(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTotal := 0.006
	if output.TotalCost < expectedTotal-0.0001 || output.TotalCost > expectedTotal+0.0001 {
		t.Errorf("expected total cost ~%f, got %f", expectedTotal, output.TotalCost)
	}
}

func TestRun_TotalCostPartialFailure(t *testing.T) {
	t.Parallel()

	cost := 0.005
	var requestNum atomic.Int32
	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		num := requestNum.Add(1)
		if num == 2 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(client.Response{
			ID: "gen-ok",
			Choices: []client.Choice{{
				Message: client.ChoiceMessage{Content: "response"},
			}},
			Usage: &client.Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
				Cost:             &cost,
			},
		})
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
			{Name: "m2", Model: "test/m2", Enabled: true},
		},
	}

	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, t.TempDir())

	output, err := r.Run(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// only successful request should contribute to total cost
	if output.TotalCost < 0.004 || output.TotalCost > 0.006 {
		t.Errorf("expected total cost ~0.005, got %f", output.TotalCost)
	}
	if output.Successful != 1 || output.Failed != 1 {
		t.Errorf("expected 1 success and 1 failure, got %d/%d", output.Successful, output.Failed)
	}
}

func TestRun_ProgressOutput(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(client.Response{
			Choices: []client.Choice{{Message: client.ChoiceMessage{Content: "ok"}}},
		})
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "test-model", Model: "test/model", Enabled: true},
		},
	}

	var progressOut strings.Builder
	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, t.TempDir(), WithProgressOut(&progressOut))

	output, err := r.Run(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Successful != 1 {
		t.Fatalf("expected success, got %d failed", output.Failed)
	}

	progress := progressOut.String()
	if !strings.Contains(progress, "test-model - [requesting]") {
		t.Errorf("progress should contain requesting status, got: %q", progress)
	}
	if !strings.Contains(progress, "test-model - [done") {
		t.Errorf("progress should contain done status, got: %q", progress)
	}
}

func TestRun_SavesOutput(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(client.Response{
			Choices: []client.Choice{{Message: client.ChoiceMessage{Content: "test response"}}},
		})
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "test-model", Model: "test/model", Enabled: true},
		},
	}

	saveDir := t.TempDir()
	var progressOut strings.Builder
	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, saveDir, WithProgressOut(&progressOut))

	output, err := r.Run(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Successful != 1 {
		t.Fatalf("expected 1 successful, got %d", output.Successful)
	}

	// verify progress contains done + path in single line
	progress := progressOut.String()
	if !strings.Contains(progress, "[done (") || !strings.Contains(progress, ".json]") {
		t.Errorf("progress should contain done status with path, got: %q", progress)
	}

	// verify file exists
	files, _ := filepath.Glob(filepath.Join(saveDir, "orx-*.json"))
	if len(files) != 1 {
		t.Fatalf("expected 1 saved file, got %d", len(files))
	}

	// verify content
	data, _ := os.ReadFile(files[0])
	var saved savedOutput
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if saved.Name != "test-model" {
		t.Errorf("name = %q, want %q", saved.Name, "test-model")
	}
	if saved.Content != "test response" {
		t.Errorf("content = %q, want %q", saved.Content, "test response")
	}
}

func TestRun_SavesMultipleOutputs(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(client.Response{
			Choices: []client.Choice{{Message: client.ChoiceMessage{Content: "ok"}}},
		})
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
			{Name: "m2", Model: "test/m2", Enabled: true},
		},
	}

	saveDir := t.TempDir()
	var progressOut strings.Builder
	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, saveDir, WithProgressOut(&progressOut))

	output, err := r.Run(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Successful != 2 {
		t.Fatalf("expected 2 successful, got %d", output.Successful)
	}

	files, _ := filepath.Glob(filepath.Join(saveDir, "orx-*.json"))
	if len(files) != 2 {
		t.Fatalf("expected 2 saved files, got %d", len(files))
	}

	// verify 2 done+path notifications in stderr
	if strings.Count(progressOut.String(), ".json]") != 2 {
		t.Errorf("expected 2 save notifications, got: %q", progressOut.String())
	}
}

func TestRun_NoSaveOnError(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("error"))
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
		},
	}

	saveDir := t.TempDir()
	var progressOut strings.Builder
	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, saveDir, WithProgressOut(&progressOut))

	_, err := r.Run(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(saveDir, "orx-*.json"))
	if len(files) != 0 {
		t.Errorf("expected no saved files, got %d", len(files))
	}
	if strings.Contains(progressOut.String(), ".json]") {
		t.Errorf("should not contain save message, got: %q", progressOut.String())
	}
}

func TestRun_SaveErrorDoesNotFailRun(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(client.Response{
			Choices: []client.Choice{{Message: client.ChoiceMessage{Content: "ok"}}},
		})
	})

	cfg := &config.Config{
		Models: []config.Model{
			{Name: "m1", Model: "test/m1", Enabled: true},
		},
	}

	var progressOut strings.Builder
	cl := client.New("token", false, nil, client.WithBaseURL(server.URL))
	r := New(cfg.EnabledModels(), cl, "/nonexistent/dir/path", WithProgressOut(&progressOut))

	output, err := r.Run(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Successful != 1 {
		t.Fatalf("expected 1 successful, got %d", output.Successful)
	}

	if !strings.Contains(progressOut.String(), "save error") {
		t.Errorf("should contain save error warning, got: %q", progressOut.String())
	}
}
