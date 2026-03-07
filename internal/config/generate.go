package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type generatedModel struct {
	Name          string
	Model         string
	Enabled       bool
	ActiveParams  map[string]any
	AvailableKeys []string // supported but not in defaults
}

func GenerateFromModels(models []SelectedModel) string {
	var sb strings.Builder
	sb.WriteString("{\n")
	sb.WriteString("  \"models\": [\n")

	for i := range models {
		gm := buildModel(&models[i])
		writeModel(&sb, gm, i == len(models)-1)
	}

	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	return sb.String()
}

func buildModel(m *SelectedModel) generatedModel {
	gm := generatedModel{
		Name:         m.Name,
		Model:        m.ID,
		Enabled:      m.Enabled,
		ActiveParams: make(map[string]any),
	}

	if !m.Enabled {
		if m.ExistingParams != nil {
			gm.ActiveParams = modelToParamsMap(m.ExistingParams)
		}
		return gm
	}

	// for enabled models: ExistingParams as baseline, API DefaultParameters override
	if m.ExistingParams != nil {
		existing := modelToParamsMap(m.ExistingParams)
		for _, param := range m.SupportedParameters {
			if val, ok := existing[param]; ok {
				gm.ActiveParams[param] = val
			}
		}
	}

	applyDefaultParams(&gm, m)

	return gm
}

func applyDefaultParams(gm *generatedModel, m *SelectedModel) {
	for _, param := range m.SupportedParameters {
		if param == "reasoning" {
			if m.ReasoningEffort != "" {
				gm.ActiveParams["reasoning"] = &ReasoningConfig{Effort: m.ReasoningEffort}
			} else {
				// user did not select effort in TUI: remove any baseline from ExistingParams
				delete(gm.ActiveParams, "reasoning")
				gm.AvailableKeys = append(gm.AvailableKeys, "reasoning")
			}
			continue
		}
		if val, ok := m.DefaultParameters[param]; ok && val != nil {
			gm.ActiveParams[param] = val
		} else if _, active := gm.ActiveParams[param]; !active {
			gm.AvailableKeys = append(gm.AvailableKeys, param)
		}
	}
}

func modelToParamsMap(m *Model) map[string]any {
	params := modelSamplingParams(m)
	if m.Stop != nil {
		params["stop"] = m.Stop
	}
	if m.Reasoning != nil {
		params["reasoning"] = m.Reasoning
	}
	if m.IncludeReasoning != nil {
		params["include_reasoning"] = *m.IncludeReasoning
	}
	if m.Provider != nil {
		params["provider"] = m.Provider
	}
	return params
}

func modelSamplingParams(m *Model) map[string]any {
	params := modelPenaltyParams(m)
	if m.TopA != nil {
		params["top_a"] = *m.TopA
	}
	if m.Seed != nil {
		params["seed"] = *m.Seed
	}
	if m.MaxTokens != nil {
		params["max_tokens"] = *m.MaxTokens
	}
	if m.MaxCompletionTokens != nil {
		params["max_completion_tokens"] = *m.MaxCompletionTokens
	}
	return params
}

func modelPenaltyParams(m *Model) map[string]any {
	params := make(map[string]any)
	if m.Temperature != nil {
		params["temperature"] = *m.Temperature
	}
	if m.TopP != nil {
		params["top_p"] = *m.TopP
	}
	if m.TopK != nil {
		params["top_k"] = *m.TopK
	}
	if m.FrequencyPenalty != nil {
		params["frequency_penalty"] = *m.FrequencyPenalty
	}
	if m.PresencePenalty != nil {
		params["presence_penalty"] = *m.PresencePenalty
	}
	if m.RepetitionPenalty != nil {
		params["repetition_penalty"] = *m.RepetitionPenalty
	}
	if m.MinP != nil {
		params["min_p"] = *m.MinP
	}
	return params
}

func writeModel(sb *strings.Builder, gm generatedModel, isLast bool) {
	sb.WriteString("    {\n")
	fmt.Fprintf(sb, "      \"name\": %q,\n", gm.Name)
	fmt.Fprintf(sb, "      \"model\": %q,\n", gm.Model)
	fmt.Fprintf(sb, "      \"enabled\": %t", gm.Enabled)

	keys := make([]string, 0, len(gm.ActiveParams))
	for k := range gm.ActiveParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(",\n")
		fmt.Fprintf(sb, "      %q: %s", k, formatValue(gm.ActiveParams[k]))
	}

	sb.WriteString("\n")

	if len(gm.AvailableKeys) > 0 {
		fmt.Fprintf(sb, "      // available: %s\n", strings.Join(gm.AvailableKeys, ", "))
	}

	sb.WriteString("    }")
	if !isLast {
		sb.WriteString(",")
	}
	sb.WriteString("\n")
}

func formatValue(v any) string {
	switch val := v.(type) {
	case float64:
		if val == float64(int(val)) {
			return fmt.Sprintf("%d", int(val))
		}
		return fmt.Sprintf("%g", val)
	case int:
		return fmt.Sprintf("%d", val)
	case string:
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case nil:
		return "null"
	default:
		data, err := json.Marshal(val)
		if err != nil {
			return "null"
		}
		return string(data)
	}
}

func WriteConfig(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
