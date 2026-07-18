package rules_test

import (
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
	"github.com/google/go-cmp/cmp"
)

func TestRegisterRulesUnknownOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		platforms  []linter.Platform
		overrides  map[string]linter.Severity
		wantErr    bool
		wantErrIDs []string // substrings that must all appear in the error message
	}{
		{"nil overrides", nil, nil, false, nil},
		{"known rule id", nil, map[string]linter.Severity{"no-image-latest": linter.SeverityError}, false, nil},
		{"unknown rule id", nil, map[string]linter.Severity{"no-image-latst": linter.SeverityError}, true, []string{"no-image-latst"}},
		{
			"multiple unknown ids",
			nil,
			map[string]linter.Severity{"zzz-bogus": linter.SeverityError, "aaa-bogus": linter.SeverityWarn},
			true,
			[]string{"aaa-bogus", "zzz-bogus"},
		},
		{
			"platform-scoped rule id not selected is not unknown",
			nil,
			map[string]linter.Severity{"no-bind-mount": linter.SeverityError},
			false,
			nil,
		},
		{
			"platform-scoped rule id selected is not unknown",
			[]linter.Platform{linter.PlatformCodespaces},
			map[string]linter.Severity{"no-bind-mount": linter.SeverityError},
			false,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := linter.New()
			err := rules.RegisterRules(l, tt.platforms, rules.Overrides{Rules: tt.overrides})
			if (err != nil) != tt.wantErr {
				t.Fatalf("RegisterRules() error = %v, wantErr %v", err, tt.wantErr)
			}
			for _, id := range tt.wantErrIDs {
				if !strings.Contains(err.Error(), id) {
					t.Errorf("RegisterRules() error = %q, want it to mention %q", err, id)
				}
			}
		})
	}
}

func TestRegisterRulesPlatformFilter(t *testing.T) {
	t.Parallel()

	// no-bind-mount is scoped to PlatformCodespaces, so it should only fire when that
	// platform is selected.
	const src = `{"mounts": ["source=/host,target=/data,type=bind"]}`

	tests := []struct {
		name      string
		platforms []linter.Platform
		wantFired bool
	}{
		{"platform not selected", nil, false},
		{"platform selected", []linter.Platform{linter.PlatformCodespaces}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := linter.New()
			if err := rules.RegisterRules(l, tt.platforms, rules.Overrides{}); err != nil {
				t.Fatalf("RegisterRules: %v", err)
			}
			issues := lintSource(t, l, "devcontainer.json", linter.Devcontainer, src)
			fired := ruleFired(issues, "no-bind-mount")
			if fired != tt.wantFired {
				t.Errorf("no-bind-mount fired = %v, want %v (issues: %v)", fired, tt.wantFired, issues)
			}
		})
	}
}

func TestRegisterRulesDefaultOffRule(t *testing.T) {
	t.Parallel()

	// no-seccomp-override is off by default, so it should never fire without an override, even
	// though src trips its check.
	const src = `{"securityOpt": ["seccomp=custom.json"]}`

	l := linter.New()
	if err := rules.RegisterRules(l, nil, rules.Overrides{}); err != nil {
		t.Fatalf("RegisterRules: %v", err)
	}
	issues := lintSource(t, l, "devcontainer.json", linter.Devcontainer, src)
	if ruleFired(issues, "no-seccomp-override") {
		t.Errorf("no-seccomp-override fired despite being off by default: %v", issues)
	}
}

func TestRegisterRulesOverrideEnablesOffDefaultRule(t *testing.T) {
	t.Parallel()

	const src = `{"securityOpt": ["seccomp=custom.json"]}`

	l := linter.New()
	overrides := rules.Overrides{Rules: map[string]linter.Severity{"no-seccomp-override": linter.SeverityError}}
	if err := rules.RegisterRules(l, nil, overrides); err != nil {
		t.Fatalf("RegisterRules: %v", err)
	}
	issues := lintSource(t, l, "devcontainer.json", linter.Devcontainer, src)
	if !ruleFired(issues, "no-seccomp-override") {
		t.Errorf("no-seccomp-override did not fire despite being overridden to error: %v", issues)
	}
}

// ruleFired reports whether issues contains a finding from the rule with the given ID.
func ruleFired(issues []linter.Issue, ruleID string) bool {
	for _, issue := range issues {
		if issue.RuleID == ruleID {
			return true
		}
	}
	return false
}

func TestRegisterRulesSeverityOverride(t *testing.T) {
	t.Parallel()

	const src = `{"image": "ubuntu:latest"}`
	const message = `image "ubuntu:latest" uses the "latest" tag; pin a specific version`

	tests := []struct {
		name      string
		overrides map[string]linter.Severity
		want      []linter.Issue
	}{
		{
			// no-image-latest is in the reproducibility category, which is off by default, so with
			// no overrides at all the rule doesn't fire.
			"no overrides keeps default severity",
			nil,
			nil,
		},
		{
			"override promotes severity",
			map[string]linter.Severity{"no-image-latest": linter.SeverityError},
			[]linter.Issue{
				{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "no-image-latest", Message: message, Severity: linter.SeverityError},
			},
		},
		{
			"override disables the rule",
			map[string]linter.Severity{"no-image-latest": linter.SeverityOff},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := linter.New()
			if err := rules.RegisterRules(l, nil, rules.Overrides{Rules: tt.overrides}); err != nil {
				t.Fatalf("RegisterRules: %v", err)
			}
			got := lintSource(t, l, "devcontainer.json", linter.Devcontainer, src)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("issues mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
