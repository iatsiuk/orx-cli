package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateExample(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path func(t *testing.T) string
	}{
		{"custom path", func(t *testing.T) string { return filepath.Join(t.TempDir(), "orx.json") }},
		{"creates parent directories", func(t *testing.T) string { return filepath.Join(t.TempDir(), "nested", "dir", "orx.json") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.path(t)
			if err := GenerateExample(path); err != nil {
				t.Fatalf("GenerateExample() error: %v", err)
			}

			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Error("expected file to be created")
			}
		})
	}
}

func TestGenerateExample_ConfigPassesValidation(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "orx.json")
	if err := GenerateExample(path); err != nil {
		t.Fatalf("GenerateExample() error: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("generated config failed validation: %v", err)
	}

	enabled := cfg.EnabledModels()
	if len(enabled) == 0 {
		t.Error("expected at least one enabled model in example config")
	}
}
