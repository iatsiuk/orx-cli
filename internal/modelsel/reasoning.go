package modelsel

import "orx/internal/config"

var effortOrder = []string{"", "none", "minimal", "low", "medium", "high", "xhigh"}

func nextEffort(current string) string {
	for i, e := range effortOrder {
		if e == current {
			return effortOrder[(i+1)%len(effortOrder)]
		}
	}
	return "none"
}

func supportsReasoning(params []string) bool {
	for _, p := range params {
		if p == "reasoning" {
			return true
		}
	}
	return false
}

func filterReasoningSelectedModels(models []config.SelectedModel) []config.SelectedModel {
	var result []config.SelectedModel
	for i := range models {
		if supportsReasoning(models[i].SupportedParameters) {
			result = append(result, models[i])
		}
	}
	return result
}
