package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	for i, m := range models {
		gm := buildModel(m)
		writeModel(&sb, gm, i == len(models)-1)
	}

	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	return sb.String()
}

func buildModel(m SelectedModel) generatedModel {
	gm := generatedModel{
		Name:         m.Name,
		Model:        m.ID,
		Enabled:      m.Enabled,
		ActiveParams: make(map[string]any),
	}

	if m.Enabled {
		for _, param := range m.SupportedParameters {
			if val, ok := m.DefaultParameters[param]; ok && val != nil {
				gm.ActiveParams[param] = val
			} else {
				gm.AvailableKeys = append(gm.AvailableKeys, param)
			}
		}
	}

	return gm
}

func writeModel(sb *strings.Builder, gm generatedModel, isLast bool) {
	sb.WriteString("    {\n")
	fmt.Fprintf(sb, "      \"name\": %q,\n", gm.Name)
	fmt.Fprintf(sb, "      \"model\": %q,\n", gm.Model)
	fmt.Fprintf(sb, "      \"enabled\": %t", gm.Enabled)

	for k, v := range gm.ActiveParams {
		sb.WriteString(",\n")
		fmt.Fprintf(sb, "      %q: %s", k, formatValue(v))
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
		data, _ := json.Marshal(val)
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
