# ORX - OpenRouter eXecutor

CLI tool for sending prompts to multiple LLM models via [OpenRouter API](https://openrouter.ai/) in parallel.

## Features

- Single binary, zero runtime dependencies
- Parallel execution to all enabled models
- Interactive TUI for model selection (`orx init`)
- JSON with Comments (JSONC) configuration
- Structured JSON output with cost tracking
- Retry with backoff for transient failures
- Verbose mode for HTTP debugging
- Graceful shutdown on SIGINT/SIGTERM

## Installation

### From source

```bash
git clone <repository-url>
cd orx-cli
make build
```

### From releases

Download binary from GitHub Releases page.

## Quick Start

1. Get an API key from [OpenRouter](https://openrouter.ai/)

2. Generate config (interactive TUI):
```bash
orx init
# Or use static template without TUI:
orx init --template
```

3. Set API key:
```bash
export OPENROUTER_API_KEY="sk-or-v1-..."
```

4. Run:
```bash
echo "Explain TCP vs UDP" | orx
```

## Usage

```
orx [flags] < prompt.txt
orx init [--output path] [--template]

Flags:
  -c, --config string       Path to config file (default: ~/.config/orx.json)
  -t, --timeout int         Global timeout in seconds (default: 600)
      --token string        OpenRouter API key (default: $OPENROUTER_API_KEY)
  -p, --prompt-file string  Read prompt from file instead of stdin
      --verbose             Dump HTTP request/response to stderr
  -h, --help                Show help
  -v, --version             Show version

Init Flags:
  -o, --output string       Output path (default: ~/.config/orx.json)
      --template            Generate static template without interactive TUI

Subcommands:
  init    Interactive model selection and configuration file generation
```

### Examples

```bash
# Prompt via stdin
echo "What is recursion?" | orx

# Prompt from file
orx -p question.txt

# Custom config and timeout
orx -c ~/custom.json -t 300 < prompt.txt

# Debug mode
orx --verbose < prompt.txt
```

## Configuration

Standard JSON config with optional `//` comments support. Default location: `~/.config/orx.json`

```jsonc
{
  // Global system prompt (optional)
  "system_prompt": "You are an expert assistant.",

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
      "enabled": true,
      "max_tokens": 8192
    },
    {
      "name": "deepseek-r1",
      "model": "deepseek/deepseek-r1",
      "enabled": false,
      "include_reasoning": true,  // Include thinking in response
      "max_tokens": 8192
    }
  ]
}
```

### Model Parameters

| Parameter | Description | Range |
|-----------|-------------|-------|
| `temperature` | Controls randomness | 0.0-2.0 |
| `top_p` | Nucleus sampling | 0.0-1.0 |
| `top_k` | Limits token choices | 0+ |
| `max_tokens` | Maximum response length | 1+ |
| `frequency_penalty` | Reduces repetition | -2.0 to 2.0 |
| `presence_penalty` | Encourages new topics | -2.0 to 2.0 |

See [OpenRouter docs](https://openrouter.ai/docs/api/reference/parameters) for full parameter list.

### Reasoning Models

For models with reasoning support (o1, o3, claude-3.7, deepseek-r1):

```jsonc
{
  "name": "o1",
  "model": "openai/o1",
  "enabled": true,
  "reasoning": {
    "effort": "high",     // "low" | "medium" | "high"
    "exclude": false      // Hide reasoning from response
  },
  "include_reasoning": true
}
```

## Output Format

JSON output to stdout:

```json
{
  "results": [
    {
      "name": "gpt-4o",
      "status": "success",
      "content": "Response text...",
      "duration_ms": 2345,
      "cost": 0.00125
    },
    {
      "name": "claude-sonnet",
      "status": "error",
      "error": "timeout after 600s",
      "duration_ms": 600000
    }
  ],
  "total_duration_ms": 600000,
  "total_cost": 0.00125,
  "successful": 1,
  "failed": 1
}
```

Progress output to stderr:
```
gpt-4o - [requesting]
claude-sonnet - [requesting]
gpt-4o - [done] (2.3s)
claude-sonnet - [error]
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (all models succeeded or user cancelled) |
| 1 | Partial failure (at least one succeeded) |
| 2 | All models failed |
| 3 | Configuration or input error |

## License

MIT
