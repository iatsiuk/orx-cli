package modelsel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"sort"
	"strconv"
	"strings"
)

const defaultBaseURL = "https://openrouter.ai/api/v1"

type ModelsResponse struct {
	Data []APIModel `json:"data"`
}

type APIModel struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	ContextLength       int            `json:"context_length"`
	Architecture        Architecture   `json:"architecture"`
	Pricing             Pricing        `json:"pricing"`
	SupportedParameters []string       `json:"supported_parameters"`
	DefaultParameters   map[string]any `json:"default_parameters"`
}

type Architecture struct {
	Modality string `json:"modality"`
}

type Pricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

func fetchModelsWithURL(ctx context.Context, token, baseURL string, verbose bool, verboseW io.Writer) ([]APIModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	if verbose && verboseW != nil {
		dump, _ := httputil.DumpRequestOut(req, false)
		_, _ = fmt.Fprintf(verboseW, ">>> Request:\n%s\n", dump)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if verbose && verboseW != nil {
		dump, _ := httputil.DumpResponse(resp, false)
		_, _ = fmt.Fprintf(verboseW, "<<< Response:\n%s\n", dump)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Data, nil
}

func filterTextModels(models []APIModel) []APIModel {
	var result []APIModel
	for i := range models {
		if canProcessText(models[i].Architecture.Modality) {
			result = append(result, models[i])
		}
	}
	return result
}

// canProcessText checks if modality has text input and text output (e.g. "text->text", "text+image->text")
func canProcessText(modality string) bool {
	if modality == "" {
		return false
	}
	parts := strings.SplitN(modality, "->", 2)
	if len(parts) != 2 {
		return false
	}
	input, output := parts[0], parts[1]
	// input must contain "text", output must be "text"
	return strings.Contains(input, "text") && output == "text"
}

func sortByProvider(models []APIModel) []APIModel {
	sorted := make([]APIModel, len(models))
	copy(sorted, models)

	sort.Slice(sorted, func(i, j int) bool {
		pi := extractProvider(sorted[i].ID)
		pj := extractProvider(sorted[j].ID)
		if pi != pj {
			return pi < pj
		}
		return sorted[i].ID < sorted[j].ID
	})

	return sorted
}

func extractProvider(modelID string) string {
	if idx := strings.Index(modelID, "/"); idx != -1 {
		return modelID[:idx]
	}
	return modelID
}

func formatPricing(price string) string {
	if price == "" || price == "0" {
		return "FREE"
	}

	val, err := strconv.ParseFloat(price, 64)
	if err != nil {
		return price
	}

	if val == 0 {
		return "FREE"
	}

	// convert per-token to per-million-tokens
	perMillion := val * 1_000_000

	if perMillion < 0.01 {
		return "$0.00 per 1M tokens"
	}

	return fmt.Sprintf("$%.2f per 1M tokens", perMillion)
}

func formatContextLength(n int) string {
	if n == 0 {
		return "Unknown"
	}

	s := strconv.Itoa(n)
	var result strings.Builder
	length := len(s)

	for i, r := range s {
		if i > 0 && (length-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(r)
	}

	return result.String() + " tokens"
}
