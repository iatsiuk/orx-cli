package runner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orx/internal/client"
	"orx/internal/config"
	"orx/internal/testutil"
)

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
	r := New(cfg, cl, 30*time.Second, nil)

	output, err := r.Run(context.Background(), "test prompt")
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
	r := New(cfg, cl, 30*time.Second, nil)

	output, err := r.Run(context.Background(), "test")
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
	r := New(cfg, cl, 100*time.Millisecond, nil)

	output, err := r.Run(context.Background(), "test")
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
	r := New(cfg, cl, 30*time.Second, nil)

	_, err := r.Run(context.Background(), "test")
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
	r := New(cfg, cl, 30*time.Second, nil)

	output, err := r.Run(context.Background(), "test")
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
	r := New(cfg, cl, 30*time.Second, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	output, err := r.Run(ctx, "test")
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
	r := New(cfg, cl, 30*time.Second, nil)

	output, _ := r.Run(context.Background(), "test")

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
	r := New(cfg, cl, 30*time.Second, nil)

	output, err := r.Run(context.Background(), "test")
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
	r := New(cfg, cl, 30*time.Second, nil)

	output, err := r.Run(context.Background(), "test")
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
	r := New(cfg, cl, 30*time.Second, &progressOut)

	output, err := r.Run(context.Background(), "test")
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
