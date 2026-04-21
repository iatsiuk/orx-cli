# Retry on Empty Content in API Response

## Overview
- Treat HTTP 200 responses with empty/whitespace `choices[0].message.content` as retryable errors
- Reuses existing retry machinery (maxRetries=3, retryableError, isRetryable)
- After retry exhaustion, the result is reported as an error instead of a fake success with empty content

**Problem**: a model can return HTTP 200 with a non-empty `choices` array but empty/null `content`. Current code sets `Status="success"` with `Content=""` and never retries. The user receives `null`/empty output saved to the result file.

**Observed**: session `playwright-cli/70acaa92-819c-4446-a041-5b28a32f48f5` - Gemini 3.1 Pro Preview returned `content: null` after 31.8s, saved as success with empty content, no retry attempted.

## Context (from discovery)
- Files/components involved:
  - `internal/client/client.go` - `parseResponse` (406-424), `Execute` retry loop (233-283), `retryableError` (426-436), `isRetryable` (438-468)
  - `internal/client/client_test.go` - `TestExecute_EmptyChoices` (259-287) serves as the template
  - `internal/testutil` - `NewTestServer` helper used by existing tests
- Related patterns found:
  - `parseResponse` already returns `retryableError{statusCode: 0, body: "no choices in response"}` when `len(result.Choices) == 0`
  - `Execute` loop retries any `retryableError` via `isRetryable` (lines 244-262)
  - Existing test `TestExecute_EmptyChoices` verifies 3 attempts + error status with "no choices" message
- Dependencies identified: none - `strings` package already imported in `client.go`

## Development Approach
- **Testing approach**: Regular (code first, then tests) - the change is 3 lines inside an existing function; writing code first and mirroring an existing test is more efficient than TDD here
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: add `TestExecute_EmptyContent` in `internal/client/client_test.go` mirroring `TestExecute_EmptyChoices`
  - `testutil.NewTestServer` responds with `Response{ID:"test", Choices:[{Message:{Content:""}}]}`
  - Assert `result.Status == "error"`, `strings.Contains(result.Error, "empty content")`, `attempts.Load() == 3`
  - Use `t.Parallel()`, `WithRetryDelay(0)`, `context.WithTimeout(5 * time.Second)`
- **E2E tests**: N/A - project has no UI, client tests cover the retry path

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## What Goes Where
- **Implementation Steps** (checkboxes): code change, new test, lint/test runs
- **Post-Completion** (no checkboxes): manual verification against real API, commit/push

## Implementation Steps

### Task 1: Add empty-content check to parseResponse
- [ ] in `internal/client/client.go`, inside `parseResponse` (after the `len(result.Choices) == 0` check at line 419-421), add:
  ```go
  if strings.TrimSpace(result.Choices[0].Message.Content) == "" {
      return nil, &retryableError{statusCode: 0, body: "empty content in response"}
  }
  ```
- [ ] add `TestExecute_EmptyContent` in `internal/client/client_test.go` mirroring `TestExecute_EmptyChoices` (lines 259-287):
  - server responds with `Response{ID:"test", Choices:[]Choice{{Message: ChoiceMessage{Content: ""}}}}`
  - assert `result.Status == "error"`
  - assert `strings.Contains(result.Error, "empty content")`
  - assert `attempts.Load() == 3` (retry exhaustion)
  - use `t.Parallel()`, `WithRetryDelay(0)`, 5s context timeout
- [ ] + optional: add a second sub-case with whitespace-only content (`"   \n\t"`) to confirm `TrimSpace` path - either as a table-driven variant or an inline second test
- [ ] run `go test -race ./internal/client/...` - all tests (existing + new) must pass
- [ ] run `make build` - golangci-lint must pass

### Task 2: Verify acceptance criteria
- [ ] verify `parseResponse` returns `retryableError` for empty content
- [ ] verify `Execute` retries 3 times and ends with `Status="error"` and `Error` containing `"empty content"`
- [ ] verify existing `TestExecute_EmptyChoices` still passes unchanged
- [ ] run full test suite (`go test -race ./...`)
- [ ] run `make build` - lint + build pass

## Technical Details

**Change site** - `internal/client/client.go`, `parseResponse`:

```go
func (c *Client) parseResponse(body []byte) (*Response, error) {
    var result Response
    if err := json.Unmarshal(body, &result); err != nil {
        return nil, &retryableError{statusCode: 0, body: fmt.Sprintf("unmarshal response: %s", err.Error())}
    }

    if result.Error != nil {
        if isRetryableAPIError(result.Error) {
            return nil, &retryableError{statusCode: http.StatusBadGateway, body: result.Error.Message}
        }
        return nil, fmt.Errorf("api error: %s", result.Error.Message)
    }

    if len(result.Choices) == 0 {
        return nil, &retryableError{statusCode: 0, body: "no choices in response"}
    }

    // NEW:
    if strings.TrimSpace(result.Choices[0].Message.Content) == "" {
        return nil, &retryableError{statusCode: 0, body: "empty content in response"}
    }

    return &result, nil
}
```

**Why TrimSpace**: a response with just whitespace/newlines is equally useless. Mirrors how a user would perceive the output.

**Why only `Choices[0]`**: `Execute` extracts `resp.Choices[0].Message.Content` at client.go:266. Checking the same index keeps the validation aligned with the actual consumption.

**Why ignore `Reasoning`**: in this codebase `Result.Reasoning` is only used for verbose/debug output (`-R` flag). The file written by `runner.go` contains only `Content`. Empty `Content` = unusable output regardless of reasoning.

## Post-Completion

**Manual verification** (optional, when convenient):
- Reproduce the original failure path by hitting a model that occasionally returns empty content (e.g., Gemini via OpenRouter); confirm retry kicks in and final error surfaces with `"empty content in response"` after 3 attempts.

**Git**:
- Branch: `fix/empty-content-retry` (no ticket available; use `fix/TICKET-123/empty-content-retry` if one exists)
- Commit message: `fix: retry when API returns empty content in choices`
- Follow project conventions: English, ASCII only, imports grouped (stdlib / external / local)

## Not in scope
- Changes to retry count, retry delay, or backoff strategy
- Changes to `runner.go`, request building, or config handling
- Handling of `tool_calls` or multi-choice responses
- Refactoring of `parseResponse` beyond the added check
- Changes to reasoning extraction logic
