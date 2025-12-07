# ORX - OpenRouter eXecutor

## 1. Overview

### 1.1 Purpose

ORX is a CLI tool that sends prompts to multiple LLM models via [OpenRouter API](https://openrouter.ai/docs/quickstart) in parallel and aggregates responses. It replaces the need for separate subagent wrappers per model in Claude Code slash commands.

### 1.2 Goals

- Single binary, zero runtime dependencies
- Parallel execution to all enabled models
- JSON with Comments (JSONC) configuration
- Structured JSON output for easy parsing
- Retry with backoff for transient failures
- Verbose mode for HTTP debugging
- Graceful shutdown on SIGINT/SIGTERM
- Interactive TUI for model selection during init

### 1.3 Non-Goals

- Streaming output (wait for complete response)
- Model management beyond init (use OpenRouter dashboard)

---

## 2. CLI Interface

### 2.1 Usage

```
orx [flags] < prompt.txt
orx init [--output path] [--template]

Global Flags:
      --token string        OpenRouter API key (default: $OPENROUTER_API_KEY)
      --verbose             Dump HTTP request/response to stderr
  -h, --help                Show help
  -v, --version             Show version

Run Flags:
  -c, --config string       Path to config file (default: ~/.config/orx.json)
  -t, --timeout int         Global timeout in seconds (default: 600)
  -p, --prompt-file string  Read prompt from file instead of stdin

Init Flags:
  -o, --output string       Output path (default: ~/.config/orx.json)
      --template            Generate static template without interactive TUI

Subcommands:
  init    Interactive model selection and configuration file generation
```

### 2.2 Input Priority

1. `--prompt-file` flag (if specified)
2. stdin (heredoc or pipe)

### 2.3 Token Priority

1. `--token` flag
2. `OPENROUTER_API_KEY` environment variable

### 2.4 Exit Codes

| Code | Meaning                                            |
| ---- | -------------------------------------------------- |
| 0    | Success / user cancelled (Esc, overwrite declined) |
| 1    | Partial failure (at least one succeeded)           |
| 2    | All models failed                                  |
| 3    | Configuration or input error                       |

---

## 3. Init Command - TUI Model Selector

### 3.1 Overview

The `orx init` command launches an interactive TUI that:
1. Fetches available models from OpenRouter API
2. Displays a filterable, scrollable list of text-only models
3. Allows multi-select with detailed model information
4. Generates configuration file with selected models

### 3.2 Requirements

| Requirement | Value |
|-------------|-------|
| TUI library | `github.com/rivo/tview` |
| API endpoint | `GET https://openrouter.ai/api/v1/models` |
| API timeout | 30 seconds |
| API retry | None (fail immediately on error) |
| Auth | Required (global `--token` flag or `OPENROUTER_API_KEY` env) |
| Model filter | Models that can process text input and produce text output |
| Search filter | By model name (case-insensitive substring) |
| Sort order | Grouped by provider (anthropic/*, openai/*, ...) |

### 3.3 UI Layout

```
+----------------------------------------+----------------------------------+
| Search: [___________________________]  |                                  |
+----------------------------------------+  anthropic/claude-sonnet-4       |
| ( ) anthropic/claude-sonnet-4          |                                  |
| (x) anthropic/claude-3.5-haiku         |  Context: 200,000 tokens         |
| ( ) deepseek/deepseek-r1               |                                  |
| ( ) google/gemini-2.0-flash            |  Pricing:                        |
| (x) openai/gpt-4o                      |    Input:  $3.00 per 1M tokens   |
| ( ) openai/gpt-4o-mini                 |    Output: $15.00 per 1M tokens  |
| ...                                    |                                  |
|                                        |  Description:                    |
|                                        |  Most intelligent Claude model   |
|                                        |  with advanced reasoning...      |
|                                        |                                  |
+----------------------------------------+----------------------------------+
| Space Toggle  Enter Done  / Search  Tab Switch  Esc Cancel  Selected: 2   |
+--------------------------------------------------------------------------+
```

### 3.4 UI Components

1. **Search InputField** (top-left) - filters model list by name
2. **Model List** (left panel) - scrollable list with `(x)`/`( )` selection markers
3. **Details TextView** (right panel) - shows info for highlighted model
4. **Status Bar** (bottom) - hotkeys help + selection count

### 3.5 Hotkeys

| Key | Action |
|-----|--------|
| `Space` | Toggle selection of highlighted model |
| `Enter` | Confirm selection and generate config |
| `Esc` | Cancel and exit without saving |
| `/` | Focus search input |
| `Tab` | Switch focus between model list and details panel |
| `Up/Down` | Navigate model list (or scroll details when focused) |
| Any letter (when list focused) | Start typing to search |

### 3.6 Details Panel Display

| Field | Format | Example |
|-------|--------|---------|
| Model ID | As-is | `anthropic/claude-sonnet-4` |
| Context length | With thousands separator | `200,000 tokens` |
| Input pricing | Per 1M tokens or FREE | `$3.00 per 1M tokens` |
| Output pricing | Per 1M tokens or FREE | `$15.00 per 1M tokens` |
| Description | Wrapped text | Model capabilities... |

### 3.7 Generated Config

Selected models are written to config (JSONC format) with:
- `name`: From API `name` field (e.g., "OpenAI: GPT-4o")
- `model`: From API `id` field (e.g., "openai/gpt-4o")
- `enabled`: `true` for all selected models
- Parameters: From `default_parameters` (non-null values only)
- Comment: `// available: ...` listing other supported parameters without defaults
- `system_prompt`: Empty string `""`

Example output:

```jsonc
{
  "system_prompt": "",
  "models": [
    {
      "name": "OpenAI: GPT-4o",
      "model": "openai/gpt-4o",
      "enabled": true,
      "temperature": 1,
      "max_tokens": 4096
      // available: top_p, frequency_penalty, presence_penalty, stop
    },
    {
      "name": "Anthropic: Claude Sonnet 4",
      "model": "anthropic/claude-sonnet-4",
      "enabled": true
      // available: temperature, max_tokens, top_p, top_k
    }
  ]
}
```

### 3.8 Overwrite Prompt

When config file exists, prompt user before launching TUI:

```
File ~/.config/orx.json exists. Overwrite? [y/N]
```

- `y` or `yes` (case-insensitive): continue to TUI
- Any other input or empty: print "Aborted", exit 0

### 3.9 Edge Cases

| Scenario | Behavior | Exit |
|----------|----------|------|
| API unavailable | Show error message | 3 |
| API timeout (30s) | Show error message | 3 |
| API rate limit (429) | Show error message (no retry) | 3 |
| Empty model list after filtering | Show "No text models available" | 3 |
| No models selected + Enter | Show warning, stay in TUI | - |
| User presses Esc | Print "Cancelled" | 0 |
| File exists, user declines overwrite | Print "Aborted" | 0 |
| File exists, user confirms overwrite | Continue to TUI | - |
| Ctrl+C during API fetch | Cancel request, exit | 0 |

---

## 4. Configuration

### 4.1 Format

JSON with Comments (JSONC). Supports `//` comments and trailing commas when reading.
Generated config (`orx init`) uses standard JSON for tooling compatibility.

### 4.2 Default Location

`~/.config/orx.json`

### 4.3 Schema

```jsonc
{
  // Global system prompt prepended to all requests (optional)
  // Sent as message with role="system" before user prompt
  "system_prompt": "You are an expert code reviewer...",

  // Array of model configurations
  "models": [
    {
      // --- Required fields ---
      "name": "gpt-4o",           // Display name (used in output)
      "model": "openai/gpt-4o",   // OpenRouter model ID

      // --- Optional fields ---
      "enabled": true,            // Default: false

      // Sampling parameters (OpenRouter API)
      // See: https://openrouter.ai/docs/api/reference/parameters
      "temperature": 0.7,         // 0.0-2.0, default: 1.0
      "top_p": 1.0,               // 0.0-1.0, default: 1.0
      "top_k": 0,                 // 0+, default: 0 (disabled)
      "frequency_penalty": 0.0,   // -2.0 to 2.0, default: 0.0
      "presence_penalty": 0.0,    // -2.0 to 2.0, default: 0.0
      "repetition_penalty": 1.0,  // 0.0-2.0, default: 1.0
      "min_p": 0.0,               // 0.0-1.0, default: 0.0
      "top_a": 0.0,               // 0.0-1.0, default: 0.0
      "seed": null,               // Integer or null
      "max_tokens": 4096,         // 1+, model-dependent
      "stop": null,               // String, array of strings, or null

      // Reasoning parameters (for o1/o3/claude-3.7/deepseek-r1)
      // See: https://openrouter.ai/docs/use-cases/reasoning-tokens
      "reasoning": {
        "effort": "high",         // "low" | "medium" | "high"
        // OR (mutually exclusive - if both specified, validation error)
        "max_tokens": 2000,       // Exact token limit
        "exclude": false          // Hide reasoning from response
      },

      // Include reasoning tokens in response (for thinking models)
      "include_reasoning": true,

      // Provider routing (optional)
      // See: https://openrouter.ai/docs/guides/routing/provider-selection
      "provider": {
        "order": ["Azure", "OpenAI"],
        "allow_fallbacks": true,
        "require_parameters": true,
        "data_collection": "deny" // "allow" | "deny"
      }
    }
  ]
}
```

Users can add `//` comments manually - they will be parsed correctly.

---

## 5. Output Format

### 5.1 JSON Output (stdout)

```json
{
  "results": [
    {
      "name": "gpt-4o",
      "status": "success",
      "content": "Response text here...",
      "duration_ms": 2345,
      "cost": 0.00492
    },
    {
      "name": "claude-sonnet",
      "status": "success",
      "content": "Response text...",
      "duration_ms": 3000,
      "cost": 0.00315
    },
    {
      "name": "deepseek",
      "status": "error",
      "error": "timeout after 600s",
      "duration_ms": 600000
    }
  ],
  "total_duration_ms": 600000,
  "total_cost": 0.00807,
  "successful": 2,
  "failed": 1
}
```

### 5.2 Cost Reporting

For successful results, `cost` and `cost_error` fields are mutually exclusive:

| Condition | Output |
|-----------|--------|
| Cost available in API response | Include `cost` (USD), omit `cost_error` |
| Cost unavailable or retrieval failed | Include `cost_error` with reason, omit `cost` |

The `total_cost` aggregates only results with valid `cost` values.

### 5.3 Progress Output (stderr)

```
gpt-4o - [requesting]
claude-sonnet - [requesting]
gpt-4o - [done] (2.3s)
claude-sonnet - [error] timeout after 600s
```

### 5.4 Verbose Output (stderr)

When `--verbose` flag is set, dump HTTP request/response using `httputil.DumpRequestOut()` and `httputil.DumpResponse()`:

```
=== REQUEST [gpt-4o] ===
POST /api/v1/chat/completions HTTP/1.1
Host: openrouter.ai
Authorization: Bearer sk-or-v1-...
Content-Type: application/json

{"model":"openai/gpt-4o","messages":[{"role":"user","content":"..."}]}

=== RESPONSE [gpt-4o] ===
HTTP/1.1 200 OK
Content-Type: application/json

{"id":"gen-...","choices":[{"message":{"content":"..."}}]}
```

---

## 6. Architecture

### 6.1 Project Structure

Following [official Go module layout](https://go.dev/doc/modules/layout) and common patterns:

```
orx/
├── cmd/
│   └── orx/
│       └── main.go           # Entry point, CLI setup, JSON output
├── internal/
│   ├── config/
│   │   ├── config.go         # Config struct, loading, validation
│   │   ├── config_test.go
│   │   ├── generate.go       # Config generation from selected models
│   │   └── template.go       # Fallback static template (if needed)
│   ├── client/
│   │   ├── client.go         # OpenRouter HTTP client, request/response structs
│   │   └── client_test.go
│   ├── modelsel/
│   │   ├── modelsel.go       # Public API: Run() -> []SelectedModel
│   │   ├── api.go            # Models API client, filtering, sorting
│   │   ├── ui.go             # TUI components (tview)
│   │   └── modelsel_test.go
│   └── runner/
│       ├── runner.go         # Parallel execution, progress output, result aggregation
│       └── runner_test.go
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── .goreleaser.yaml          # Release automation
```

### 6.2 Component Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         cmd/orx/main.go                      │
│  - Parse CLI flags (cobra)                                   │
│  - Load config / Run init TUI                                │
│  - Read prompt                                               │
│  - Initialize runner                                         │
│  - Output results                                            │
└─────────────────────────────────────────────────────────────┘
              │                              │
              │ (main command)               │ (init subcommand)
              ▼                              ▼
┌──────────────────────────┐    ┌──────────────────────────────┐
│     internal/runner       │    │      internal/modelsel        │
│  - Create errgroup        │    │  - Fetch models from API      │
│  - Launch goroutines      │    │  - Filter text->text only     │
│  - Collect results        │    │  - Display TUI (tview)        │
│  - Handle timeout         │    │  - Return selected models     │
└──────────────────────────┘    └──────────────────────────────┘
              │                              │
              ▼                              ▼
┌──────────────────────────┐    ┌──────────────────────────────┐
│     internal/client       │    │      internal/config          │
│  - Build request          │    │  - GenerateFromModels()       │
│  - Execute with retry     │    │  - WriteConfig()              │
│  - Parse response         │    │  - Load(), Validate()         │
└──────────────────────────┘    └──────────────────────────────┘
```

---

## 7. Dependencies

### 7.1 Direct Dependencies

| Package                      | Purpose                 | Version |
| ---------------------------- | ----------------------- | ------- |
| `github.com/spf13/cobra`     | CLI framework           | v1.8+   |
| `github.com/titanous/json5`  | JSONC parsing           | latest  |
| `golang.org/x/sync/errgroup` | Parallel execution      | latest  |
| `github.com/rivo/tview`      | TUI for model selection | latest  |

### 7.2 Transitive Dependencies (via tview)

| Package                     | Purpose              |
| --------------------------- | -------------------- |
| `github.com/gdamore/tcell/v2` | Terminal handling  |
| `github.com/rivo/uniseg`    | Unicode segmentation |

### 7.3 Standard Library (no external deps)

- `net/http` - HTTP client
- `net/http/httputil` - Request/response dumping
- `context` - Timeout and cancellation
- `encoding/json` - JSON output
- `os/signal` - Graceful shutdown
- `time` - Duration handling
- `sync` - Mutex for result collection
- `io` - Stream handling
- `strconv` - Number formatting
- `strings` - String manipulation
- `sort` - Model sorting

### 7.4 Why These Choices

**Cobra over urfave/cli:**

- More widely used (kubectl, docker, helm)
- Better subcommand support
- Automatic help generation
- Shell completion built-in

**titanous/json5 for JSONC support:**

- Based on encoding/json (familiar API)
- Supports `//` comments and trailing commas
- Struct tags work as expected

**rivo/tview for TUI:**

- Most popular Go TUI library
- Rich widget set (List, InputField, TextView, Flex)
- Good documentation and examples
- Active maintenance

**No external HTTP retry library:**

- Simple retry logic (3 attempts, 5s fixed delay)
- Full control over retry conditions
- Fewer dependencies

---

## 8. OpenRouter Models API

### 8.1 Endpoint

`GET https://openrouter.ai/api/v1/models`

### 8.2 Authentication

Required: `Authorization: Bearer <API_KEY>`

### 8.3 Response Structure

```json
{
  "data": [
    {
      "id": "openai/gpt-4o",
      "name": "OpenAI: GPT-4o",
      "description": "GPT-4o is OpenAI's flagship model...",
      "context_length": 128000,
      "architecture": {
        "modality": "text->text",
        "tokenizer": "GPT"
      },
      "pricing": {
        "prompt": "0.0000025",
        "completion": "0.00001"
      },
      "supported_parameters": [
        "temperature",
        "top_p",
        "max_tokens",
        "frequency_penalty",
        "presence_penalty",
        "stop"
      ],
      "default_parameters": {
        "temperature": 1.0,
        "max_tokens": 4096
      }
    }
  ]
}
```

### 8.4 Model Filtering

Models are included if they can process text input and produce text output.

Included modalities:
- `text->text` (pure text models)
- `text+image->text` (vision models - can also process text-only)
- `text+image+audio->text` (multimodal - can also process text-only)

Excluded modalities:
- `text->image` (image generation)
- `text->audio` (TTS)
- `audio->text` (STT)
- `image->text` (no text input)

### 8.5 Pricing Conversion

API returns cost per token as string. Convert to human-readable format:

```
"0.0000025" -> "$2.50 per 1M tokens"
"0.00001"   -> "$10.00 per 1M tokens"
"0"         -> "FREE"
```

Formula: `price_per_million = float(pricing) * 1_000_000`

---

## 9. Error Handling

### 9.1 Error Categories

| Category                   | Handling                    | Exit Code |
| -------------------------- | --------------------------- | --------- |
| Config parse error         | Return immediately          | 3         |
| Missing API token          | Return immediately          | 3         |
| Empty prompt               | Return immediately          | 3         |
| Models API error           | Return immediately          | 3         |
| Network error (connection) | Retry 3 times               | -         |
| HTTP 429 (rate limit)      | Retry 3 times with 5s delay | -         |
| HTTP 5xx (server error)    | Retry 3 times with 5s delay | -         |
| HTTP 4xx (client error)    | No retry, mark as failed    | -         |
| Timeout                    | Mark as failed              | -         |
| All models failed          | -                           | 2         |
| Partial failure            | -                           | 1         |
| All succeeded              | -                           | 0         |

### 9.2 Error Wrapping

Use `fmt.Errorf` with `%w` verb for error wrapping to preserve error chain.

### 9.3 Signal Handling

| Signal | Behavior |
|--------|----------|
| SIGINT (Ctrl+C) | Cancel all in-flight requests, output partial results if any, exit 0 |
| SIGTERM | Cancel all in-flight requests, output partial results if any, exit 0 |

---

## 10. Testing Strategy

### 10.1 Approach

- Table-driven tests for multiple scenarios
- Use `httptest.Server` for HTTP client testing (no external mocking libraries)
- Use stdlib `testing` package only (no testify)
- Test error paths: timeouts, retries, context cancellation
- Run with race detector: `go test -race ./...`

### 10.2 Coverage Target

Aim for >80% code coverage on `internal/` packages.

---

## 11. Build and Release

### 11.1 Build Targets

| OS | Architecture |
|----|--------------|
| linux | amd64, arm64 |
| darwin | amd64, arm64 |

### 11.2 Release Process

- Use GoReleaser for cross-compilation and release automation
- Static binaries (CGO_ENABLED=0)
- Version embedded via ldflags from git tags
- Archives in tar.gz format with checksums

---

## 12. Usage Examples

### 12.1 Initialize Configuration

```bash
# Interactive model selection (requires API token)
export OPENROUTER_API_KEY=sk-or-v1-...
orx init

# Or with explicit token (global flag)
orx --token sk-or-v1-... init

# Custom output path
orx init -o ~/my-orx-config.json
```

### 12.2 Basic Usage

```bash
# Single prompt via stdin
echo "Explain the difference between TCP and UDP" | orx

# Prompt from file
orx --prompt-file question.txt

# Custom config and timeout
orx -c ~/my-config.json -t 300 < prompt.txt
```

---

## Appendix A: OpenRouter API Reference

- **Chat Completions**: `POST https://openrouter.ai/api/v1/chat/completions`
- **List Models**: `GET https://openrouter.ai/api/v1/models`
- **Auth**: `Authorization: Bearer <API_KEY>`
- **Parameters**: https://openrouter.ai/docs/api/reference/parameters
- **Reasoning**: https://openrouter.ai/docs/use-cases/reasoning-tokens
- **Provider Routing**: https://openrouter.ai/docs/guides/routing/provider-selection
