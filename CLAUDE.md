# ORX Project Instructions

## Code Style

### Imports

Group imports in order, separated by blank lines:
1. Standard library
2. External packages
3. Local packages (`orx/...`)

```go
import (
    "context"
    "fmt"

    "golang.org/x/sync/errgroup"

    "orx/internal/client"
)
```

### Naming

- Package names: short, lowercase, no underscores (`config`, `client`, `runner`)
- Exported types: PascalCase (`Config`, `Model`, `Runner`)
- Unexported: camelCase (`validateModel`, `buildRequest`)
- Acronyms: consistent case (`URL`, `HTTP`, `API` or `url`, `http`, `api`)
- Receivers: short, 1-2 letters (`c` for `*Client`, `r` for `*Runner`)
- Errors: `Err` prefix for sentinel errors (`ErrNoEnabledModels`)

### Functions

- Max 80 lines, 50 statements (enforced by `funlen` linter)
- Max cyclomatic complexity: 10 (enforced by `cyclop` linter)
- Max nesting depth: 5 (enforced by `nestif` linter)
- Early returns for error handling
- Group related functions together

### Error Handling

- Wrap errors with context: `fmt.Errorf("operation: %w", err)`
- Check all errors (enforced by `errcheck` linter)
- Use `errors.Is`/`errors.As` for error comparison
- Sentinel errors as package-level variables

### Comments

- Only for non-obvious logic
- English, lowercase, brief
- No comments for self-explanatory code
- Package comment in `doc.go` if needed

### Structs

- JSON tags on all exported fields: `json:"field_name"`
- Use `omitempty` for optional fields
- Pointer types for optional values (`*float64`, `*int`)
- Group related fields together

```go
type Model struct {
    Name    string   `json:"name"`
    Model   string   `json:"model"`
    Enabled bool     `json:"enabled"`
    MaxTokens *int   `json:"max_tokens,omitempty"`
}
```

### Variables

- Package-level constants in `const` block
- Related constants grouped together
- Unexported package variables with `var`

```go
const (
    defaultBaseURL = "https://api.example.com"
    maxRetries     = 3
    retryDelay     = 5 * time.Second
)

var retryablePatterns = []string{"stream error", "connection reset"}
```

### Control Flow

- Use `range` with index for modifying slices
- Prefer `for i := range n` over `for i := 0; i < n; i++` (Go 1.22+)
- Use `switch` over long `if-else` chains

### Concurrency

- Use `context.Context` as first parameter
- Use `sync.Mutex` for simple locking
- Use `errgroup` for parallel operations
- Use `atomic` for counters

## Testing

Follow patterns from `best-practices-for-testing.md` when writing tests:
- Use table-driven tests for multiple scenarios
- Use `httptest.NewServer` for HTTP client testing (no external mocking libraries)
- Use stdlib `testing` package only (no testify) - aligns with "zero dependencies" goal
- Test error paths: timeouts, retries, context cancellation
- Run with race detector: `go test -race ./...`
- Use `t.Parallel()` for independent tests
- Test files: `*_test.go` in same package

## Language

All documentation, comments, and text must be in English.

## Building

- Always build with `make build` (runs linter automatically)
- Direct `go build` skips linting - avoid it

## Linting and Formatting

- Run `golangci-lint run` before committing (executed automatically via `make build`)
- Fix formatting issues with `goimports -w <file>` or `gofmt -w <file>`
- Config: `.golangci.yml` defines enabled linters
- Key linters: goimports, govet, errcheck, staticcheck, unused
- No trailing whitespace, proper import grouping (stdlib, external, local)

## Architecture Notes

### TUI testing pattern

TUI screens (tview-based) are not unit-testable directly. Extract business logic as pure functions
and test those; mark tview integration as "manual verification only". Example: `nextEffort`,
`filterReasoningSelectedModels`, `applyEfforts`, `cycleEffort` are pure functions tested directly;
`newReasoningTuiApp` + `run()` are verified manually.

### Config generation merge priority (enabled models)

1. `ReasoningEffort` from TUI (highest) - always controls "reasoning" param exclusively
2. `DefaultParameters` from API - overrides ExistingParams for all other params
3. `ExistingParams` from previous config (lowest) - baseline for params not in DefaultParameters

Disabled models: only ExistingParams emitted (no API defaults, no TUI input).
When user skips reasoning TUI (Esc), "reasoning" goes to `// available:` comment regardless
of ExistingParams.

## Working with todo.md

- Mark completed items with `[x]`: `- [x] Completed task`
- Keep incomplete items as `[ ]`: `- [ ] Pending task`
- Update todo.md immediately after completing each task
