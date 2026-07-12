package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/google/go-cmp/cmp"
)

func TestParseConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		want    Config
		wantErr bool
	}{
		{"empty rules", `{"rules": {}}`, Config{Rules: map[string]linter.Severity{}}, false},
		{"no rules member", `{}`, Config{}, false},
		{
			"jsonc with comments and trailing comma",
			"{\n  // override severities\n  \"rules\": {\n    \"no-image-latest\": \"error\",\n  },\n}\n",
			Config{Rules: map[string]linter.Severity{"no-image-latest": linter.SeverityError}},
			false,
		},
		{
			"multiple rules",
			`{"rules": {"no-image-latest": "error", "pin-image-digest": "warn", "require-non-root": "off"}}`,
			Config{Rules: map[string]linter.Severity{
				"no-image-latest":  linter.SeverityError,
				"pin-image-digest": linter.SeverityWarn,
				"require-non-root": linter.SeverityOff,
			}},
			false,
		},
		{"invalid severity", `{"rules": {"no-image-latest": "critical"}}`, Config{}, true},
		{"malformed json", `{"rules": {`, Config{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseConfig("config.jsonc", []byte(tt.src))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseConfig() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadConfigExplicitPath(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		if _, err := loadConfig(filepath.Join(t.TempDir(), "nonexistent.jsonc")); err == nil {
			t.Error("loadConfig: got nil error, want a read error")
		}
	})

	t.Run("valid file", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "decolint.jsonc")
		if err := os.WriteFile(path, []byte(`{"rules": {"no-image-latest": "error"}}`), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		got, err := loadConfig(path)
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		want := Config{Rules: map[string]linter.Severity{"no-image-latest": linter.SeverityError}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("loadConfig() mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestLoadConfigDiscovery(t *testing.T) {
	// Uses t.Chdir, which cannot be combined with t.Parallel.
	tests := []struct {
		name  string
		files map[string]string
		want  Config
	}{
		{"no default config present", nil, Config{}},
		{
			".decolint.jsonc present",
			map[string]string{".decolint.jsonc": `{"rules": {"no-image-latest": "error"}}`},
			Config{Rules: map[string]linter.Severity{"no-image-latest": linter.SeverityError}},
		},
		{
			".decolint.json fallback",
			map[string]string{".decolint.json": `{"rules": {"no-image-latest": "warn"}}`},
			Config{Rules: map[string]linter.Severity{"no-image-latest": linter.SeverityWarn}},
		},
		{
			".decolint.jsonc takes precedence over .decolint.json",
			map[string]string{
				".decolint.jsonc": `{"rules": {"no-image-latest": "error"}}`,
				".decolint.json":  `{"rules": {"no-image-latest": "warn"}}`,
			},
			Config{Rules: map[string]linter.Severity{"no-image-latest": linter.SeverityError}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for name, content := range tt.files {
				if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}
			got, err := loadConfig("")
			if err != nil {
				t.Fatalf("loadConfig: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("loadConfig() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
