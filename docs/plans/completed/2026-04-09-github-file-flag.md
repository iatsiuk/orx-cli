# GitHub File Flag (`--github-file`)

## Overview
- Add `--github-file` flag to download files from GitHub by URL and attach them to the prompt
- Works the same as local `-f` files but fetches content via GitHub Contents API
- Auth via `GITHUB_TOKEN` env var, supports blob and raw URL formats

## Context (from discovery)
- Files/components involved:
  - `internal/files/files.go` - file loading, `formatFile` (needs export)
  - `internal/files/binary.go` - `IsBinary` check (already exported)
  - `cmd/orx/main.go` - CLI flags, `options` struct, `run()`, `appendFileContent()`
- Related patterns found:
  - `-f/--file` flag with `files.LoadContent()` pipeline (binary check, size check, token estimation)
  - HTTP client in `internal/client/client.go` (Bearer auth, User-Agent, error handling)
  - Parallel execution via `errgroup` in `internal/runner/runner.go`
- Dependencies identified: no new Go dependencies needed (stdlib `net/http` + `net/url`)

## Development Approach
- **Testing approach**: TDD (tests first)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: table-driven tests for URL parsing, httptest.NewServer for API calls
- **Integration**: manual verification with real GitHub URLs

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: ParseURL tests and implementation
- [x] create `internal/github/github_test.go` with table-driven tests for ParseURL
  - valid: blob URL with scheme, blob URL without scheme, raw URL, nested path, ref with dots/tags
  - invalid: wrong host, missing path, empty segments, unsupported type (`tree`), too few segments
- [x] create `internal/github/github.go` with ParseURL implementation
  - strip optional `https://` scheme
  - split by `/`, expect: `github.com/{owner}/{repo}/{blob|raw}/{ref}/{path...}`
  - accept both `blob` and `raw` as the type segment
  - path is everything after ref joined with `/`
  - error on missing components, non-`github.com` host, unsupported type
- [x] run tests - must pass before next task

### Task 2: FetchFile tests and implementation
- [x] add table-driven tests for FetchFile in `internal/github/github_test.go`
  - httptest.NewServer for 200 (returns content), 404 (file not found), 403 (access denied)
  - verify Authorization header sent correctly
  - verify Accept header is `application/vnd.github.raw`
  - context cancellation test
- [x] implement FetchFile in `internal/github/github.go`
  - build URL: `https://api.github.com/repos/{owner}/{repo}/contents/{path}?ref={ref}`
  - set headers: `Authorization: Bearer {token}`, `Accept: application/vnd.github.raw`, `User-Agent: orx-cli`
  - return body bytes on 200
  - descriptive errors for 404, 403, other status codes
- [x] run tests - must pass before next task

### Task 3: Export FormatFile
- [x] rename `formatFile` to `FormatFile` in `internal/files/files.go`
- [x] update call site in `LoadContent`
- [x] run tests - must pass before next task

### Task 4: Wire `--github-file` flag and `appendGitHubFiles` into CLI
- [x] add `githubFiles []string` to `options` struct in `cmd/orx/main.go`
- [x] register flag: `rootCmd.Flags().StringArrayVar(&opts.githubFiles, "github-file", nil, ...)`
- [x] implement `appendGitHubFiles(ctx, prompt, opts)` function:
  - return early if no github files
  - read `GITHUB_TOKEN` env var, hard error if empty
  - parse all URLs with `github.ParseURL`
  - download files in parallel via `errgroup` (respects ctx timeout)
  - for each: check binary, check size (`--max-file-size`), format with `files.FormatFile`
  - return formatted string appended to prompt
- [x] wire into `run()` after `appendFileContent` (move ctx creation before file loading)
- [x] run tests - must pass before next task

### Task 5: Verify acceptance criteria
- [x] verify ParseURL handles blob and raw URLs with/without scheme
- [x] verify FetchFile sends correct headers and handles error responses
- [x] verify `--github-file` flag works alongside `-f` (both local and remote files in same prompt)
- [x] verify missing `GITHUB_TOKEN` produces clear error
- [x] run full test suite (`go test -race ./...`)
- [x] run linter (`make build`)

## Technical Details

**URL parsing**: `github.com/{owner}/{repo}/{blob|raw}/{ref}/{path...}` -> GitHub Contents API call

**API request**:
```
GET https://api.github.com/repos/{owner}/{repo}/contents/{path}?ref={ref}
Authorization: Bearer {GITHUB_TOKEN}
Accept: application/vnd.github.raw
User-Agent: orx-cli
```

**File processing pipeline** (same as local files):
1. Download raw content
2. Check binary (IsBinary on first 8KB)
3. Check size (--max-file-size)
4. Format with FormatFile(url, content)
5. Check total token limit (--max-tokens)

## Post-Completion

**Manual verification**:
- Test with real public GitHub repo URL
- Test with real private repo URL (requires valid GITHUB_TOKEN)
- Test error messages for invalid URLs, missing token, 404 files

## Not in scope
- Caching downloaded files
- GitHub Enterprise support
- Directory or glob downloads
- `--github-token` CLI flag (env var only)
- Retry logic for GitHub API (single attempt is fine for interactive CLI)
