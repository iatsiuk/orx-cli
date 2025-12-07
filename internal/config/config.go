package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/titanous/json5"
)

type Config struct {
	SystemPrompt string  `json:"system_prompt"`
	Models       []Model `json:"models"`
}

type Model struct {
	Name              string           `json:"name"`
	Model             string           `json:"model"`
	Enabled           bool             `json:"enabled"`
	Temperature       *float64         `json:"temperature,omitempty"`
	TopP              *float64         `json:"top_p,omitempty"`
	TopK              *int             `json:"top_k,omitempty"`
	FrequencyPenalty  *float64         `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float64         `json:"presence_penalty,omitempty"`
	RepetitionPenalty *float64         `json:"repetition_penalty,omitempty"`
	MinP              *float64         `json:"min_p,omitempty"`
	TopA              *float64         `json:"top_a,omitempty"`
	Seed              *int             `json:"seed,omitempty"`
	MaxTokens         *int             `json:"max_tokens,omitempty"`
	Stop              any              `json:"stop,omitempty"`
	Reasoning         *ReasoningConfig `json:"reasoning,omitempty"`
	IncludeReasoning  *bool            `json:"include_reasoning,omitempty"`
	Provider          *ProviderConfig  `json:"provider,omitempty"`
}

type ReasoningConfig struct {
	Effort    string `json:"effort,omitempty"`
	MaxTokens *int   `json:"max_tokens,omitempty"`
	Exclude   bool   `json:"exclude,omitempty"`
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
	if err := validateTemperatureParams(m); err != nil {
		return err
	}
	return validatePenaltyParams(m)
}

func validateTemperatureParams(m *Model) error {
	if err := validateRange01(m.Name, "top_p", m.TopP); err != nil {
		return err
	}
	if err := validateRange01(m.Name, "min_p", m.MinP); err != nil {
		return err
	}
	if err := validateRange01(m.Name, "top_a", m.TopA); err != nil {
		return err
	}
	return validateOtherParams(m)
}

func validateOtherParams(m *Model) error {
	if m.Temperature != nil && (*m.Temperature < 0.0 || *m.Temperature > 2.0) {
		return fmt.Errorf("model %q: temperature must be between 0.0 and 2.0", m.Name)
	}
	if m.TopK != nil && *m.TopK < 0 {
		return fmt.Errorf("model %q: top_k must be >= 0", m.Name)
	}
	if m.MaxTokens != nil && *m.MaxTokens < 1 {
		return fmt.Errorf("model %q: max_tokens must be >= 1", m.Name)
	}
	return nil
}

func validateRange01(name, param string, val *float64) error {
	if val != nil && (*val < 0.0 || *val > 1.0) {
		return fmt.Errorf("model %q: %s must be between 0.0 and 1.0", name, param)
	}
	return nil
}

func validatePenaltyParams(m *Model) error {
	if m.FrequencyPenalty != nil && (*m.FrequencyPenalty < -2.0 || *m.FrequencyPenalty > 2.0) {
		return fmt.Errorf("model %q: frequency_penalty must be between -2.0 and 2.0", m.Name)
	}
	if m.PresencePenalty != nil && (*m.PresencePenalty < -2.0 || *m.PresencePenalty > 2.0) {
		return fmt.Errorf("model %q: presence_penalty must be between -2.0 and 2.0", m.Name)
	}
	if m.RepetitionPenalty != nil && (*m.RepetitionPenalty < 0.0 || *m.RepetitionPenalty > 2.0) {
		return fmt.Errorf("model %q: repetition_penalty must be between 0.0 and 2.0", m.Name)
	}
	return nil
}

func validateReasoning(name string, r *ReasoningConfig) error {
	if r.Effort != "" && r.MaxTokens != nil {
		return fmt.Errorf("model %q: reasoning.effort and reasoning.max_tokens are mutually exclusive", name)
	}

	if r.Effort != "" {
		validEfforts := map[string]bool{"low": true, "medium": true, "high": true}
		if !validEfforts[r.Effort] {
			return fmt.Errorf("model %q: reasoning.effort must be 'low', 'medium', or 'high'", name)
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
