package config

import (
	"os"
	"path/filepath"
)

const exampleConfig = `{
  "models": [
    {
      "name": "gpt-4o",
      "model": "openai/gpt-4o",
      "enabled": true,
      "temperature": 0.7,
      "max_tokens": 4096
    },
    {
      "name": "claude-sonnet",
      "model": "anthropic/claude-sonnet-4-20250514",
      "enabled": false,
      "max_tokens": 8192
    },
    {
      "name": "deepseek-r1",
      "model": "deepseek/deepseek-r1",
      "enabled": false,
      "include_reasoning": true,
      "max_tokens": 8192
    }
  ]
}
`

func GenerateExample(outputPath string) error {
	if outputPath == "" {
		outputPath = DefaultConfigPath()
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	return os.WriteFile(outputPath, []byte(exampleConfig), 0o600)
}
