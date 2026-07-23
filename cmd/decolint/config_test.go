package main

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/google/go-cmp/cmp"
)

func TestConfigMarshalJSONTo(t *testing.T) {
	t.Parallel()

	t.Run("all fields present", func(t *testing.T) {
		t.Parallel()
		cfg := Config{
			Platforms:    []linter.Platform{linter.PlatformVSCode, linter.PlatformCodespaces},
			Merge:        true,
			DenyWarnings: true,
			Format:       "github",
			LocalEnv:     map[string]string{"HOME": "/home/user"},
			Categories:   map[string]linter.Severity{"security": linter.SeverityError},
			Rules:        map[string]linter.Severity{"no-image-latest": linter.SeverityError},
		}
		want := `{"platforms":["vscode","codespaces"],"merge":true,"denyWarnings":true,"format":"github","localEnv":{"HOME":"/home/user"},"categories":{"security":"error"},"rules":{"no-image-latest":"error"}}`
		got, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		if string(got) != want {
			t.Errorf("json.Marshal(cfg) = %s, want %s", got, want)
		}
	})

	t.Run("all optional fields absent", func(t *testing.T) {
		t.Parallel()
		// Platforms, merge, denyWarnings, format, and categories are omitted when empty or false;
		// rules is always written.
		want := `{"rules":{}}`
		got, err := json.Marshal(Config{})
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		if string(got) != want {
			t.Errorf("json.Marshal(cfg) = %s, want %s", got, want)
		}
	})

	t.Run("sorts localEnv, categories, and rules by key", func(t *testing.T) {
		t.Parallel()
		cfg := Config{
			LocalEnv: map[string]string{
				"USERPROFILE": "C:\\Users\\user",
				"HOME":        "/home/user",
			},
			Categories: map[string]linter.Severity{
				"security":        linter.SeverityError,
				"reproducibility": linter.SeverityWarn,
			},
			Rules: map[string]linter.Severity{
				"require-non-root":       linter.SeverityOff,
				"no-image-latest":        linter.SeverityError,
				"pin-image-digest":       linter.SeverityWarn,
				"id-dir-mismatch":        linter.SeverityError,
				"missing-required-props": linter.SeverityError,
			},
		}
		want := `{"localEnv":{"HOME":"/home/user","USERPROFILE":"C:\\Users\\user"},"categories":{"reproducibility":"warn","security":"error"},"rules":{"id-dir-mismatch":"error","missing-required-props":"error","no-image-latest":"error","pin-image-digest":"warn","require-non-root":"off"}}`
		// Marshal repeatedly: map iteration order is randomized per run, so this would be
		// flaky if MarshalJSONTo didn't force a deterministic, key-sorted order.
		for range 5 {
			got, err := json.Marshal(cfg)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if string(got) != want {
				t.Errorf("json.Marshal(cfg) = %s, want %s", got, want)
			}
		}
	})
}

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
		{
			"categories and rules",
			`{"categories": {"security": "error"}, "rules": {"no-image-latest": "off"}}`,
			Config{
				Categories: map[string]linter.Severity{"security": linter.SeverityError},
				Rules:      map[string]linter.Severity{"no-image-latest": linter.SeverityOff},
			},
			false,
		},
		{
			"single platform",
			`{"platforms": ["vscode"]}`,
			Config{Platforms: []linter.Platform{linter.PlatformVSCode}},
			false,
		},
		{
			"multiple platforms with mixed case",
			`{"platforms": ["VSCode", "codespaces"]}`,
			Config{Platforms: []linter.Platform{linter.PlatformVSCode, linter.PlatformCodespaces}},
			false,
		},
		{
			"platforms with rules",
			`{"platforms": ["codespaces"], "rules": {"no-image-latest": "error"}}`,
			Config{
				Platforms: []linter.Platform{linter.PlatformCodespaces},
				Rules:     map[string]linter.Severity{"no-image-latest": linter.SeverityError},
			},
			false,
		},
		{
			"merge",
			`{"merge": true}`,
			Config{Merge: true},
			false,
		},
		{
			"denyWarnings",
			`{"denyWarnings": true}`,
			Config{DenyWarnings: true},
			false,
		},
		{
			"format",
			`{"format": "json"}`,
			Config{Format: "json"},
			false,
		},
		{
			"localEnv",
			`{"localEnv": {"HOME": "/home/user", "TAG": ""}}`,
			Config{LocalEnv: map[string]string{"HOME": "/home/user", "TAG": ""}},
			false,
		},
		{"invalid severity", `{"rules": {"no-image-latest": "critical"}}`, Config{}, true},
		{"invalid category severity", `{"categories": {"security": "critical"}}`, Config{}, true},
		{"unknown platform", `{"platforms": ["intellij"]}`, Config{}, true},
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

func TestLoadConfig_ExplicitPath(t *testing.T) {
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

func TestLoadConfig_Discovery(t *testing.T) {
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

func TestMergeConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts Options
		cfg  Config
		want Config
	}{
		{
			"CLI platform overrides config file platforms",
			Options{Platforms: []linter.Platform{linter.PlatformVSCode}},
			Config{Platforms: []linter.Platform{linter.PlatformCodespaces}},
			Config{Platforms: []linter.Platform{linter.PlatformVSCode}},
		},
		{
			"CLI platform unset falls back to config file platforms",
			Options{},
			Config{Platforms: []linter.Platform{linter.PlatformCodespaces}},
			Config{Platforms: []linter.Platform{linter.PlatformCodespaces}},
		},
		{
			"CLI merge flag enables it",
			Options{Merge: true, mergeSet: true},
			Config{},
			Config{Merge: true},
		},
		{
			"CLI merge flag not given falls back to config file merge",
			Options{},
			Config{Merge: true},
			Config{Merge: true},
		},
		{
			"CLI merge=false overrides config file merge: true",
			Options{Merge: false, mergeSet: true},
			Config{Merge: true},
			Config{Merge: false},
		},
		{
			"CLI deny-warnings flag enables it",
			Options{DenyWarnings: true, denyWarningsSet: true},
			Config{},
			Config{DenyWarnings: true},
		},
		{
			"CLI deny-warnings not given falls back to config file denyWarnings",
			Options{},
			Config{DenyWarnings: true},
			Config{DenyWarnings: true},
		},
		{
			"CLI deny-warnings=false overrides config file denyWarnings: true",
			Options{DenyWarnings: false, denyWarningsSet: true},
			Config{DenyWarnings: true},
			Config{DenyWarnings: false},
		},
		{
			"CLI format overrides config file format",
			Options{Format: "github"},
			Config{Format: "json"},
			Config{Format: "github"},
		},
		{
			"CLI format unset falls back to config file format",
			Options{},
			Config{Format: "json"},
			Config{Format: "json"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mergeConfig(tt.opts, tt.cfg)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mergeConfig() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
