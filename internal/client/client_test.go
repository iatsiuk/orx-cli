package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

			var attempts atomic.Int32
			server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				if attempts.Load() < 2 {
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
			if attempts.Load() != 2 {
				t.Errorf("expected 2 attempts, got %d", attempts.Load())
			}
		})
	}
}

func TestExecute_NoRetry4xx(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	})

	c := New("token", false, nil, WithBaseURL(server.URL))
	result := c.Execute(context.Background(), &config.Model{Name: "t", Model: "m"}, "", "prompt")

	if result.Status != "error" {
		t.Error("expected error status")
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts.Load())
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

//nolint:cyclop,funlen // table-driven test with multiple cases
func TestBuildRequest_OptionalParams(t *testing.T) {
	t.Parallel()

	enabled := true
	maxCompTokens := 4096

	tests := []struct {
		name  string
		model *config.Model
		check func(*testing.T, *Request)
	}{
		{
			name: "reasoning enabled",
			model: &config.Model{
				Name:  "test",
				Model: "test/model",
				Reasoning: &config.ReasoningConfig{
					Enabled: &enabled,
					Effort:  "high",
				},
			},
			check: func(t *testing.T, req *Request) {
				if req.Reasoning == nil {
					t.Fatal("reasoning is nil")
				}
				if req.Reasoning.Enabled == nil || !*req.Reasoning.Enabled {
					t.Error("reasoning.enabled not set")
				}
				if req.Reasoning.Effort != "high" {
					t.Errorf("expected effort 'high', got %q", req.Reasoning.Effort)
				}
			},
		},
		{
			name: "reasoning summary",
			model: &config.Model{
				Name:  "test",
				Model: "test/model",
				Reasoning: &config.ReasoningConfig{
					Effort:  "high",
					Summary: "concise",
				},
			},
			check: func(t *testing.T, req *Request) {
				if req.Reasoning == nil {
					t.Fatal("reasoning is nil")
				}
				if req.Reasoning.Summary != "concise" {
					t.Errorf("expected summary 'concise', got %q", req.Reasoning.Summary)
				}
			},
		},
		{
			name: "max completion tokens",
			model: &config.Model{
				Name:                "test",
				Model:               "test/model",
				MaxCompletionTokens: &maxCompTokens,
			},
			check: func(t *testing.T, req *Request) {
				if req.MaxCompletionTokens == nil || *req.MaxCompletionTokens != 4096 {
					t.Errorf("expected max_completion_tokens 4096, got %v", req.MaxCompletionTokens)
				}
			},
		},
		{
			name: "stop string",
			model: &config.Model{
				Name:  "test",
				Model: "test/model",
				Stop:  "STOP",
			},
			check: func(t *testing.T, req *Request) {
				if req.Stop != "STOP" {
					t.Errorf("expected stop string 'STOP', got %v", req.Stop)
				}
			},
		},
		{
			name: "stop array",
			model: &config.Model{
				Name:  "test",
				Model: "test/model",
				Stop:  []string{"STOP1", "STOP2"},
			},
			check: func(t *testing.T, req *Request) {
				stopArr, ok := req.Stop.([]string)
				if !ok {
					t.Fatalf("expected stop to be []string, got %T", req.Stop)
				}
				if len(stopArr) != 2 || stopArr[0] != "STOP1" || stopArr[1] != "STOP2" {
					t.Errorf("unexpected stop array: %v", stopArr)
				}
			},
		},
	}

	c := New("token", false, nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := c.buildRequest(tt.model, "", "prompt")
			tt.check(t, req)
		})
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

//nolint:funlen // table-driven test with multiple cases
func TestExecute_RequestSerialization(t *testing.T) {
	t.Parallel()

	enabled := true
	maxCompTokens := 8192

	tests := []struct {
		name  string
		model *config.Model
		check func(*testing.T, *Request)
	}{
		{
			name: "reasoning enabled and effort",
			model: &config.Model{
				Name:  "test",
				Model: "openai/gpt-5.2",
				Reasoning: &config.ReasoningConfig{
					Enabled: &enabled,
					Effort:  "high",
				},
			},
			check: func(t *testing.T, req *Request) {
				if req.Reasoning == nil {
					t.Fatal("reasoning not sent in request")
				}
				if req.Reasoning.Enabled == nil || !*req.Reasoning.Enabled {
					t.Error("reasoning.enabled not sent")
				}
				if req.Reasoning.Effort != "high" {
					t.Errorf("expected effort 'high', got %q", req.Reasoning.Effort)
				}
			},
		},
		{
			name: "reasoning summary",
			model: &config.Model{
				Name:  "test",
				Model: "openai/gpt-5.2",
				Reasoning: &config.ReasoningConfig{
					Effort:  "high",
					Summary: "detailed",
				},
			},
			check: func(t *testing.T, req *Request) {
				if req.Reasoning == nil {
					t.Fatal("reasoning not sent in request")
				}
				if req.Reasoning.Summary != "detailed" {
					t.Errorf("expected summary 'detailed', got %q", req.Reasoning.Summary)
				}
			},
		},
		{
			name: "max completion tokens",
			model: &config.Model{
				Name:                "test",
				Model:               "openai/gpt-5.2",
				MaxCompletionTokens: &maxCompTokens,
			},
			check: func(t *testing.T, req *Request) {
				if req.MaxCompletionTokens == nil {
					t.Fatal("max_completion_tokens not sent in request")
				}
				if *req.MaxCompletionTokens != 8192 {
					t.Errorf("expected max_completion_tokens 8192, got %d", *req.MaxCompletionTokens)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var receivedRequest Request
			server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedRequest)
				_ = json.NewEncoder(w).Encode(Response{
					Choices: []Choice{{Message: ChoiceMessage{Content: "ok"}}},
				})
			})

			c := New("token", false, nil, WithBaseURL(server.URL))
			c.Execute(context.Background(), tt.model, "", "prompt")
			tt.check(t, &receivedRequest)
		})
	}
}

//nolint:funlen // table-driven test with many cases
func TestIsRetryableAPIError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		apiErr   *APIError
		expected bool
	}{
		// numeric code (float64 from JSON)
		{
			name:     "code 500",
			apiErr:   &APIError{Code: float64(500), Message: "server down"},
			expected: true,
		},
		{
			name:     "code 502",
			apiErr:   &APIError{Code: float64(502), Message: "bad gateway"},
			expected: true,
		},
		{
			name:     "code 503",
			apiErr:   &APIError{Code: float64(503), Message: "unavailable"},
			expected: true,
		},
		{
			name:     "code 429",
			apiErr:   &APIError{Code: float64(429), Message: "rate limited"},
			expected: true,
		},
		{
			name:     "code 400 not retryable",
			apiErr:   &APIError{Code: float64(400), Message: "bad request"},
			expected: false,
		},
		{
			name:     "code 401 not retryable",
			apiErr:   &APIError{Code: float64(401), Message: "unauthorized"},
			expected: false,
		},
		{
			name:     "code 404 not retryable",
			apiErr:   &APIError{Code: float64(404), Message: "not found"},
			expected: false,
		},
		// string code
		{
			name:     "string code rate_limit",
			apiErr:   &APIError{Code: "rate_limit", Message: "slow down"},
			expected: true,
		},
		{
			name:     "string code timeout",
			apiErr:   &APIError{Code: "timeout", Message: "timed out"},
			expected: true,
		},
		{
			name:     "string code overloaded",
			apiErr:   &APIError{Code: "overloaded", Message: "busy"},
			expected: true,
		},
		{
			name:     "string code server_error",
			apiErr:   &APIError{Code: "server_error", Message: "failed"},
			expected: true,
		},
		{
			name:     "string code invalid_request not retryable",
			apiErr:   &APIError{Code: "invalid_request", Message: "bad"},
			expected: false,
		},
		// message fallback (no code)
		{
			name:     "message internal server error",
			apiErr:   &APIError{Message: "Internal Server Error"},
			expected: true,
		},
		{
			name:     "message bad gateway",
			apiErr:   &APIError{Message: "Bad Gateway"},
			expected: true,
		},
		{
			name:     "message service unavailable",
			apiErr:   &APIError{Message: "Service Unavailable"},
			expected: true,
		},
		{
			name:     "message gateway timeout",
			apiErr:   &APIError{Message: "Gateway Timeout"},
			expected: true,
		},
		{
			name:     "message overloaded",
			apiErr:   &APIError{Message: "Model is currently overloaded"},
			expected: true,
		},
		{
			name:     "message too many requests",
			apiErr:   &APIError{Message: "Too Many Requests"},
			expected: true,
		},
		{
			name:     "message rate limit",
			apiErr:   &APIError{Message: "Rate limit exceeded"},
			expected: true,
		},
		{
			name:     "message invalid model not retryable",
			apiErr:   &APIError{Message: "invalid model"},
			expected: false,
		},
		{
			name:     "message bad request not retryable",
			apiErr:   &APIError{Message: "bad request: missing field"},
			expected: false,
		},
		// nil code with retryable message
		{
			name:     "nil code with retryable message",
			apiErr:   &APIError{Code: nil, Message: "Internal Server Error"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isRetryableAPIError(tt.apiErr); got != tt.expected {
				t.Errorf("isRetryableAPIError(%+v) = %v, want %v", tt.apiErr, got, tt.expected)
			}
		})
	}
}

func TestExecute_RetryOnAPIError200(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		apiError  APIError
		wantRetry bool
	}{
		{
			name:      "internal server error retries",
			apiError:  APIError{Message: "Internal Server Error"},
			wantRetry: true,
		},
		{
			name:      "numeric code 502 retries",
			apiError:  APIError{Code: float64(502), Message: "upstream failed"},
			wantRetry: true,
		},
		{
			name:      "overloaded retries",
			apiError:  APIError{Message: "Model is currently overloaded"},
			wantRetry: true,
		},
		{
			name:      "invalid model does not retry",
			apiError:  APIError{Message: "invalid model"},
			wantRetry: false,
		},
		{
			name:      "numeric code 400 does not retry",
			apiError:  APIError{Code: float64(400), Message: "bad request"},
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32
			server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				if attempts.Load() < 2 {
					_ = json.NewEncoder(w).Encode(Response{Error: &tt.apiError})
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

			wantStatus := "error"
			wantAttempts := int32(1)
			if tt.wantRetry {
				wantStatus = "success"
				wantAttempts = 2
			}

			if result.Status != wantStatus {
				t.Errorf("expected status %s, got %s: %s", wantStatus, result.Status, result.Error)
			}
			if attempts.Load() != wantAttempts {
				t.Errorf("expected %d attempts, got %d", wantAttempts, attempts.Load())
			}
		})
	}
}

func TestKeyInfo_Success(t *testing.T) {
	t.Parallel()

	limit := 10.0
	limitRemaining := 7.5
	limitReset := "2026-04-01T00:00:00Z"

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/key" {
			t.Errorf("expected path /key, got %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or invalid authorization header")
		}
		_ = json.NewEncoder(w).Encode(KeyInfoResponse{
			Data: KeyInfoData{
				Label:          "my-key",
				Limit:          &limit,
				LimitReset:     &limitReset,
				LimitRemaining: &limitRemaining,
				Usage:          2.5,
				UsageDaily:     0.3,
				UsageWeekly:    1.2,
				UsageMonthly:   2.5,
				IsFreeTier:     false,
			},
		})
	})

	c := New("test-token", false, nil, WithBaseURL(server.URL))
	resp, err := c.KeyInfo(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data.Label != "my-key" {
		t.Errorf("expected label 'my-key', got %q", resp.Data.Label)
	}
	if resp.Data.Limit == nil || *resp.Data.Limit != 10.0 {
		t.Errorf("expected limit 10.0, got %v", resp.Data.Limit)
	}
	if resp.Data.LimitRemaining == nil || *resp.Data.LimitRemaining != 7.5 {
		t.Errorf("expected limit_remaining 7.5, got %v", resp.Data.LimitRemaining)
	}
	if resp.Data.LimitReset == nil || *resp.Data.LimitReset != "2026-04-01T00:00:00Z" {
		t.Errorf("unexpected limit_reset: %v", resp.Data.LimitReset)
	}
	if resp.Data.Usage != 2.5 {
		t.Errorf("expected usage 2.5, got %v", resp.Data.Usage)
	}
	if resp.Data.UsageDaily != 0.3 {
		t.Errorf("expected usage_daily 0.3, got %v", resp.Data.UsageDaily)
	}
	if resp.Data.UsageWeekly != 1.2 {
		t.Errorf("expected usage_weekly 1.2, got %v", resp.Data.UsageWeekly)
	}
	if resp.Data.UsageMonthly != 2.5 {
		t.Errorf("expected usage_monthly 2.5, got %v", resp.Data.UsageMonthly)
	}
	if resp.Data.IsFreeTier {
		t.Error("expected is_free_tier false")
	}
}

func TestKeyInfo_Unauthorized(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	})

	c := New("bad-token", false, nil, WithBaseURL(server.URL))
	resp, err := c.KeyInfo(context.Background())

	if err == nil {
		t.Fatal("expected error for 401")
	}
	if resp != nil {
		t.Error("expected nil response on error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got %q", err.Error())
	}
}

func TestKeyInfo_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json {{{"))
	})

	c := New("token", false, nil, WithBaseURL(server.URL))
	resp, err := c.KeyInfo(context.Background())

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if resp != nil {
		t.Error("expected nil response on error")
	}
}

func TestKeyInfo_Verbose(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(KeyInfoResponse{
			Data: KeyInfoData{Label: "test-key"},
		})
	})

	var out strings.Builder
	c := New("token", true, &out, WithBaseURL(server.URL))
	_, err := c.KeyInfo(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "=== REQUEST [key] ===") {
		t.Errorf("missing request dump in verbose output, got: %q", output)
	}
	if !strings.Contains(output, "=== RESPONSE [key] ===") {
		t.Errorf("missing response dump in verbose output, got: %q", output)
	}
}

func TestKeyInfo_ContextCancellation(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})

	c := New("token", false, nil, WithBaseURL(server.URL))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	resp, err := c.KeyInfo(ctx)

	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
	if resp != nil {
		t.Error("expected nil response on error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got %q", err.Error())
	}
}

func TestIsRetryable_StreamError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "http2 stream error",
			err:      fmt.Errorf("read response: stream error: stream ID 1; INTERNAL_ERROR; received from peer"),
			expected: true,
		},
		{
			name:     "http2 stream error uppercase",
			err:      fmt.Errorf("Stream Error: INTERNAL_ERROR"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      fmt.Errorf("read tcp: connection reset by peer"),
			expected: true,
		},
		{
			name:     "rst_stream",
			err:      fmt.Errorf("http2: RST_STREAM received"),
			expected: true,
		},
		{
			name:     "goaway",
			err:      fmt.Errorf("http2: GOAWAY received"),
			expected: true,
		},
		{
			name:     "unexpected EOF",
			err:      io.ErrUnexpectedEOF,
			expected: true,
		},
		{
			name:     "wrapped unexpected EOF",
			err:      fmt.Errorf("read response: %w", io.ErrUnexpectedEOF),
			expected: true,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "wrapped context canceled",
			err:      fmt.Errorf("request failed: %w", context.Canceled),
			expected: false,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("some unrelated error"),
			expected: false,
		},
		{
			name:     "retryable error",
			err:      &retryableError{statusCode: 500, body: "server error"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isRetryable(tt.err); got != tt.expected {
				t.Errorf("isRetryable(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}
