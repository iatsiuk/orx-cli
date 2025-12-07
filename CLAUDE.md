# ORX Project Instructions

## Project Files

- `specification.md` - full technical specification for the ORX CLI tool
- `tui-init.md` - implementation plan for `orx init` TUI model selector
- `todo.md` - implementation checklist with all tasks
- `best-practices-for-testing.md` - testing guidelines and patterns

## Testing

Follow patterns from `best-practices-for-testing.md` when writing tests:
- Use table-driven tests for multiple scenarios
- Use `httptest.NewServer` for HTTP client testing (no external mocking libraries)
- Use stdlib `testing` package only (no testify) - aligns with "zero dependencies" goal
- Test error paths: timeouts, retries, context cancellation
- Run with race detector: `go test -race ./...`

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

## Working with todo.md

- Mark completed items with `[x]`: `- [x] Completed task`
- Keep incomplete items as `[ ]`: `- [ ] Pending task`
- Update todo.md immediately after completing each task
