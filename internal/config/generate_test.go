package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titanous/json5"
)

func TestGenerateFromModels_RealAPIModels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		model         SelectedModel
		wantInOutput  []string
		wantActive    []string // params as JSON keys (have defaults)
		wantNotActive []string // params with nil defaults
	}{
		{
			name: "GPT-5.2 reasoning model",
			model: SelectedModel{
				ID:                  "openai/gpt-5.2",
				Name:                "OpenAI: GPT-5.2",
				Enabled:             true,
				SupportedParameters: []string{"include_reasoning", "max_tokens", "reasoning", "seed"},
				DefaultParameters:   map[string]any{"temperature": nil},
			},
			wantInOutput:  []string{"reasoning", "include_reasoning", "max_tokens", "seed"},
			wantNotActive: []string{"temperature"},
		},
		{
			name: "DeepSeek with defaults",
			model: SelectedModel{
				ID:                  "deepseek/deepseek-v3.2",
				Name:                "DeepSeek V3.2",
				Enabled:             true,
				SupportedParameters: []string{"reasoning", "temperature", "top_p"},
				DefaultParameters:   map[string]any{"temperature": float64(1), "top_p": float64(0.95)},
			},
			wantInOutput: []string{"reasoning"},
			wantActive:   []string{`"temperature"`, `"top_p"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := GenerateFromModels([]SelectedModel{tt.model})
			for _, want := range tt.wantInOutput {
				if !strings.Contains(result, want) {
					t.Errorf("expected %q in output", want)
				}
			}
			for _, want := range tt.wantActive {
				if !strings.Contains(result, want) {
					t.Errorf("expected active param %q", want)
				}
			}
			for _, notWant := range tt.wantNotActive {
				if containsAsJSONKey(result, notWant) {
					t.Errorf("param %q with nil default should not be active", notWant)
				}
			}
		})
	}
}

// checks if param appears as JSON key (not in comment)
func containsAsJSONKey(output, param string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "//") {
			continue
		}
		if strings.Contains(line, `"`+param+`"`) {
			return true
		}
	}
	return false
}

func TestGenerateFromModels_AllSupportedParamsInAvailable(t *testing.T) {
	t.Parallel()

	params := []string{"reasoning", "include_reasoning", "max_tokens", "temperature", "custom_param"}
	models := []SelectedModel{
		{
			ID:                  "test/model",
			Name:                "Test Model",
			Enabled:             true,
			SupportedParameters: params,
			DefaultParameters:   map[string]any{},
		},
	}

	result := GenerateFromModels(models)

	for _, param := range params {
		if !strings.Contains(result, param) {
			t.Errorf("expected %q in output", param)
		}
	}
}

func TestGenerateFromModels_DefaultParamsAsActive(t *testing.T) {
	t.Parallel()

	models := []SelectedModel{
		{
			ID:                  "test/model",
			Name:                "Test Model",
			Enabled:             true,
			SupportedParameters: []string{"temperature", "top_p", "max_tokens"},
			DefaultParameters: map[string]any{
				"temperature": float64(0.7),
				"top_p":       nil,
			},
		},
	}

	result := GenerateFromModels(models)

	if !strings.Contains(result, `"temperature": 0.7`) {
		t.Error("expected temperature as active param with value 0.7")
	}
	if !strings.Contains(result, "// available:") {
		t.Error("expected available comment")
	}
	if !strings.Contains(result, "top_p") {
		t.Error("expected top_p in available")
	}
	if !strings.Contains(result, "max_tokens") {
		t.Error("expected max_tokens in available")
	}
}

func TestGenerateFromModels_EmptyList(t *testing.T) {
	t.Parallel()

	result := GenerateFromModels(nil)

	// should have models key with empty array (multiline format)
	if !strings.Contains(result, `"models"`) {
		t.Error("expected models key")
	}
	// should not contain any model entries
	if strings.Contains(result, `"model":`) {
		t.Error("expected no model entries")
	}
}

func TestGenerateFromModels_MultipleModels(t *testing.T) {
	t.Parallel()

	models := []SelectedModel{
		{ID: "model/a", Name: "Model A", Enabled: true, SupportedParameters: []string{"temperature"}},
		{ID: "model/b", Name: "Model B", Enabled: true, SupportedParameters: []string{"max_tokens"}},
	}

	result := GenerateFromModels(models)

	if !strings.Contains(result, "model/a") {
		t.Error("expected model/a in output")
	}
	if !strings.Contains(result, "model/b") {
		t.Error("expected model/b in output")
	}
	if strings.Count(result, `"enabled": true`) != 2 {
		t.Error("expected 2 enabled models")
	}
}

func TestGenerateFromModels_DisabledModel(t *testing.T) {
	t.Parallel()

	models := []SelectedModel{
		{ID: "model/a", Name: "Model A", Enabled: false},
	}

	result := GenerateFromModels(models)

	if !strings.Contains(result, `"enabled": false`) {
		t.Error("expected \"enabled\": false in output")
	}
	if strings.Contains(result, "// available:") {
		t.Error("expected no available comment for disabled model")
	}
}

func TestGenerateFromModels_MixedEnabledDisabled(t *testing.T) {
	t.Parallel()

	models := []SelectedModel{
		{ID: "model/a", Name: "Model A", Enabled: true, SupportedParameters: []string{"temperature"}},
		{ID: "model/b", Name: "Model B", Enabled: false},
	}

	result := GenerateFromModels(models)

	if strings.Count(result, `"enabled": true`) != 1 {
		t.Error("expected 1 enabled model")
	}
	if strings.Count(result, `"enabled": false`) != 1 {
		t.Error("expected 1 disabled model")
	}
}

func TestGenerateFromModels_OutputIsValidJSON5(t *testing.T) {
	t.Parallel()

	models := []SelectedModel{
		{
			ID:                  "test/model",
			Name:                "Test Model",
			Enabled:             true,
			SupportedParameters: []string{"temperature", "max_tokens"},
			DefaultParameters:   map[string]any{"temperature": float64(0.7)},
		},
	}

	result := GenerateFromModels(models)

	// strip comments for JSON validation
	lines := strings.Split(result, "\n")
	jsonLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if idx := strings.Index(line, "//"); idx != -1 {
			line = line[:idx]
		}
		jsonLines = append(jsonLines, line)
	}
	jsonStr := strings.Join(jsonLines, "\n")

	var cfg Config
	if err := json5.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		t.Errorf("generated config is not valid JSON5: %v", err)
	}
	if len(cfg.Models) != 1 {
		t.Errorf("expected 1 model, got %d", len(cfg.Models))
	}
}

func TestWriteConfig(t *testing.T) {
	t.Parallel()

	t.Run("creates parent directories", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "deep", "config.json")
		content := `{"models": []}`

		err := WriteConfig(path, content)
		if err != nil {
			t.Fatalf("WriteConfig failed: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read written file: %v", err)
		}
		if string(data) != content {
			t.Errorf("content mismatch: got %q, want %q", string(data), content)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")

		_ = WriteConfig(path, "old content")
		err := WriteConfig(path, "new content")
		if err != nil {
			t.Fatalf("WriteConfig failed: %v", err)
		}

		data, _ := os.ReadFile(path)
		if string(data) != "new content" {
			t.Errorf("expected overwritten content")
		}
	})

	t.Run("file permissions", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")

		_ = WriteConfig(path, "content")

		info, _ := os.Stat(path)
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Errorf("expected 0600 permissions, got %o", perm)
		}
	})
}
