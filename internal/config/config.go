package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/titanous/json5"
)

type Config struct {
	Models []Model `json:"models"`
}

type Model struct {
	Name                string           `json:"name"`
	Model               string           `json:"model"`
	Enabled             bool             `json:"enabled"`
	Temperature         *float64         `json:"temperature,omitempty"`
	TopP                *float64         `json:"top_p,omitempty"`
	TopK                *int             `json:"top_k,omitempty"`
	FrequencyPenalty    *float64         `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float64         `json:"presence_penalty,omitempty"`
	RepetitionPenalty   *float64         `json:"repetition_penalty,omitempty"`
	MinP                *float64         `json:"min_p,omitempty"`
	TopA                *float64         `json:"top_a,omitempty"`
	Seed                *int             `json:"seed,omitempty"`
	MaxTokens           *int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int             `json:"max_completion_tokens,omitempty"`
	Stop                any              `json:"stop,omitempty"`
	Reasoning           *ReasoningConfig `json:"reasoning,omitempty"`
	IncludeReasoning    *bool            `json:"include_reasoning,omitempty"`
	Provider            *ProviderConfig  `json:"provider,omitempty"`
}

type ReasoningConfig struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Effort    string `json:"effort,omitempty"`
	MaxTokens *int   `json:"max_tokens,omitempty"`
	Exclude   bool   `json:"exclude,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

type ProviderConfig struct {
	Order             []string `json:"order,omitempty"`
	AllowFallbacks    *bool    `json:"allow_fallbacks,omitempty"`
	RequireParameters *bool    `json:"require_parameters,omitempty"`
	DataCollection    string   `json:"data_collection,omitempty"`
}

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "orx.json")
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json5.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) EnabledModels() []Model {
	var models []Model
	for i := range c.Models {
		if c.Models[i].Enabled {
			models = append(models, c.Models[i])
		}
	}
	return models
}

func (c *Config) Validate() error {
	for i := range c.Models {
		if err := c.validateModel(i); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateModel(i int) error {
	m := &c.Models[i]
	if m.Name == "" {
		return fmt.Errorf("model[%d]: name is required", i)
	}
	if m.Model == "" {
		return fmt.Errorf("model[%d] %q: model is required", i, m.Name)
	}
	if err := validateSamplingParams(m); err != nil {
		return err
	}
	if m.Reasoning != nil {
		if err := validateReasoning(m.Name, m.Reasoning); err != nil {
			return err
		}
	}
	if m.Provider != nil {
		if err := validateProvider(m.Name, m.Provider); err != nil {
			return err
		}
	}
	return nil
}

func validateSamplingParams(m *Model) error {
	checks := []error{
		validateRange(m.Name, "top_p", m.TopP, 0, 1),
		validateRange(m.Name, "min_p", m.MinP, 0, 1),
		validateRange(m.Name, "top_a", m.TopA, 0, 1),
		validateRange(m.Name, "temperature", m.Temperature, 0, 2),
		validateRange(m.Name, "frequency_penalty", m.FrequencyPenalty, -2, 2),
		validateRange(m.Name, "presence_penalty", m.PresencePenalty, -2, 2),
		validateRange(m.Name, "repetition_penalty", m.RepetitionPenalty, 0, 2),
		validateMinInt(m.Name, "top_k", m.TopK, 0),
		validateMinInt(m.Name, "max_tokens", m.MaxTokens, 1),
		validateMinInt(m.Name, "max_completion_tokens", m.MaxCompletionTokens, 1),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}

func validateRange(name, param string, val *float64, lo, hi float64) error {
	if val != nil && (*val < lo || *val > hi) {
		return fmt.Errorf("model %q: %s must be between %.1f and %.1f", name, param, lo, hi)
	}
	return nil
}

func validateMinInt(name, param string, val *int, lo int) error {
	if val != nil && *val < lo {
		return fmt.Errorf("model %q: %s must be >= %d", name, param, lo)
	}
	return nil
}

func validateReasoning(name string, r *ReasoningConfig) error {
	if r.Effort != "" && r.MaxTokens != nil {
		return fmt.Errorf("model %q: reasoning.effort and reasoning.max_tokens are mutually exclusive", name)
	}

	if r.Effort != "" {
		validEfforts := map[string]bool{
			"none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true,
		}
		if !validEfforts[r.Effort] {
			return fmt.Errorf("model %q: reasoning.effort must be 'none', 'minimal', 'low', 'medium', 'high', or 'xhigh'", name)
		}
	}

	if r.Summary != "" {
		validSummaries := map[string]bool{
			"auto": true, "concise": true, "detailed": true,
		}
		if !validSummaries[r.Summary] {
			return fmt.Errorf("model %q: reasoning.summary must be 'auto', 'concise', or 'detailed'", name)
		}
	}

	if r.MaxTokens != nil && *r.MaxTokens < 1 {
		return fmt.Errorf("model %q: reasoning.max_tokens must be >= 1", name)
	}

	return nil
}

func validateProvider(name string, p *ProviderConfig) error {
	if p.DataCollection != "" {
		if p.DataCollection != "allow" && p.DataCollection != "deny" {
			return fmt.Errorf("model %q: provider.data_collection must be 'allow' or 'deny'", name)
		}
	}
	return nil
}

var ErrNoEnabledModels = errors.New("no enabled models in configuration")

type SelectedModel struct {
	ID                  string
	Name                string
	Enabled             bool
	SupportedParameters []string
	DefaultParameters   map[string]any
	ExistingParams      *Model
}
