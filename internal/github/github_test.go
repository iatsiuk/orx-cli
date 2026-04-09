package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    FileRef
		wantErr bool
	}{
		{
			name:  "blob URL with scheme",
			input: "https://github.com/owner/repo/blob/main/file.go",
			want:  FileRef{Owner: "owner", Repo: "repo", Ref: "main", Path: "file.go"},
		},
		{
			name:  "blob URL without scheme",
			input: "github.com/owner/repo/blob/main/file.go",
			want:  FileRef{Owner: "owner", Repo: "repo", Ref: "main", Path: "file.go"},
		},
		{
			name:  "raw URL",
			input: "https://github.com/owner/repo/raw/main/file.go",
			want:  FileRef{Owner: "owner", Repo: "repo", Ref: "main", Path: "file.go"},
		},
		{
			name:  "nested path",
			input: "https://github.com/owner/repo/blob/main/path/to/file.go",
			want:  FileRef{Owner: "owner", Repo: "repo", Ref: "main", Path: "path/to/file.go"},
		},
		{
			name:  "ref with dots (tag)",
			input: "https://github.com/owner/repo/blob/v1.2.3/file.go",
			want:  FileRef{Owner: "owner", Repo: "repo", Ref: "v1.2.3", Path: "file.go"},
		},
		{
			name:  "ref with slash-like tag",
			input: "github.com/owner/repo/blob/release/v2/cmd/main.go",
			want:  FileRef{Owner: "owner", Repo: "repo", Ref: "release", Path: "v2/cmd/main.go"},
		},
		{
			name:  "URL with line anchor stripped",
			input: "https://github.com/owner/repo/blob/main/file.go#L42",
			want:  FileRef{Owner: "owner", Repo: "repo", Ref: "main", Path: "file.go"},
		},
		{
			name:  "URL with line range anchor stripped",
			input: "https://github.com/owner/repo/blob/main/file.go#L10-L20",
			want:  FileRef{Owner: "owner", Repo: "repo", Ref: "main", Path: "file.go"},
		},
		{
			name:  "URL with query string stripped",
			input: "https://github.com/owner/repo/blob/main/file.go?plain=1",
			want:  FileRef{Owner: "owner", Repo: "repo", Ref: "main", Path: "file.go"},
		},
		{
			name:    "wrong host",
			input:   "https://gitlab.com/owner/repo/blob/main/file.go",
			wantErr: true,
		},
		{
			name:    "missing path segment",
			input:   "https://github.com/owner/repo/blob/main",
			wantErr: true,
		},
		{
			name:    "unsupported type tree",
			input:   "https://github.com/owner/repo/tree/main/dir",
			wantErr: true,
		},
		{
			name:    "too few segments",
			input:   "https://github.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "empty owner",
			input:   "github.com//repo/blob/main/file.go",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseURL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("ParseURL(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFetchFile(t *testing.T) {
	t.Parallel()

	ref := FileRef{Owner: "owner", Repo: "repo", Ref: "main", Path: "file.go"}

	tests := []struct {
		name       string
		statusCode int
		body       string
		token      string
		wantErr    bool
		wantBody   string
		checkAuth  bool
	}{
		{
			name:       "200 returns content",
			statusCode: http.StatusOK,
			body:       "package main\n",
			token:      "mytoken",
			wantBody:   "package main\n",
		},
		{
			name:       "404 file not found error",
			statusCode: http.StatusNotFound,
			body:       `{"message":"Not Found"}`,
			token:      "mytoken",
			wantErr:    true,
		},
		{
			name:       "403 access denied error",
			statusCode: http.StatusForbidden,
			body:       `{"message":"Forbidden"}`,
			token:      "",
			wantErr:    true,
		},
		{
			name:       "500 unexpected status error",
			statusCode: http.StatusInternalServerError,
			body:       "internal error",
			token:      "mytoken",
			wantErr:    true,
		},
		{
			name:       "authorization header sent",
			statusCode: http.StatusOK,
			body:       "content",
			token:      "secrettoken",
			checkAuth:  true,
			wantBody:   "content",
		},
		{
			name:       "no authorization header when token empty",
			statusCode: http.StatusOK,
			body:       "public content",
			token:      "",
			checkAuth:  true,
			wantBody:   "public content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotAuthHeader string
			var gotAcceptHeader string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuthHeader = r.Header.Get("Authorization")
				gotAcceptHeader = r.Header.Get("Accept")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			// redirect requests to test server by overriding the URL in FetchFile
			// we need to call a variant that accepts a base URL - instead, use a round-tripper override
			// since FetchFile uses http.DefaultClient, we swap it temporarily per-test via a custom client
			// but that's not safe for parallel tests. Instead, we inject via a package-level var.
			// Simplest approach: use the httptest server URL directly via fetchFileWithBase.
			got, err := fetchFileWithBase(context.Background(), ref, tt.token, srv.URL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FetchFile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if string(got) != tt.wantBody {
				t.Errorf("FetchFile() body = %q, want %q", got, tt.wantBody)
			}
			if tt.checkAuth {
				wantAuth := ""
				if tt.token != "" {
					wantAuth = "Bearer " + tt.token
				}
				if gotAuthHeader != wantAuth {
					t.Errorf("Authorization header = %q, want %q", gotAuthHeader, wantAuth)
				}
			}
			wantAccept := "application/vnd.github.raw"
			if gotAcceptHeader != wantAccept {
				t.Errorf("Accept header = %q, want %q", gotAcceptHeader, wantAccept)
			}
		})
	}
}

func TestFetchFileURLEncoding(t *testing.T) {
	t.Parallel()

	var gotRequestURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	// path segment already contains a percent-encoded space; should not be double-encoded
	ref := FileRef{Owner: "owner", Repo: "repo", Ref: "main", Path: "my%20file.go"}
	_, err := fetchFileWithBase(context.Background(), ref, "", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(gotRequestURI, "%2520") {
		t.Errorf("path was double-encoded: %q", gotRequestURI)
	}
	if !strings.Contains(gotRequestURI, "my%20file.go") {
		t.Errorf("expected properly encoded path, got: %q", gotRequestURI)
	}
}

func TestFetchFileContextCancellation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// block until client disconnects
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ref := FileRef{Owner: "o", Repo: "r", Ref: "main", Path: "f.go"}
	_, err := fetchFileWithBase(ctx, ref, "", srv.URL)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
