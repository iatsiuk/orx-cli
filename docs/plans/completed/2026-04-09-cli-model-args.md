# CLI model arguments support

## Overview
- Allow `orx` to run without a config file by passing model IDs and reasoning effort directly via CLI arguments
- Supports multiple models with per-model reasoning effort via `@` syntax: `-m model@effort`
- `-m` and `-c` flags are mutually exclusive

## Context (from discovery)
- Files/components involved: `cmd/orx/main.go`, `cmd/orx/main_test.go`, `internal/config/config.go`
- Related patterns found: cobra flags, `StringArrayVarP` for repeatable flags (see `--file`), `config.Model` struct, `config.ReasoningConfig` struct
- Dependencies identified: reuses existing `config.Model` and `config.ReasoningConfig` types as-is

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: table-driven tests for `parseModelFlag` and `buildCLIModels`
- **Integration tests**: cobra command execution with `-m` flags (follows existing test patterns in `cmd/orx/main_test.go`)

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix
- Update plan if implementation deviates from original scope

## Technical Details

### Syntax

```
-m, --model  (repeatable)  model ID with optional reasoning effort via "@"
```

Examples:

```bash
echo "hello" | orx -m anthropic/claude-sonnet
echo "hello" | orx -m anthropic/claude-sonnet@medium -m deepseek/deepseek-r1@high
echo "hello" | orx -m anthropic/claude-sonnet@low -m openai/gpt-4o
```

### Parsing rules

`strings.Cut(value, "@")` splits model value:
- Left part: model ID (required, must not be empty)
- Right part: reasoning effort (optional)
- Valid efforts: `none`, `minimal`, `low`, `medium`, `high`, `xhigh`
- Display name auto-generated from model ID: part after `/` (or full ID if no `/`)

### Constraints

- `-m` and `-c` are mutually exclusive (return error if both provided)
- When `-m` is provided, config file is not loaded
- When `-m` is not provided, behavior unchanged (config required)
- API token still required via `--token` or `$OPENROUTER_API_KEY`

## Implementation Steps

### Task 1: Add parseModelFlag and buildCLIModels functions
- [x] add `parseModelFlag(value string) (config.Model, error)` to `cmd/orx/main.go` - splits on `@`, validates effort, builds `config.Model` with auto-generated name
- [x] add `buildCLIModels(flags []string) ([]config.Model, error)` to `cmd/orx/main.go` - iterates flags, calls parseModelFlag, collects results
- [x] write tests for `parseModelFlag`: model without effort, model with effort, invalid effort, empty model ID, no `/` in model ID
- [x] write tests for `buildCLIModels`: multiple valid models, one invalid among valid, empty slice
- [x] run tests - must pass before next task

### Task 2: Wire -m flag into root command and run()
- [x] add `models []string` field to `options` struct
- [x] register `-m, --model` flag via `StringArrayVarP` on rootCmd
- [x] modify `run()`: if `opts.models` non-empty and `opts.configPath` non-empty, return mutual exclusion error
- [x] modify `run()`: if `opts.models` non-empty, call `buildCLIModels` instead of `config.Load`
- [x] write test: `-m` and `-c` together returns error
- [x] write test: `-m` with valid model builds config and proceeds (use httptest server)
- [x] run tests - must pass before next task

### Task 3: Verify acceptance criteria
- [x] verify: `orx -m model` works without config file
- [x] verify: `orx -m model@effort` sets reasoning correctly
- [x] verify: `orx -m a -m b` passes multiple models to runner
- [x] verify: `orx -m a -c config.json` returns error
- [x] verify: `orx` without `-m` still requires config (backward compatible)
- [x] run full test suite (`go test -race ./...`)
- [x] run linter via `make build`

## Post-Completion

**Manual verification**:
- Test with real OpenRouter API: single model, multiple models, with reasoning effort
- Verify error messages are clear for invalid model syntax
