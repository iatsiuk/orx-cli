package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orx/internal/config"
	"orx/internal/testutil"
)

func TestExecute_Success(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or invalid authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing content-type header")
		}

		_ = json.NewEncoder(w).Encode(Response{
			ID: "test-id",
			Choices: []Choice{{
				Message: ChoiceMessage{Content: "test response"},
			}},
			Usage: &Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		})
	})

	c := New("test-token", false, nil, WithBaseURL(server.URL))

	model := &config.Model{
		Name:  "test",
		Model: "test/model",
	}

	result := c.Execute(context.Background(), model, "system", "user prompt")

	if result.Status != "success" {
		t.Errorf("expected success, got %s: %s", result.Status, result.Error)
	}
	if result.Content != "test response" {
		t.Errorf("unexpected content: %s", result.Content)
	}
	if result.Usage == nil || result.Usage.TotalTokens != 30 {
		t.Error("usage not parsed correctly")
	}
}

func TestExecute_WithReasoning(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{
			Choices: []Choice{{
				Message: ChoiceMessage{
					Content:   "answer",
					Reasoning: "thinking process",
				},
			}},
		})
	})

	c := New("token", false, nil, WithBaseURL(server.URL))
	result := c.Execute(context.Background(), &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Reasoning != "thinking process" {
		t.Errorf("expected reasoning, got %q", result.Reasoning)
	}
}

func TestExecute_ReasoningFromDetails(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{
			Choices: []Choice{{
				Message: ChoiceMessage{Content: "answer"},
				ReasoningDetails: []ReasoningDetail{
					{Type: "thinking", Content: "detailed thinking"},
				},
			}},
		})
	})

	c := New("token", false, nil, WithBaseURL(server.URL))
	result := c.Execute(context.Background(), &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Reasoning != "detailed thinking" {
		t.Errorf("expected reasoning from details, got %q", result.Reasoning)
	}
}

func TestExecute_Retry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
	}{
		{"429 rate limit", http.StatusTooManyRequests},
		{"502 bad gateway", http.StatusBadGateway},
		{"503 service unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			attempts := 0
			server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				attempts++
				if attempts < 2 {
					w.WriteHeader(tt.statusCode)
					return
				}
				_ = json.NewEncoder(w).Encode(Response{
					Choices: []Choice{{Message: ChoiceMessage{Content: "ok"}}},
				})
			})

			c := New("token", false, nil, WithBaseURL(server.URL))

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			result := c.Execute(ctx, &config.Model{Name: "t", Model: "m"}, "", "prompt")

			if result.Status != "success" {
				t.Errorf("expected success after retry, got %s: %s", result.Status, result.Error)
			}
			if attempts != 2 {
				t.Errorf("expected 2 attempts, got %d", attempts)
			}
		})
	}
}

func TestExecute_NoRetry4xx(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	})

	c := New("token", false, nil, WithBaseURL(server.URL))
	result := c.Execute(context.Background(), &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "error" {
		t.Error("expected error status")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
}

func TestExecute_Timeout(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	})

	c := New("token", false, nil, WithBaseURL(server.URL))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := c.Execute(ctx, &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "error" {
		t.Error("expected error status on timeout")
	}
}

func TestExecute_Verbose(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{
			Choices: []Choice{{Message: ChoiceMessage{Content: "ok"}}},
		})
	})

	var out strings.Builder
	c := New("token", true, &out, WithBaseURL(server.URL))

	c.Execute(context.Background(), &config.Model{Name: "test-model", Model: "m"}, "", "prompt")

	output := out.String()
	if !strings.Contains(output, "=== REQUEST [test-model] ===") {
		t.Error("missing request dump in verbose output")
	}
	if !strings.Contains(output, "=== RESPONSE [test-model] ===") {
		t.Error("missing response dump in verbose output")
	}
}

func TestExecute_APIError(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{
			Error: &APIError{Message: "invalid model"},
		})
	})

	c := New("token", false, nil, WithBaseURL(server.URL))
	result := c.Execute(context.Background(), &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "error" {
		t.Error("expected error status")
	}
	if !strings.Contains(result.Error, "invalid model") {
		t.Errorf("expected error message, got %q", result.Error)
	}
}

func TestExecute_RetryExhaustion(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	})

	c := New("token", false, nil, WithBaseURL(server.URL))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := c.Execute(ctx, &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "error" {
		t.Error("expected error status after retry exhaustion")
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestExecute_EmptyChoices(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{
			ID:      "test",
			Choices: []Choice{},
		})
	})

	c := New("token", false, nil, WithBaseURL(server.URL))
	result := c.Execute(context.Background(), &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "error" {
		t.Error("expected error for empty choices")
	}
	if !strings.Contains(result.Error, "no choices") {
		t.Errorf("expected 'no choices' error, got %q", result.Error)
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})

	c := New("token", false, nil, WithBaseURL(server.URL))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result := c.Execute(ctx, &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "error" {
		t.Error("expected error on context cancellation")
	}
	if !strings.Contains(result.Error, "context canceled") {
		t.Errorf("expected context canceled error, got %q", result.Error)
	}
}

func TestExecute_NetworkErrorRetry(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		// close connection immediately to simulate network error
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server doesn't support hijacking")
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	})

	c := New("token", false, nil, WithBaseURL(server.URL))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := c.Execute(ctx, &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "error" {
		t.Error("expected error status on network failure")
	}
	if attempts.Load() < 2 {
		t.Errorf("expected retry on network error, got %d attempts", attempts.Load())
	}
}

func TestExecute_InvalidJSONResponse(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json {{{"))
	})

	c := New("token", false, nil, WithBaseURL(server.URL))
	result := c.Execute(context.Background(), &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "error" {
		t.Error("expected error for invalid JSON response")
	}
	if !strings.Contains(result.Error, "unmarshal") {
		t.Errorf("expected unmarshal error, got %q", result.Error)
	}
}

func TestBuildRequest(t *testing.T) {
	t.Parallel()

	temp := 0.5
	maxTokens := 100
	includeReasoning := true

	model := &config.Model{
		Name:             "test",
		Model:            "test/model",
		Temperature:      &temp,
		MaxTokens:        &maxTokens,
		IncludeReasoning: &includeReasoning,
		Reasoning: &config.ReasoningConfig{
			Effort: "high",
		},
		Provider: &config.ProviderConfig{
			Order:          []string{"OpenAI"},
			DataCollection: "deny",
		},
	}

	c := New("token", false, nil)
	req := c.buildRequest(model, "system prompt", "user prompt")

	if req.Model != "test/model" {
		t.Errorf("unexpected model: %s", req.Model)
	}

	if len(req.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(req.Messages))
	}

	if req.Messages[0].Role != "system" || req.Messages[0].Content != "system prompt" {
		t.Error("system message not set correctly")
	}

	if *req.Temperature != 0.5 {
		t.Error("temperature not set")
	}

	if req.Reasoning == nil || req.Reasoning.Effort != "high" {
		t.Error("reasoning not set")
	}

	if req.Provider == nil || req.Provider.DataCollection != "deny" {
		t.Error("provider not set")
	}
}

func TestBuildRequest_StopString(t *testing.T) {
	t.Parallel()

	model := &config.Model{
		Name:  "test",
		Model: "test/model",
		Stop:  "STOP",
	}

	c := New("token", false, nil)
	req := c.buildRequest(model, "", "prompt")

	if req.Stop != "STOP" {
		t.Errorf("expected stop string 'STOP', got %v", req.Stop)
	}
}

func TestBuildRequest_StopArray(t *testing.T) {
	t.Parallel()

	model := &config.Model{
		Name:  "test",
		Model: "test/model",
		Stop:  []string{"STOP1", "STOP2"},
	}

	c := New("token", false, nil)
	req := c.buildRequest(model, "", "prompt")

	stopArr, ok := req.Stop.([]string)
	if !ok {
		t.Fatalf("expected stop to be []string, got %T", req.Stop)
	}
	if len(stopArr) != 2 || stopArr[0] != "STOP1" || stopArr[1] != "STOP2" {
		t.Errorf("unexpected stop array: %v", stopArr)
	}
}

func TestExecute_WithCost(t *testing.T) {
	t.Parallel()

	cost := 0.00125
	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{
			ID: "gen-12345",
			Choices: []Choice{{
				Message: ChoiceMessage{Content: "response"},
			}},
			Usage: &Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
				Cost:             &cost,
			},
		})
	})

	c := New("token", false, nil, WithBaseURL(server.URL))
	result := c.Execute(context.Background(), &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "success" {
		t.Fatalf("expected success, got %s: %s", result.Status, result.Error)
	}
	if result.Cost == nil || *result.Cost != 0.00125 {
		t.Errorf("expected cost 0.00125, got %v", result.Cost)
	}
	if result.Usage == nil {
		t.Fatal("expected usage to be set")
	}
	if result.Usage.TotalTokens != 150 {
		t.Errorf("expected 150 total tokens, got %d", result.Usage.TotalTokens)
	}
}

func TestExecute_NoCostInResponse(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{
			ID: "gen-12345",
			Choices: []Choice{{
				Message: ChoiceMessage{Content: "response"},
			}},
			Usage: &Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		})
	})

	c := New("token", false, nil, WithBaseURL(server.URL))
	result := c.Execute(context.Background(), &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "success" {
		t.Errorf("expected success, got %s", result.Status)
	}
	if result.Cost != nil {
		t.Errorf("expected no cost, got %v", result.Cost)
	}
	if result.Usage == nil || result.Usage.TotalTokens != 15 {
		t.Error("usage should be populated")
	}
}
