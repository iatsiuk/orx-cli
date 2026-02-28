package files

import (
	"bytes"
	"net/http"
	"strings"
)

// BinaryCheckSize is the number of bytes to read for binary detection.
const BinaryCheckSize = 8192

// IsBinary checks if data appears to be binary content.
// It first checks for UTF-16 BOM (which indicates text), then checks for null bytes,
// and uses http.DetectContentType as a secondary signal.
func IsBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	// UTF-16 files with BOM: we don't decode them, so treat as binary to avoid garbage in prompt
	if hasUTF16BOM(data) {
		return true
	}

	if hasNullBytes(data) {
		return true
	}

	// use content-type detection as secondary signal
	contentType := http.DetectContentType(data)
	// binary if not text/* or application/json
	if !strings.HasPrefix(contentType, "text/") &&
		contentType != "application/json" &&
		contentType != "application/javascript" {
		return true
	}

	return false
}

// checks for UTF-16 byte order marks
func hasUTF16BOM(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	// UTF-16 LE: FF FE
	if data[0] == 0xFF && data[1] == 0xFE {
		return true
	}
	// UTF-16 BE: FE FF
	if data[0] == 0xFE && data[1] == 0xFF {
		return true
	}
	return false
}

// checks for null bytes in data
func hasNullBytes(data []byte) bool {
	return bytes.Contains(data, []byte{0x00})
}
