# Add orx-ralphex-review script

## Overview
- Ship `orx-ralphex-review` shell script alongside the `orx` binary
- Compatible with ralphex `custom_review_script` contract: receives single arg (prompt file path)
- Extracts `git diff` command from prompt, executes it, passes diff + prompt to orx
- Formats orx JSON output as plain text via jq
- No Go code needed -- reuses existing `orx` binary
- Dependency: `jq` (widely available, documented as requirement)

## Context (from discovery)

### ralphex contract (verified against code)
- `exec.Command(script, promptFile)` -- single arg: path to prompt file (custom.go:29)
- Prompt has marker "Run this command to see the changes:" followed by git diff command
- Script must: (1) parse prompt to extract diff command, (2) execute it, (3) pass diff + prompt to LLMs
- Script output is raw text passed to Claude for evaluation (runner.go:660,672)
- ralphex does NOT require signal from custom script -- signal comes from Claude eval (runner.go:689-693)
- ralphex merges stderr into stdout (`cmd.Stderr = cmd.Stdout` in custom.go:39)
- ralphex preserves output even on non-zero exit (output NOT discarded, custom.go:118-136)
- ralphex env: filtered via `filterEnv()`, `OPENROUTER_API_KEY` passes through (executor.go:66)
- Prompt may contain PREVIOUS REVIEW CONTEXT on iterations (prompts.go:200-217) -- empty diff is valid
- Output format is irrelevant to ralphex -- it passes raw stdout to Claude without parsing

### Reference implementation
- Existing working script: `/Users/me/Desktop/orx-review.sh`
- Extracts diff command via awk, validates it starts with `git diff`
- Runs `git diff` to temp file, passes via `orx -p <prompt> -f <diff> --max-file-size 512KB`
- Formats JSON output via jq
- Handles empty diff (PREVIOUS REVIEW CONTEXT flow)

### Known issues in reference script (from expert review + fact-check)
- stderr not suppressed: orx progress output leaks into ralphex stdout (CRITICAL)
- pipefail + orx exit codes: orx exits 1/2 for partial/total model failures, pipefail kills wrapper even when JSON is valid (HIGH)
- missing `--no-ext-diff --no-textconv` on git diff: allows `.gitattributes` to trigger external commands (MEDIUM)
- `mktemp -t` portability: behaves differently on macOS vs GNU/Linux (LOW)
- `${DIFF_ARGS[1]}` unbound under `set -u` when extracted command has < 2 tokens (LOW)

## Development Approach
- Copy and adapt existing working script into the repo
- Apply fixes from expert review before shipping
- Shell script tests via bash assertions
- Validate goreleaser packaging with snapshot build

## Progress Tracking
- Mark completed items with `[x]` immediately when done

## Implementation Steps

### Task 1: Add script to repo
- [x] copy `/Users/me/Desktop/orx-review.sh` to `scripts/orx-ralphex-review`
- [x] rename, remove `.sh` extension (installed as binary name)
- [x] add shebang `#!/bin/bash`, ensure `set -euo pipefail`
- [x] fix stderr: capture orx output to temp files (`>json 2>err`), not pipe, to isolate stderr from stdout
- [x] fix pipefail: don't pipe orx to jq directly; capture JSON to file, then format separately
- [x] fix git diff safety: add `--no-ext-diff --no-textconv` flags to git diff invocation
- [x] fix mktemp portability: use `mktemp "${TMPDIR:-/tmp}/ralphex-orx-diff.XXXXXX"` instead of `mktemp -t`
- [x] fix unbound variable: guard `${DIFF_ARGS[1]}` access with array length check
- [x] verify script is executable (`chmod +x`)
- [x] run script manually against a test prompt file

### Task 2: Update build configuration
- [x] add `extra_files` in `.goreleaser.yaml` archives section to include the script
- [x] add second `binary` stanza in `homebrew_casks` for `orx-ralphex-review`
- [x] validate with `goreleaser release --snapshot --clean`
- [x] inspect produced archive -- must contain both `orx` and `orx-ralphex-review`
- [x] inspect generated cask -- must install both executables

### Task 3: Verify acceptance criteria
- [ ] verify ralphex compatibility: script accepts exactly 1 positional arg
- [ ] verify diff extraction: parses prompt, executes git diff, passes as context
- [ ] verify output format: plain text, no JSON, no signal
- [ ] verify orx stderr suppressed (no progress in output)
- [ ] verify empty diff case works (PREVIOUS REVIEW CONTEXT)
- [ ] verify missing jq produces clear error message
- [ ] verify orx partial failure (exit 1) still produces formatted output
- [ ] verify git diff uses --no-ext-diff --no-textconv

### Task 4: [Final] Update documentation
- [ ] update README.md with ralphex integration section
- [ ] document: requires `jq` as dependency
- [ ] document config example: `custom_review_script = orx-ralphex-review`

## Technical Details

### Script interface
```
orx-ralphex-review <prompt-file-path>
```
Uses orx config from default location (`~/.config/orx.json`) and `OPENROUTER_API_KEY` env var.

### Execution flow
1. Validate single positional arg, file exists
2. Extract git diff command from prompt (after "Run this command to see the changes:" marker)
3. Validate command starts with `git diff` (with array length guard), fallback to `git diff` if no marker
4. Execute `git -c color.ui=false diff --no-ext-diff --no-textconv <args>`, save to temp file
5. Run `orx -p <prompt> -f <diff> --max-file-size 512KB` with stdout to JSON file, stderr to err file
6. Format JSON output via jq: extract model responses as plain text
7. On jq failure: emit error with captured stderr content

### Output format
```
=== model-name-1 ===
- file:line - description of issue

=== model-name-2 ===
- file:line - description of issue
```

### Exit codes
- 0: success (formatted output produced)
- 1: arg validation error
- 2: invalid diff command extracted
- 3: git diff execution failed
- 4: orx/jq failure (no parseable output)

### Stderr handling
orx stderr captured to temp file (not /dev/null) because ralphex merges stderr into stdout.
On orx/jq failure, captured stderr is emitted for debugging.
Script's own validation errors go to stderr (script exits before orx runs).

### Dependencies
- `orx` (must be on PATH)
- `jq` (for JSON parsing)
- `git` (for diff execution)
- `awk` (for prompt parsing, POSIX standard)

### ralphex config
```ini
external_review_tool = custom
custom_review_script = orx-ralphex-review
```

## Post-Completion
- Manual verification: configure ralphex with `custom_review_script = orx-ralphex-review` and run a real review
- Validate packaging: `goreleaser release --snapshot --clean`, inspect archive and cask
- Verify `brew install` puts both `orx` and `orx-ralphex-review` on PATH
