package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"orx/internal/config"
)

// retryable error patterns for http/2 transport failures
var retryablePatterns = []string{
	"stream error",
	"internal_error",
	"connection reset",
	"rst_stream",
	"goaway",
}

// retryable API error message patterns (upstream provider failures proxied via HTTP 200)
var retryableAPIPatterns = []string{
	"internal server error",
	"bad gateway",
	"service unavailable",
	"gateway timeout",
	"overloaded",
	"too many requests",
	"rate limit",
}

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	maxRetries     = 3
	retryDelay     = 5 * time.Second
)

type Client struct {
	httpClient *http.Client
	token      string
	verbose    bool
	output     io.Writer
	baseURL    string
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

func New(token string, verbose bool, output io.Writer, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{},
		token:      token,
		verbose:    verbose,
		output:     output,
		baseURL:    defaultBaseURL,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type Request struct {
	Model               string     `json:"model"`
	Messages            []Message  `json:"messages"`
	Temperature         *float64   `json:"temperature,omitempty"`
	TopP                *float64   `json:"top_p,omitempty"`
	TopK                *int       `json:"top_k,omitempty"`
	FrequencyPenalty    *float64   `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float64   `json:"presence_penalty,omitempty"`
	RepetitionPenalty   *float64   `json:"repetition_penalty,omitempty"`
	MinP                *float64   `json:"min_p,omitempty"`
	TopA                *float64   `json:"top_a,omitempty"`
	Seed                *int       `json:"seed,omitempty"`
	MaxTokens           *int       `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int       `json:"max_completion_tokens,omitempty"`
	Stop                any        `json:"stop,omitempty"`
	Reasoning           *Reasoning `json:"reasoning,omitempty"`
	IncludeReasoning    *bool      `json:"include_reasoning,omitempty"`
	Provider            *Provider  `json:"provider,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Reasoning struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Effort    string `json:"effort,omitempty"`
	MaxTokens *int   `json:"max_tokens,omitempty"`
	Exclude   bool   `json:"exclude,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

type Provider struct {
	Order             []string `json:"order,omitempty"`
	AllowFallbacks    *bool    `json:"allow_fallbacks,omitempty"`
	RequireParameters *bool    `json:"require_parameters,omitempty"`
	DataCollection    string   `json:"data_collection,omitempty"`
}

type Response struct {
	ID      string    `json:"id"`
	Choices []Choice  `json:"choices"`
	Usage   *Usage    `json:"usage,omitempty"`
	Error   *APIError `json:"error,omitempty"`
}

type APIError struct {
	Message string `json:"message"`
	Code    any    `json:"code,omitempty"`
}

type Choice struct {
	Message          ChoiceMessage     `json:"message"`
	ReasoningDetails []ReasoningDetail `json:"reasoning_details,omitempty"`
}

type ChoiceMessage struct {
	Content   string `json:"content"`
	Reasoning string `json:"reasoning,omitempty"`
}

type ReasoningDetail struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
}

type Usage struct {
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	TotalTokens      int      `json:"total_tokens"`
	Cost             *float64 `json:"cost,omitempty"`
}

type KeyInfoResponse struct {
	Data KeyInfoData `json:"data"`
}

type KeyInfoData struct {
	Label              string   `json:"label"`
	Limit              *float64 `json:"limit"`
	LimitReset         *string  `json:"limit_reset"`
	LimitRemaining     *float64 `json:"limit_remaining"`
	IncludeBYOKInLimit bool     `json:"include_byok_in_limit"`
	Usage              float64  `json:"usage"`
	UsageDaily         float64  `json:"usage_daily"`
	UsageWeekly        float64  `json:"usage_weekly"`
	UsageMonthly       float64  `json:"usage_monthly"`
	IsFreeTier         bool     `json:"is_free_tier"`
}

// keyInfoURL returns the /key endpoint URL.
func (c *Client) keyInfoURL() string {
	return c.baseURL + "/key"
}

// KeyInfo fetches API key usage info from the /api/v1/key endpoint.
func (c *Client) KeyInfo(ctx context.Context) (*KeyInfoResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.keyInfoURL(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	if c.verbose && c.output != nil {
		dump, _ := httputil.DumpRequestOut(httpReq, true)
		_, _ = fmt.Fprintf(c.output, "\n=== REQUEST [key] ===\n%s\n", dump)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if c.verbose && c.output != nil {
		_, _ = fmt.Fprintf(c.output, "\n=== RESPONSE [key] ===\nHTTP/%d.%d %s\n\n%s\n",
			resp.ProtoMajor, resp.ProtoMinor, resp.Status, body)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var result KeyInfoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

type Result struct {
	Name       string   `json:"name"`
	Model      string   `json:"-"`
	Status     string   `json:"status"`
	Content    string   `json:"content,omitempty"`
	Reasoning  string   `json:"-"`
	Error      string   `json:"error,omitempty"`
	DurationMs int64    `json:"duration_ms"`
	Cost       *float64 `json:"cost,omitempty"`
	Usage      *Usage   `json:"-"`
}

func (c *Client) Execute(ctx context.Context, model *config.Model, systemPrompt, userPrompt string) Result {
	start := time.Now()
	result := Result{
		Name:  model.Name,
		Model: model.Model,
	}

	req := c.buildRequest(model, systemPrompt, userPrompt)

	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				result.Status = "error"
				result.Error = ctx.Err().Error()
				result.DurationMs = time.Since(start).Milliseconds()
				return result
			case <-time.After(retryDelay):
			}
		}

		resp, err := c.doRequest(ctx, model.Name, req)
		if err != nil {
			lastErr = err
			if !isRetryable(err) {
				break
			}
			continue
		}

		result.Status = "success"
		if len(resp.Choices) > 0 {
			result.Content = resp.Choices[0].Message.Content
			result.Reasoning = extractReasoning(&resp.Choices[0])
		}
		result.Usage = resp.Usage
		result.DurationMs = time.Since(start).Milliseconds()

		if resp.Usage != nil && resp.Usage.Cost != nil {
			result.Cost = resp.Usage.Cost
		}

		return result
	}

	result.Status = "error"
	result.Error = lastErr.Error()
	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

func extractReasoning(choice *Choice) string {
	if choice.Message.Reasoning != "" {
		return choice.Message.Reasoning
	}

	// fallback: extract from reasoning_details
	for _, detail := range choice.ReasoningDetails {
		if detail.Type == "thinking" && detail.Content != "" {
			return detail.Content
		}
	}

	return ""
}

func (c *Client) buildRequest(model *config.Model, systemPrompt, userPrompt string) *Request {
	var messages []Message
	if systemPrompt != "" {
		messages = append(messages, Message{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, Message{Role: "user", Content: userPrompt})

	req := Request{
		Model:               model.Model,
		Messages:            messages,
		Temperature:         model.Temperature,
		TopP:                model.TopP,
		TopK:                model.TopK,
		FrequencyPenalty:    model.FrequencyPenalty,
		PresencePenalty:     model.PresencePenalty,
		RepetitionPenalty:   model.RepetitionPenalty,
		MinP:                model.MinP,
		TopA:                model.TopA,
		Seed:                model.Seed,
		MaxTokens:           model.MaxTokens,
		MaxCompletionTokens: model.MaxCompletionTokens,
		Stop:                model.Stop,
		IncludeReasoning:    model.IncludeReasoning,
	}

	if model.Reasoning != nil {
		req.Reasoning = &Reasoning{
			Enabled:   model.Reasoning.Enabled,
			Effort:    model.Reasoning.Effort,
			MaxTokens: model.Reasoning.MaxTokens,
			Exclude:   model.Reasoning.Exclude,
			Summary:   model.Reasoning.Summary,
		}
	}

	if model.Provider != nil {
		req.Provider = &Provider{
			Order:             model.Provider.Order,
			AllowFallbacks:    model.Provider.AllowFallbacks,
			RequireParameters: model.Provider.RequireParameters,
			DataCollection:    model.Provider.DataCollection,
		}
	}

	return &req
}

func (c *Client) doRequest(ctx context.Context, name string, req *Request) (*Response, error) {
	httpReq, err := c.buildHTTPRequest(ctx, name, req)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return c.handleResponse(name, resp)
}

func (c *Client) buildHTTPRequest(ctx context.Context, name string, req *Request) (*http.Request, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	if c.verbose && c.output != nil {
		dump, _ := httputil.DumpRequestOut(httpReq, true)
		_, _ = fmt.Fprintf(c.output, "\n=== REQUEST [%s] ===\n%s\n", name, dump)
	}

	return httpReq, nil
}

func (c *Client) handleResponse(name string, resp *http.Response) (*Response, error) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if c.verbose && c.output != nil {
		_, _ = fmt.Fprintf(c.output, "\n=== RESPONSE [%s] ===\nHTTP/%d.%d %s\n\n%s\n",
			name, resp.ProtoMajor, resp.ProtoMinor, resp.Status, respBody)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &retryableError{statusCode: resp.StatusCode, body: string(respBody)}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}

	return c.parseResponse(respBody)
}

func (c *Client) parseResponse(body []byte) (*Response, error) {
	var result Response
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, &retryableError{statusCode: 0, body: fmt.Sprintf("unmarshal response: %s", err.Error())}
	}

	if result.Error != nil {
		if isRetryableAPIError(result.Error) {
			return nil, &retryableError{statusCode: http.StatusBadGateway, body: result.Error.Message}
		}
		return nil, fmt.Errorf("api error: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return nil, &retryableError{statusCode: 0, body: "no choices in response"}
	}

	return &result, nil
}

type retryableError struct {
	statusCode int
	body       string
}

func (e *retryableError) Error() string {
	return fmt.Sprintf("retryable error %d: %s", e.statusCode, e.body)
}

func isRetryable(err error) bool {
	// never retry on user cancellation (Ctrl+C)
	if errors.Is(err, context.Canceled) {
		return false
	}

	var re *retryableError
	if errors.As(err, &re) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// http/2 stream errors and unexpected EOF during body read
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// case-insensitive pattern matching for transport errors
	errStr := strings.ToLower(err.Error())
	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func isRetryableAPIError(apiErr *APIError) bool {
	if retryable, decided := isRetryableByCode(apiErr.Code); decided {
		return retryable
	}

	// fallback: message pattern matching
	msg := strings.ToLower(apiErr.Message)
	for _, p := range retryableAPIPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}

	return false
}

func isRetryableByCode(code any) (retryable, decided bool) {
	switch v := code.(type) {
	case float64:
		c := int(v)
		if c == http.StatusTooManyRequests || c >= 500 {
			return true, true
		}
		if c >= 400 && c < 500 {
			return false, true
		}
	case string:
		s := strings.ToLower(v)
		for _, p := range []string{"rate_limit", "timeout", "overload", "server_error"} {
			if strings.Contains(s, p) {
				return true, true
			}
		}
	}

	return false, false
}
