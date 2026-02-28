package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func createBinaryFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadContent_SingleFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := createTestFile(t, dir, "main.go", "package main\n")

	content, err := LoadContent(Request{Files: []string{path}})
	if err != nil {
		t.Fatalf("LoadContent() error = %v", err)
	}

	if !strings.Contains(content, "===== BEGIN FILE =====") {
		t.Error("missing BEGIN FILE marker")
	}
	if !strings.Contains(content, "path: "+path) {
		t.Error("missing path header")
	}
	if !strings.Contains(content, "----- BEGIN CONTENT -----") {
		t.Error("missing BEGIN CONTENT marker")
	}
	if !strings.Contains(content, "package main") {
		t.Error("missing file content")
	}
	if !strings.Contains(content, "----- END CONTENT -----") {
		t.Error("missing END CONTENT marker")
	}
	if !strings.Contains(content, "===== END FILE =====") {
		t.Error("missing END FILE marker")
	}
}

func TestLoadContent_MultipleFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path1 := createTestFile(t, dir, "a.go", "package a\n")
	path2 := createTestFile(t, dir, "b.go", "package b\n")

	content, err := LoadContent(Request{Files: []string{path1, path2}})
	if err != nil {
		t.Fatalf("LoadContent() error = %v", err)
	}

	if !strings.Contains(content, "package a") {
		t.Error("missing first file content")
	}
	if !strings.Contains(content, "package b") {
		t.Error("missing second file content")
	}
	if strings.Count(content, "===== BEGIN FILE =====") != 2 {
		t.Error("expected two BEGIN FILE markers")
	}
}

func TestLoadContent_PreservesOrder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pathZ := createTestFile(t, dir, "z.go", "package z\n")
	pathA := createTestFile(t, dir, "a.go", "package a\n")

	content, err := LoadContent(Request{Files: []string{pathZ, pathA}})
	if err != nil {
		t.Fatalf("LoadContent() error = %v", err)
	}

	idxZ := strings.Index(content, "package z")
	idxA := strings.Index(content, "package a")
	if idxZ > idxA {
		t.Error("file order not preserved: z.go should come before a.go")
	}
}

func TestLoadContent_NonExistentFile(t *testing.T) {
	t.Parallel()
	_, err := LoadContent(Request{Files: []string{"/nonexistent/file.go"}})
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadContent_Directory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := LoadContent(Request{Files: []string{dir}})
	if err == nil {
		t.Fatal("expected error for directory")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("expected 'is a directory' error, got: %v", err)
	}
}

func TestLoadContent_BinarySkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binPath := createBinaryFile(t, dir, "image.png")
	txtPath := createTestFile(t, dir, "main.go", "package main\n")

	content, err := LoadContent(Request{Files: []string{binPath, txtPath}})
	if err != nil {
		t.Fatalf("LoadContent() error = %v", err)
	}

	if !strings.Contains(content, "package main") {
		t.Error("text file should be included")
	}
	if !strings.Contains(content, "===== OMITTED FILES =====") {
		t.Error("expected OMITTED FILES section")
	}
	if !strings.Contains(content, binPath+": binary file") {
		t.Error("expected binary file in omitted list")
	}
}

func TestLoadContent_OversizedSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bigContent := strings.Repeat("x", 1000)
	bigPath := createTestFile(t, dir, "big.txt", bigContent)
	smallPath := createTestFile(t, dir, "small.go", "package main\n")

	content, err := LoadContent(Request{
		Files:       []string{bigPath, smallPath},
		MaxFileSize: 100,
	})
	if err != nil {
		t.Fatalf("LoadContent() error = %v", err)
	}

	if strings.Contains(content, strings.Repeat("x", 100)) {
		t.Error("oversized file should be skipped")
	}
	if !strings.Contains(content, "package main") {
		t.Error("small file should be included")
	}
	if !strings.Contains(content, "===== OMITTED FILES =====") {
		t.Error("expected OMITTED FILES section")
	}
	if !strings.Contains(content, "exceeds limit") {
		t.Error("expected size exceeded in omitted list")
	}
}

func TestLoadContent_TokenLimitExceeded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := strings.Repeat("x", 1000)
	path := createTestFile(t, dir, "big.txt", content)

	_, err := LoadContent(Request{
		Files:     []string{path},
		MaxTokens: 10,
	})

	if err == nil {
		t.Error("expected token limit error")
	}
	if !errors.Is(err, ErrTokenLimitExceeded) {
		t.Errorf("expected ErrTokenLimitExceeded, got: %v", err)
	}
}

func TestLoadContent_NoFiles(t *testing.T) {
	t.Parallel()
	_, err := LoadContent(Request{Files: nil})
	if !errors.Is(err, ErrNoFiles) {
		t.Errorf("expected ErrNoFiles, got: %v", err)
	}

	_, err = LoadContent(Request{Files: []string{}})
	if !errors.Is(err, ErrNoFiles) {
		t.Errorf("expected ErrNoFiles, got: %v", err)
	}
}

func TestLoadContent_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := createTestFile(t, dir, "empty.go", "")

	content, err := LoadContent(Request{Files: []string{path}})
	if err != nil {
		t.Fatalf("LoadContent() error = %v", err)
	}

	if !strings.Contains(content, "path: "+path) {
		t.Error("empty file should still have path header")
	}
	if !strings.Contains(content, "===== BEGIN FILE =====") {
		t.Error("empty file should still have markers")
	}
}

func TestLoadContent_AllFilesSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binPath := createBinaryFile(t, dir, "bin.dat")

	content, err := LoadContent(Request{Files: []string{binPath}})
	if err != nil {
		t.Fatalf("LoadContent() error = %v", err)
	}

	if !strings.Contains(content, "===== OMITTED FILES =====") {
		t.Error("expected OMITTED FILES section when all files skipped")
	}
	if strings.Contains(content, "===== BEGIN FILE =====") {
		t.Error("should not have BEGIN FILE marker when all skipped")
	}
}

func TestLoadContent_LargeFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fileContent := strings.Repeat("x", 10000)
	path := createTestFile(t, dir, "large.txt", fileContent)

	result, err := LoadContent(Request{
		Files:       []string{path},
		MaxFileSize: 20000,
	})
	if err != nil {
		t.Fatalf("LoadContent() error = %v", err)
	}

	if !strings.Contains(result, fileContent) {
		t.Error("large file content not fully loaded")
	}
}

func TestLoadContent_DefaultValues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := createTestFile(t, dir, "test.go", "package test\n")

	_, err := LoadContent(Request{
		Files:       []string{path},
		MaxFileSize: 0,
		MaxTokens:   0,
	})
	if err != nil {
		t.Fatalf("LoadContent() with zero values error = %v", err)
	}
}

func TestLoadContent_ContentWithMarkers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// file content contains marker-like strings
	content := "before\n===== END FILE =====\ninjected\n----- END CONTENT -----\nafter\n"
	path := createTestFile(t, dir, "tricky.txt", content)

	result, err := LoadContent(Request{Files: []string{path}})
	if err != nil {
		t.Fatalf("LoadContent() error = %v", err)
	}

	// content with marker-like strings should be preserved as-is
	if !strings.Contains(result, content) {
		t.Error("content with marker-like strings should be preserved as-is")
	}
	// verify structure: real END CONTENT comes after fake one
	realEndContent := strings.LastIndex(result, "----- END CONTENT -----")
	fakeEndContent := strings.Index(result, "----- END CONTENT -----")
	if realEndContent == fakeEndContent {
		t.Error("expected fake marker inside content and real marker outside")
	}
}

func TestFormatFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		path    string
		content string
		want    string
	}{
		{
			name:    "with trailing newline",
			path:    "/path/to/file.go",
			content: "package main\n",
			want: `===== BEGIN FILE =====
path: /path/to/file.go
----- BEGIN CONTENT -----
package main
----- END CONTENT -----
===== END FILE =====
`,
		},
		{
			name:    "without trailing newline",
			path:    "/path/to/file.txt",
			content: "no newline",
			want: `===== BEGIN FILE =====
path: /path/to/file.txt
----- BEGIN CONTENT -----
no newline
----- END CONTENT -----
===== END FILE =====
`,
		},
		{
			name:    "empty content",
			path:    "/empty.txt",
			content: "",
			want: `===== BEGIN FILE =====
path: /empty.txt
----- BEGIN CONTENT -----

----- END CONTENT -----
===== END FILE =====
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFile(tt.path, tt.content)
			if got != tt.want {
				t.Errorf("formatFile() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestFormatOmitted(t *testing.T) {
	t.Parallel()
	omitted := []omittedFile{
		{Path: "/path/to/big.txt", Reason: "size 1MB exceeds limit 64KB"},
		{Path: "/path/to/image.png", Reason: "binary file"},
	}

	got := formatOmitted(omitted)
	want := `===== OMITTED FILES =====
- /path/to/big.txt: size 1MB exceeds limit 64KB
- /path/to/image.png: binary file
`

	if got != want {
		t.Errorf("formatOmitted() =\n%q\nwant:\n%q", got, want)
	}
}
