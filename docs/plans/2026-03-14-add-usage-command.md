# Add `orx usage` command

## Overview
- Add `orx usage` subcommand that calls OpenRouter `/api/v1/key` endpoint
- Displays API key usage info: total/daily/weekly/monthly spend, limit, remaining balance
- Human-readable formatted output to stdout
- Reuses existing `Client` from `internal/client` (token + httpClient already available)

## Context (from discovery)
- Files/components involved:
  - `internal/client/client.go` - add `KeyInfo()` method + response types
  - `internal/client/client_test.go` - tests for new method
  - `cmd/orx/main.go` - register `usage` cobra subcommand
  - `cmd/orx/main_test.go` - tests for command integration
- Related patterns found:
  - `Client` struct with `WithBaseURL` option for testability
  - `testutil.NewTestServer` for HTTP mocking
  - Cobra subcommands registered via `rootCmd.AddCommand()`
  - `--token` is a PersistentFlag (available to subcommands)
  - `--verbose` is a PersistentFlag (available to subcommands)
- Base URL: current `defaultBaseURL` points to `/chat/completions`, new endpoint is `/api/v1/key`

## Development Approach
- **Testing approach**: TDD (tests first)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task (see Development Approach above)
- Use `testutil.NewTestServer` for HTTP mocking (existing pattern)
- Table-driven tests for multiple scenarios
- Test error paths: HTTP errors, invalid JSON, missing fields

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Write tests for `KeyInfo()` client method
- [x] define `KeyInfoResponse` and `KeyInfoData` types in `client.go` (structs only, no logic)
- [x] write test `TestKeyInfo_Success` - mock server returns full response, verify all fields parsed
- [x] write test `TestKeyInfo_Unauthorized` - mock server returns 401, verify error
- [x] write test `TestKeyInfo_InvalidJSON` - mock server returns garbage, verify error
- [x] write test `TestKeyInfo_Verbose` - verify verbose output includes request/response dump
- [x] write test `TestKeyInfo_ContextCancellation` - verify context propagation
- [x] run tests - must pass (they will fail until Task 2)

### Task 2: Implement `KeyInfo()` client method
- [x] add `keyInfoURL()` helper that derives `/api/v1/key` from `baseURL` (or use separate base)
- [x] implement `KeyInfo(ctx context.Context) (*KeyInfoResponse, error)` method on `Client`
- [x] handle HTTP errors (non-200 status codes)
- [x] handle JSON parsing
- [x] support verbose mode (dump request/response)
- [x] run tests - all Task 1 tests must pass

### Task 3: Write tests for `usage` cobra subcommand
- [x] write test `TestUsageCmd_Success` - mock server, verify formatted output on stdout
- [x] write test `TestUsageCmd_MissingToken` - verify ErrTokenRequired
- [x] write test `TestUsageCmd_APIError` - mock server returns error, verify error propagated
- [x] write test `TestUsageCmd_WithLimit` - verify limit/remaining displayed when present
- [x] write test `TestUsageCmd_NoLimit` - verify output when limit is null
- [x] run tests - must pass (they will fail until Task 4)

### Task 4: Implement `usage` cobra subcommand and formatter
- [x] add `formatKeyInfo(*KeyInfoData) string` function for human-readable output
- [x] register `usage` subcommand in `newRootCmd()` with `RunE` handler
- [x] handler: validate token, create client, call `KeyInfo()`, format and print
- [x] run tests - all Task 3 tests must pass

### Task 5: Verify acceptance criteria
- [x] verify all requirements from Overview are implemented
- [x] verify edge cases are handled (nil limit, nil limit_remaining, free tier)
- [x] run full test suite (`go test -race ./...`)
- [x] run linter (`golangci-lint run`) - all issues must be fixed
- [x] `make build` passes

## Technical Details

### API Endpoint
```
GET https://openrouter.ai/api/v1/key
Authorization: Bearer <API_KEY>
```

### Response Structure
```json
{
  "data": {
    "label": "my-key",
    "limit": 10.0,
    "limit_reset": "2026-04-01T00:00:00Z",
    "limit_remaining": 7.5,
    "include_byok_in_limit": false,
    "usage": 2.5,
    "usage_daily": 0.3,
    "usage_weekly": 1.2,
    "usage_monthly": 2.5,
    "byok_usage": 0.0,
    "byok_usage_daily": 0.0,
    "byok_usage_weekly": 0.0,
    "byok_usage_monthly": 0.0,
    "is_free_tier": false
  }
}
```

### Go Types
```go
type KeyInfoResponse struct {
    Data KeyInfoData `json:"data"`
}

type KeyInfoData struct {
    Label             string   `json:"label"`
    Limit             *float64 `json:"limit"`
    LimitReset        *string  `json:"limit_reset"`
    LimitRemaining    *float64 `json:"limit_remaining"`
    IncludeBYOKInLimit bool    `json:"include_byok_in_limit"`
    Usage             float64  `json:"usage"`
    UsageDaily        float64  `json:"usage_daily"`
    UsageWeekly       float64  `json:"usage_weekly"`
    UsageMonthly      float64  `json:"usage_monthly"`
    IsFreeTier        bool     `json:"is_free_tier"`
}
```

### Output Format
```
API Key:  my-key
Tier:     paid

Usage:
  Total:   $2.50
  Daily:   $0.30
  Weekly:  $1.20
  Monthly: $2.50

Limit:     $10.00
Remaining: $7.50
```

### Base URL Handling
The existing `baseURL` field points to `.../chat/completions`. For `/api/v1/key`:
- Derive base from `baseURL` by trimming `/chat/completions` suffix
- Or: store API base separately (e.g. `https://openrouter.ai/api/v1`)
- In tests: `server.URL` works directly since we control the handler path
