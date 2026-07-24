package linter

import "testing"

func TestDocument_OffsetAt(t *testing.T) {
	t.Parallel()

	src := `{
  "image": "ubuntu:24.04",
  "forwardPorts": [3000, 8080],
  "customizations": {
    "vscode": {
      "extensions": ["golang.go"]
    }
  }
}`
	doc, err := ParseDocument([]byte(src))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}

	tests := []struct {
		name     string
		loc      []string
		wantLine int
		wantByte byte // first source byte at the returned offset, to anchor the assertion
	}{
		{"root", nil, 1, '{'},
		{"top-level property value", []string{"image"}, 2, '"'},
		{"array element", []string{"forwardPorts", "1"}, 3, '8'},
		{"nested object", []string{"customizations", "vscode"}, 5, '{'},
		{"nested array element", []string{"customizations", "vscode", "extensions", "0"}, 6, '"'},
		{"missing member falls back to parent", []string{"customizations", "absent"}, 4, '{'},
		{"non-container segment falls back", []string{"image", "deeper"}, 2, '"'},
		{"out-of-range index falls back", []string{"forwardPorts", "9"}, 3, '['},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			off := doc.OffsetAt(tt.loc)
			if got := src[off]; got != tt.wantByte {
				t.Errorf("OffsetAt(%v) offset %d points at %q, want %q", tt.loc, off, got, tt.wantByte)
			}
			if line, _ := doc.Position(off); line != tt.wantLine {
				t.Errorf("Position(OffsetAt(%v)) line = %d, want %d", tt.loc, line, tt.wantLine)
			}
		})
	}
}

func TestDocument_Position(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument([]byte("{\n  \"a\": 1\n}"))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	// The "a" key starts at byte 4 (after "{\n  "), on line 2, column 3.
	if line, col := doc.Position(4); line != 2 || col != 3 {
		t.Errorf("Position(4) = (%d, %d), want (2, 3)", line, col)
	}
}
