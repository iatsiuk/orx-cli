package files

// EstimateTokens returns estimated token count for text.
// Uses simple heuristic: ~4 characters per token.
// Accuracy is approximately +/-20% for English text and code.
func EstimateTokens(text string) int {
	return len(text) / 4
}
