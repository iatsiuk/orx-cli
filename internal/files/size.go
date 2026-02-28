package files

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	KB                 = 1024
	MB                 = 1024 * KB
	DefaultMaxFileSize = 64 * KB
	DefaultMaxTokens   = 100000
)

// ParseSize parses human-readable size string (e.g., "64KB", "1MB", "100").
// Supports KB, MB suffixes (case-insensitive). Plain numbers are treated as bytes.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	upper := strings.ToUpper(s)

	var multiplier int64 = 1
	var numStr string

	switch {
	case strings.HasSuffix(upper, "MB"):
		multiplier = MB
		numStr = s[:len(s)-2]
	case strings.HasSuffix(upper, "KB"):
		multiplier = KB
		numStr = s[:len(s)-2]
	default:
		numStr = s
	}

	numStr = strings.TrimSpace(numStr)
	if numStr == "" {
		return 0, fmt.Errorf("invalid size: %q", s)
	}

	n, err := parseNumber(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid size: %q", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("invalid size (must be >= 0): %q", s)
	}
	return int64(n * float64(multiplier)), nil
}

func parseNumber(s string) (float64, error) {
	if strings.Contains(s, ".") {
		return strconv.ParseFloat(s, 64)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return float64(n), err
}

// FormatSize formats bytes as human-readable string (e.g., "64KB", "1.5MB").
func FormatSize(bytes int64) string {
	switch {
	case bytes >= MB:
		if bytes%MB == 0 {
			return fmt.Sprintf("%dMB", bytes/MB)
		}
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		if bytes%KB == 0 {
			return fmt.Sprintf("%dKB", bytes/KB)
		}
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
