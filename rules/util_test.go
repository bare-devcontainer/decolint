package rules

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
)

// TestHoldsFeatureRefs covers every file type, including the one no rule declaring these paths
// applies to, since the answer for it is part of the contract rather than a case that cannot arise.
func TestHoldsFeatureRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileType linter.FileType
		pointer  string
		want     bool
	}{
		{"devcontainer features", linter.Devcontainer, "/features", true},
		{"devcontainer dependsOn", linter.Devcontainer, "/dependsOn", false},
		{"feature dependsOn", linter.Feature, "/dependsOn", true},
		{"feature features", linter.Feature, "/features", false},
		{"template features", linter.Template, "/features", false},
		{"template dependsOn", linter.Template, "/dependsOn", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := holdsFeatureRefs(tt.fileType, tt.pointer); got != tt.want {
				t.Errorf("holdsFeatureRefs(%q, %q) = %v, want %v", tt.fileType, tt.pointer, got, tt.want)
			}
		})
	}
}

// TestReadConfigFile_SizeCap covers the boundary of the size cap: a file at it is read, and one over
// it is refused outright, so the rules reading a Dockerfile or a Compose file report nothing on it
// rather than on the part of it that fit.
func TestReadConfigFile_SizeCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size int
		want bool
	}{
		{"at the cap", maxConfigFileBytes, true},
		{"over the cap", maxConfigFileBytes + 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := linter.Dir{FS: fstest.MapFS{"Dockerfile": {Data: []byte(strings.Repeat("#", tt.size))}}}
			src, ok := readConfigFile(dir, "Dockerfile")
			if ok != tt.want {
				t.Fatalf("readConfigFile of a %d-byte file: ok = %v, want %v", tt.size, ok, tt.want)
			}
			if ok && len(src) != tt.size {
				t.Errorf("read %d bytes, want %d", len(src), tt.size)
			}
		})
	}
}
