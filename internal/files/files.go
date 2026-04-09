package files

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	ErrNoFiles            = errors.New("no files provided")
	ErrTokenLimitExceeded = errors.New("token limit exceeded")
)

// Request configures file loading.
type Request struct {
	Files       []string // explicit file paths (no globs)
	MaxFileSize int64    // max size per file (0 = DefaultMaxFileSize)
	MaxTokens   int      // max estimated tokens total (0 = DefaultMaxTokens)
}

// tracks a file that was skipped during loading
type omittedFile struct {
	Path   string
	Reason string
}

// LoadContent loads files and returns formatted content.
// Missing files, directories, and permission errors cause hard failures.
// Binary and oversized files are skipped and listed in OMITTED FILES section.
func LoadContent(req Request) (string, error) {
	if len(req.Files) == 0 {
		return "", ErrNoFiles
	}

	maxFileSize := req.MaxFileSize
	if maxFileSize <= 0 {
		maxFileSize = DefaultMaxFileSize
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}

	var result strings.Builder
	var omitted []omittedFile

	for _, path := range req.Files {
		content, skipReason, err := loadSingleFile(path, maxFileSize)
		if err != nil {
			return "", err
		}
		if skipReason != "" {
			omitted = append(omitted, omittedFile{Path: path, Reason: skipReason})
			continue
		}

		result.WriteString(FormatFile(path, content))
	}

	if len(omitted) > 0 {
		result.WriteString(formatOmitted(omitted))
	}

	output := result.String()
	tokens := EstimateTokens(output)
	if tokens > maxTokens {
		return "", fmt.Errorf("%w: estimated %d tokens (limit: %d)", ErrTokenLimitExceeded, tokens, maxTokens)
	}

	return output, nil
}

func FormatFile(path, content string) string {
	var b strings.Builder
	b.WriteString("===== BEGIN FILE =====\n")
	b.WriteString("path: ")
	b.WriteString(path)
	b.WriteString("\n----- BEGIN CONTENT -----\n")
	b.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("----- END CONTENT -----\n")
	b.WriteString("===== END FILE =====\n")
	return b.String()
}

func formatOmitted(omitted []omittedFile) string {
	var b strings.Builder
	b.WriteString("===== OMITTED FILES =====\n")
	for _, o := range omitted {
		b.WriteString("- ")
		b.WriteString(o.Path)
		b.WriteString(": ")
		b.WriteString(o.Reason)
		b.WriteString("\n")
	}
	return b.String()
}

func loadSingleFile(path string, maxSize int64) (content, skipReason string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", fmt.Errorf("file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return "", "", fmt.Errorf("file %q: %w", path, err)
	}

	if info.IsDir() {
		return "", "", fmt.Errorf("file %q: is a directory", path)
	}

	if info.Size() > maxSize {
		return "", fmt.Sprintf("size %s exceeds limit %s", FormatSize(info.Size()), FormatSize(maxSize)), nil
	}

	prefix := make([]byte, BinaryCheckSize)
	n, err := f.Read(prefix)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", fmt.Errorf("file %q: %w", path, err)
	}
	prefix = prefix[:n]

	if IsBinary(prefix) {
		return "", "binary file", nil
	}

	data, err := io.ReadAll(io.MultiReader(bytes.NewReader(prefix), f))
	if err != nil {
		return "", "", fmt.Errorf("file %q: %w", path, err)
	}

	return string(data), "", nil
}
