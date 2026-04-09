package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// FileRef holds parsed components of a GitHub file URL.
type FileRef struct {
	Owner string
	Repo  string
	Ref   string
	Path  string
}

// ParseURL parses a GitHub blob or raw file URL into its components.
// Accepts URLs with or without the https:// scheme.
// Format: github.com/{owner}/{repo}/{blob|raw}/{ref}/{path...}
func ParseURL(rawURL string) (FileRef, error) {
	u := strings.TrimPrefix(rawURL, "https://")
	u = strings.TrimPrefix(u, "http://")

	// strip fragment and query string before splitting (e.g. #L42 line anchors)
	u, _, _ = strings.Cut(u, "#")
	u, _, _ = strings.Cut(u, "?")

	parts := strings.Split(u, "/")
	// need at least: github.com, owner, repo, type, ref, path
	if len(parts) < 6 {
		return FileRef{}, fmt.Errorf("invalid github URL %q: too few path segments", rawURL)
	}

	host := parts[0]
	if host != "github.com" {
		return FileRef{}, fmt.Errorf("invalid github URL %q: expected github.com host, got %q", rawURL, host)
	}

	owner := parts[1]
	repo := parts[2]
	typ := parts[3]
	ref := parts[4]
	path := strings.Join(parts[5:], "/")

	if typ != "blob" && typ != "raw" {
		return FileRef{}, fmt.Errorf("invalid github URL %q: unsupported type %q (expected blob or raw)", rawURL, typ)
	}

	if owner == "" || repo == "" || ref == "" || path == "" {
		return FileRef{}, fmt.Errorf("invalid github URL %q: missing required components", rawURL)
	}

	return FileRef{Owner: owner, Repo: repo, Ref: ref, Path: path}, nil
}

// FetchFile downloads the raw content of a GitHub file via the Contents API.
func FetchFile(ctx context.Context, ref FileRef, token string) ([]byte, error) {
	return fetchFileWithBase(ctx, ref, token, "https://api.github.com")
}

// reencodePathSegment decodes a potentially already-encoded path segment and
// re-encodes it cleanly to avoid double-encoding (e.g. %20 -> %2520).
func reencodePathSegment(s string) string {
	if decoded, err := url.PathUnescape(s); err == nil {
		return url.PathEscape(decoded)
	}
	return url.PathEscape(s)
}

// reencodeQueryValue decodes a potentially already-encoded query value and
// re-encodes it cleanly to avoid double-encoding.
func reencodeQueryValue(s string) string {
	if decoded, err := url.QueryUnescape(s); err == nil {
		return url.QueryEscape(decoded)
	}
	return url.QueryEscape(s)
}

func fetchFileWithBase(ctx context.Context, ref FileRef, token, baseURL string) ([]byte, error) {
	// escape each path segment individually to preserve slash separators;
	// unescape first to avoid double-encoding already-encoded segments (e.g. %20 -> %2520)
	pathParts := strings.Split(ref.Path, "/")
	for i, p := range pathParts {
		pathParts[i] = reencodePathSegment(p)
	}
	apiURL := fmt.Sprintf(
		"%s/repos/%s/%s/contents/%s?ref=%s",
		baseURL,
		url.PathEscape(ref.Owner),
		url.PathEscape(ref.Repo),
		strings.Join(pathParts, "/"),
		reencodeQueryValue(ref.Ref),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.raw")
	req.Header.Set("User-Agent", "orx-cli")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s/%s/%s: %w", ref.Owner, ref.Repo, ref.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	return parseGitHubResponse(resp, ref)
}

// 100 MB safety cap to prevent OOM on unexpectedly large responses
const maxReadSize = 100 * 1024 * 1024

func parseGitHubResponse(resp *http.Response, ref FileRef) ([]byte, error) {
	switch resp.StatusCode {
	case http.StatusOK:
		return readLimited(resp.Body, ref)
	case http.StatusNotFound:
		return nil, fmt.Errorf("file not found: %s/%s/%s@%s", ref.Owner, ref.Repo, ref.Path, ref.Ref)
	case http.StatusForbidden, http.StatusUnauthorized:
		return nil, fmt.Errorf("access denied to %s/%s/%s (check GITHUB_TOKEN)", ref.Owner, ref.Repo, ref.Path)
	default:
		return nil, fmt.Errorf("unexpected status %d for %s/%s/%s", resp.StatusCode, ref.Owner, ref.Repo, ref.Path)
	}
}

func readLimited(r io.Reader, ref FileRef) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxReadSize+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > maxReadSize {
		return nil, fmt.Errorf("response too large for %s/%s/%s (exceeds 100MB)", ref.Owner, ref.Repo, ref.Path)
	}
	return body, nil
}
