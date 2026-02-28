package files

import "testing"

func TestParseSize(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"64KB", 65536, false},
		{"64kb", 65536, false},
		{"64Kb", 65536, false},
		{"1MB", 1048576, false},
		{"1mb", 1048576, false},
		{"100", 100, false},
		{"1.5MB", 1572864, false},
		{"0.5KB", 512, false},
		{"invalid", 0, true},
		{"", 0, true},
		{"KB", 0, true},
		{"MB", 0, true},
		{"-1KB", 0, true},
		{"-100", 0, true},
		{"-1.5MB", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSize(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{65536, "64KB"},
		{1048576, "1MB"},
		{100, "100B"},
		{1536, "1.5KB"},
		{1572864, "1.5MB"},
		{0, "0B"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
