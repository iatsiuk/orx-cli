package files

import "testing"

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", []byte{}, false},
		{"go source", []byte("package main\n\nfunc main() {\n}\n"), false},
		{"json", []byte(`{"key": "value"}`), false},
		{"png header", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, true},
		{"null bytes", []byte("hello\x00world"), true},
		{"utf16 le bom", []byte{0xFF, 0xFE, 0x68, 0x00, 0x65, 0x00}, true},
		{"utf16 be bom", []byte{0xFE, 0xFF, 0x00, 0x68, 0x00, 0x65}, true},
		{"plain text", []byte("Hello, World!"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsBinary(tt.data)
			if got != tt.want {
				t.Errorf("IsBinary(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
