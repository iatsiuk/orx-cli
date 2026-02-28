package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:cyclop,funlen // table-driven test with many cases
func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json5   string
		wantErr bool
		check   func(*testing.T, *Config)
	}{
		{
			name: "minimal config",
			json5: `{
				"models": [
					{"name": "test", "model": "test/model", "enabled": true}
				]
			}`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Models) != 1 {
					t.Errorf("expected 1 model, got %d", len(cfg.Models))
				}
				if cfg.Models[0].Name != "test" {
					t.Errorf("expected name 'test', got %q", cfg.Models[0].Name)
				}
			},
		},
		{
			name: "with comments",
			json5: `{
				// this is a comment
				"models": []
			}`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Models) != 0 {
					t.Errorf("expected empty models, got %d", len(cfg.Models))
				}
			},
		},
		{
			name: "trailing commas",
			json5: `{
				"models": [
					{"name": "test", "model": "test/model", "enabled": true,},
				],
			}`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Models) != 1 {
					t.Errorf("expected 1 model, got %d", len(cfg.Models))
				}
			},
		},
		{
			name: "with all parameters",
			json5: `{
				"models": [{
					"name": "full",
					"model": "test/full",
					"enabled": true,
					"temperature": 0.7,
					"top_p": 0.9,
					"top_k": 50,
					"frequency_penalty": 0.5,
					"presence_penalty": 0.5,
					"repetition_penalty": 1.1,
					"min_p": 0.1,
					"top_a": 0.2,
					"seed": 42,
					"max_tokens": 1000,
					"reasoning": {
						"effort": "high",
						"exclude": false
					},
					"include_reasoning": true,
					"provider": {
						"order": ["OpenAI", "Azure"],
						"allow_fallbacks": true,
						"data_collection": "deny"
					}
				}]
			}`,
			check: func(t *testing.T, cfg *Config) {
				m := cfg.Models[0]
				if *m.Temperature != 0.7 {
					t.Errorf("unexpected temperature: %v", *m.Temperature)
				}
				if m.Reasoning == nil || m.Reasoning.Effort != "high" {
					t.Error("reasoning not parsed correctly")
				}
				if m.Provider == nil || len(m.Provider.Order) != 2 {
					t.Error("provider not parsed correctly")
				}
			},
		},
		{
			name:    "missing name",
			json5:   `{"models": [{"model": "test/model"}]}`,
			wantErr: true,
		},
		{
			name:    "missing model",
			json5:   `{"models": [{"name": "test"}]}`,
			wantErr: true,
		},
		{
			name:    "temperature out of range",
			json5:   `{"models": [{"name": "t", "model": "m", "temperature": 3.0}]}`,
			wantErr: true,
		},
		{
			name:    "top_p out of range",
			json5:   `{"models": [{"name": "t", "model": "m", "top_p": 1.5}]}`,
			wantErr: true,
		},
		{
			name:    "reasoning effort and max_tokens mutually exclusive",
			json5:   `{"models": [{"name": "t", "model": "m", "reasoning": {"effort": "high", "max_tokens": 100}}]}`,
			wantErr: true,
		},
		{
			name:    "invalid reasoning effort",
			json5:   `{"models": [{"name": "t", "model": "m", "reasoning": {"effort": "invalid"}}]}`,
			wantErr: true,
		},
		{
			name:  "reasoning effort minimal",
			json5: `{"models": [{"name": "t", "model": "m", "reasoning": {"effort": "minimal"}}]}`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Models[0].Reasoning == nil || cfg.Models[0].Reasoning.Effort != "minimal" {
					t.Errorf("expected effort 'minimal', got %v", cfg.Models[0].Reasoning)
				}
			},
		},
		{
			name:  "reasoning effort none",
			json5: `{"models": [{"name": "t", "model": "m", "reasoning": {"effort": "none"}}]}`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Models[0].Reasoning == nil || cfg.Models[0].Reasoning.Effort != "none" {
					t.Errorf("expected effort 'none', got %v", cfg.Models[0].Reasoning)
				}
			},
		},
		{
			name:  "reasoning effort xhigh",
			json5: `{"models": [{"name": "t", "model": "m", "reasoning": {"effort": "xhigh"}}]}`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Models[0].Reasoning == nil || cfg.Models[0].Reasoning.Effort != "xhigh" {
					t.Errorf("expected effort 'xhigh', got %v", cfg.Models[0].Reasoning)
				}
			},
		},
		{
			name:  "reasoning summary auto",
			json5: `{"models": [{"name": "t", "model": "m", "reasoning": {"effort": "high", "summary": "auto"}}]}`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Models[0].Reasoning == nil || cfg.Models[0].Reasoning.Summary != "auto" {
					t.Errorf("expected summary 'auto', got %v", cfg.Models[0].Reasoning)
				}
			},
		},
		{
			name:  "reasoning summary concise",
			json5: `{"models": [{"name": "t", "model": "m", "reasoning": {"summary": "concise"}}]}`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Models[0].Reasoning == nil || cfg.Models[0].Reasoning.Summary != "concise" {
					t.Errorf("expected summary 'concise', got %v", cfg.Models[0].Reasoning)
				}
			},
		},
		{
			name:  "reasoning summary detailed",
			json5: `{"models": [{"name": "t", "model": "m", "reasoning": {"summary": "detailed"}}]}`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Models[0].Reasoning == nil || cfg.Models[0].Reasoning.Summary != "detailed" {
					t.Errorf("expected summary 'detailed', got %v", cfg.Models[0].Reasoning)
				}
			},
		},
		{
			name:    "invalid reasoning summary",
			json5:   `{"models": [{"name": "t", "model": "m", "reasoning": {"summary": "invalid"}}]}`,
			wantErr: true,
		},
		{
			name:  "max_completion_tokens",
			json5: `{"models": [{"name": "t", "model": "m", "max_completion_tokens": 4096}]}`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Models[0].MaxCompletionTokens == nil || *cfg.Models[0].MaxCompletionTokens != 4096 {
					t.Errorf("expected max_completion_tokens 4096, got %v", cfg.Models[0].MaxCompletionTokens)
				}
			},
		},
		{
			name:    "max_completion_tokens zero",
			json5:   `{"models": [{"name": "t", "model": "m", "max_completion_tokens": 0}]}`,
			wantErr: true,
		},
		{
			name:    "max_completion_tokens negative",
			json5:   `{"models": [{"name": "t", "model": "m", "max_completion_tokens": -100}]}`,
			wantErr: true,
		},
		{
			name:  "reasoning enabled",
			json5: `{"models": [{"name": "t", "model": "m", "reasoning": {"enabled": true, "effort": "high"}}]}`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Models[0].Reasoning == nil {
					t.Fatal("reasoning is nil")
				}
				if cfg.Models[0].Reasoning.Enabled == nil || !*cfg.Models[0].Reasoning.Enabled {
					t.Error("expected enabled true")
				}
				if cfg.Models[0].Reasoning.Effort != "high" {
					t.Errorf("expected effort 'high', got %q", cfg.Models[0].Reasoning.Effort)
				}
			},
		},
		{
			name:    "invalid data_collection",
			json5:   `{"models": [{"name": "t", "model": "m", "provider": {"data_collection": "invalid"}}]}`,
			wantErr: true,
		},
		{
			name:    "top_k negative",
			json5:   `{"models": [{"name": "t", "model": "m", "top_k": -1}]}`,
			wantErr: true,
		},
		{
			name:    "frequency_penalty too low",
			json5:   `{"models": [{"name": "t", "model": "m", "frequency_penalty": -3.0}]}`,
			wantErr: true,
		},
		{
			name:    "frequency_penalty too high",
			json5:   `{"models": [{"name": "t", "model": "m", "frequency_penalty": 3.0}]}`,
			wantErr: true,
		},
		{
			name:    "presence_penalty too low",
			json5:   `{"models": [{"name": "t", "model": "m", "presence_penalty": -2.5}]}`,
			wantErr: true,
		},
		{
			name:    "presence_penalty too high",
			json5:   `{"models": [{"name": "t", "model": "m", "presence_penalty": 2.5}]}`,
			wantErr: true,
		},
		{
			name:    "repetition_penalty negative",
			json5:   `{"models": [{"name": "t", "model": "m", "repetition_penalty": -0.5}]}`,
			wantErr: true,
		},
		{
			name:    "repetition_penalty too high",
			json5:   `{"models": [{"name": "t", "model": "m", "repetition_penalty": 2.5}]}`,
			wantErr: true,
		},
		{
			name:    "min_p negative",
			json5:   `{"models": [{"name": "t", "model": "m", "min_p": -0.1}]}`,
			wantErr: true,
		},
		{
			name:    "min_p too high",
			json5:   `{"models": [{"name": "t", "model": "m", "min_p": 1.5}]}`,
			wantErr: true,
		},
		{
			name:    "top_a negative",
			json5:   `{"models": [{"name": "t", "model": "m", "top_a": -0.1}]}`,
			wantErr: true,
		},
		{
			name:    "top_a too high",
			json5:   `{"models": [{"name": "t", "model": "m", "top_a": 1.5}]}`,
			wantErr: true,
		},
		{
			name:    "max_tokens zero",
			json5:   `{"models": [{"name": "t", "model": "m", "max_tokens": 0}]}`,
			wantErr: true,
		},
		{
			name:    "max_tokens negative",
			json5:   `{"models": [{"name": "t", "model": "m", "max_tokens": -100}]}`,
			wantErr: true,
		},
		{
			name:    "reasoning max_tokens zero",
			json5:   `{"models": [{"name": "t", "model": "m", "reasoning": {"max_tokens": 0}}]}`,
			wantErr: true,
		},
		{
			name:    "temperature negative",
			json5:   `{"models": [{"name": "t", "model": "m", "temperature": -0.5}]}`,
			wantErr: true,
		},
		{
			name:    "top_p negative",
			json5:   `{"models": [{"name": "t", "model": "m", "top_p": -0.1}]}`,
			wantErr: true,
		},
		{
			name:  "empty models array valid",
			json5: `{"models": []}`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Models) != 0 {
					t.Errorf("expected 0 models, got %d", len(cfg.Models))
				}
			},
		},
		{
			name: "valid boundary values",
			json5: `{"models": [{
				"name": "t", "model": "m",
				"temperature": 0.0,
				"top_p": 0.0,
				"top_k": 0,
				"frequency_penalty": -2.0,
				"presence_penalty": 2.0,
				"repetition_penalty": 0.0,
				"min_p": 0.0,
				"top_a": 1.0,
				"max_tokens": 1
			}]}`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Models) != 1 {
					t.Errorf("expected 1 model, got %d", len(cfg.Models))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpFile := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(tmpFile, []byte(tt.json5), 0o600); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}

			cfg, err := Load(tmpFile)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestEnabledModels(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Models: []Model{
			{Name: "m1", Enabled: true},
			{Name: "m2", Enabled: false},
			{Name: "m3", Enabled: true},
		},
	}

	enabled := cfg.EnabledModels()
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled models, got %d", len(enabled))
	}

	if enabled[0].Name != "m1" || enabled[1].Name != "m3" {
		t.Error("wrong enabled models returned")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got %v", err)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(tmpFile, []byte("not json at all {{{"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmpFile)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	t.Parallel()

	path := DefaultConfigPath()
	if path == "" {
		t.Fatal("DefaultConfigPath() returned empty string")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
	if filepath.Base(path) != "orx.json" {
		t.Errorf("expected orx.json, got %q", filepath.Base(path))
	}

	// verify path is under user home directory
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home directory: %v", err)
	}
	if !strings.HasPrefix(path, home) {
		t.Errorf("expected path under home directory %q, got %q", home, path)
	}
}
