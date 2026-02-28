// Package files provides file loading functionality for including file contents in LLM prompts.
// It reads specified files, formats them with language-specific headers, concatenates into
// prompt text, and validates estimated token count before sending to LLM providers.
//
// Key features:
//   - Explicit file paths only (no glob patterns)
//   - Binary file detection and skipping
//   - Simple token estimation (~4 chars per token)
//   - Language-specific file headers
//   - Configurable size and token limits
package files
