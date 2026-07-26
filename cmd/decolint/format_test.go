package main

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/bare-devcontainer/decolint/format"
	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
	"github.com/google/go-cmp/cmp"
)

// TestSarifRules_Fields checks that the adapter carries every field a SARIF consumer needs to
// describe a rule, the address of its documentation included, so an alert can be traced back to
// what the rule checks and why.
func TestSarifRules_Fields(t *testing.T) {
	t.Parallel()

	const id = "no-image-latest"
	var reg rules.Registration
	for _, r := range rules.Builtin() {
		if r.Rule.ID == id {
			reg = r
		}
	}
	if reg.Rule == nil {
		t.Fatalf("built-in rule %s not found", id)
	}

	// A reproducibility rule is off by default, so its category has to be enabled for the catalog
	// to describe it at all.
	cfg := Config{Categories: map[string]linter.Severity{"reproducibility": linter.SeverityError}}
	var got format.SARIFRule
	for _, r := range sarifRules(cfg) {
		if r.ID == id {
			got = r
		}
	}

	want := format.SARIFRule{
		ID:          reg.Rule.ID,
		Description: reg.Rule.Description,
		Category:    reg.Rule.Category.String(),
		HelpURI:     rules.DocsURL(reg.Rule.ID),
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("sarifRules() entry mismatch (-want +got):\n%s", diff)
	}
}

func TestParseFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		color   bool
		want    Format
		wantErr bool
	}{
		{"empty", "", false, format.TextFormat{}, false},
		{"text", "text", false, format.TextFormat{}, false},
		{"text colored", "text", true, format.TextFormat{Color: true}, false},
		{"json", "json", false, format.JSONFormat{}, false},
		// Color reaches the text format alone; a machine-readable format never carries escape
		// sequences, however the output was asked to be colored.
		{"json ignores color", "json", true, format.JSONFormat{}, false},
		{"github", "github", false, format.GitHubFormat{}, false},
		{"github ignores color", "github", true, format.GitHubFormat{}, false},
		{"sarif", "sarif", false, format.SARIFFormat{Version: version, Rules: sarifRules(Config{})}, false},
		{"sarif ignores color", "sarif", true, format.SARIFFormat{Version: version, Rules: sarifRules(Config{})}, false},
		{"unknown", "bogus", false, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{Format: tt.in}
			got, err := parseFormat(cfg, tt.color)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFormat(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// cmp.Diff rather than ==: SARIFFormat is not comparable, so an interface comparison
			// would panic.
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseFormat(%q) mismatch (-want +got):\n%s", tt.in, diff)
			}
		})
	}
}

// TestSarifRules checks that the SARIF rule catalog describes the rules the run has enabled, so a
// consumer can tell a rule that ran and found nothing from one that never ran.
func TestSarifRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         Config
		wantID      string
		wantMissing string
	}{
		{
			name:        "defaults list the correctness rules only",
			wantID:      "missing-container-def",
			wantMissing: "no-seccomp-override",
		},
		{
			name:        "a category override adds its rules",
			cfg:         Config{Categories: map[string]linter.Severity{"security": linter.SeverityError}},
			wantID:      "no-seccomp-override",
			wantMissing: "no-image-latest",
		},
		{
			name:        "a rule turned off is left out",
			cfg:         Config{Rules: map[string]linter.Severity{"missing-container-def": linter.SeverityOff}},
			wantID:      "invalid-semver",
			wantMissing: "missing-container-def",
		},
		{
			name:        "a platform-scoped rule needs its platform",
			wantID:      "missing-container-def",
			wantMissing: "no-bind-mount",
		},
		{
			name:   "a platform-scoped rule appears once selected",
			cfg:    Config{Platforms: []linter.Platform{linter.PlatformCodespaces}},
			wantID: "no-bind-mount",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ids []string
			for _, r := range sarifRules(tt.cfg) {
				if r.Description == "" || r.Category == "" {
					t.Errorf("rule %q is described as %+v, want a description and a category", r.ID, r)
				}
				ids = append(ids, r.ID)
			}
			if !slices.Contains(ids, tt.wantID) {
				t.Errorf("sarifRules() = %v, want it to contain %q", ids, tt.wantID)
			}
			if tt.wantMissing != "" && slices.Contains(ids, tt.wantMissing) {
				t.Errorf("sarifRules() = %v, want it not to contain %q", ids, tt.wantMissing)
			}
		})
	}
}

// TestRuleSnippet_ModeIsPermissionBitsOnly checks that ruleSnippet reports only the POSIX
// permission bits (see format.RuleExampleFile.Mode), not the type bits fs.FileMode also carries
// (e.g. fs.ModeDir), which would otherwise inflate the value ruleExample -rules -format=json emits.
func TestRuleSnippet_ModeIsPermissionBitsOnly(t *testing.T) {
	t.Parallel()

	snippet := ruleSnippet(linter.Snippet{
		Files: []linter.ExampleFile{
			{Path: "some-dir", Mode: fs.ModeDir | 0o755},
		},
	})
	if got, want := snippet.Files[0].Mode, uint32(0o755); got != want {
		t.Errorf("Mode = %#o, want %#o", got, want)
	}
}
