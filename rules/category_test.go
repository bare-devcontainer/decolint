package rules_test

import (
	"strings"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestBuiltin_ValidCategories(t *testing.T) {
	t.Parallel()

	for _, reg := range rules.Builtin() {
		// The zero value of Category is invalid on purpose, so a rule that forgets to declare its
		// category fails here instead of silently landing in a category.
		if _, err := linter.ParseCategory(reg.Rule.Category.String()); err != nil {
			t.Errorf("rule %s does not declare a valid category: %v", reg.Rule.ID, err)
		}
	}
}

func TestOverridesSeverityFor(t *testing.T) {
	t.Parallel()

	// no-image-latest is in the reproducibility category, which is off by default.
	reg := builtinRegistration(t, "no-image-latest")

	tests := []struct {
		name      string
		overrides rules.Overrides
		want      linter.Severity
	}{
		{
			"no overrides keeps the default",
			rules.Overrides{},
			linter.SeverityOff,
		},
		{
			"category override applies",
			rules.Overrides{Categories: map[string]linter.Severity{"reproducibility": linter.SeverityError}},
			linter.SeverityError,
		},
		{
			"category name is matched case-insensitively",
			rules.Overrides{Categories: map[string]linter.Severity{"Reproducibility": linter.SeverityError}},
			linter.SeverityError,
		},
		{
			"unrelated category leaves the default",
			rules.Overrides{Categories: map[string]linter.Severity{"security": linter.SeverityError}},
			linter.SeverityOff,
		},
		{
			"rule override takes precedence over category",
			rules.Overrides{
				Categories: map[string]linter.Severity{"reproducibility": linter.SeverityError},
				Rules:      map[string]linter.Severity{"no-image-latest": linter.SeverityOff},
			},
			linter.SeverityOff,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.overrides.SeverityFor(reg); got != tt.want {
				t.Errorf("SeverityFor(%s) = %v, want %v", reg.Rule.ID, got, tt.want)
			}
		})
	}
}

func TestRegisterRules_UnknownCategory(t *testing.T) {
	t.Parallel()

	l := linter.New()
	overrides := rules.Overrides{Categories: map[string]linter.Severity{"secure": linter.SeverityError}}
	err := rules.RegisterRules(l, nil, overrides)
	if err == nil {
		t.Fatal("RegisterRules: got nil error, want an unknown-category error")
	}
	if !strings.Contains(err.Error(), "secure") {
		t.Errorf("RegisterRules() error = %q, want it to mention %q", err, "secure")
	}
}

func TestRegisterRules_CategoryOverride(t *testing.T) {
	t.Parallel()

	// Trips no-seccomp-override, a security rule that is off by default.
	const src = `{"securityOpt": ["seccomp=custom.json"]}`

	tests := []struct {
		name      string
		overrides rules.Overrides
		wantFired bool
	}{
		{
			"category override enables an off-by-default rule",
			rules.Overrides{Categories: map[string]linter.Severity{"security": linter.SeverityError}},
			true,
		},
		{
			"rule override wins over category override",
			rules.Overrides{
				Categories: map[string]linter.Severity{"security": linter.SeverityError},
				Rules:      map[string]linter.Severity{"no-seccomp-override": linter.SeverityOff},
			},
			false,
		},
		{
			"unrelated category override leaves the rule off",
			rules.Overrides{Categories: map[string]linter.Severity{"style": linter.SeverityError}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := linter.New()
			if err := rules.RegisterRules(l, nil, tt.overrides); err != nil {
				t.Fatalf("RegisterRules: %v", err)
			}
			issues := lintSource(t, l, "devcontainer.json", linter.Devcontainer, src, nil)
			if fired := ruleFired(issues, "no-seccomp-override"); fired != tt.wantFired {
				t.Errorf("no-seccomp-override fired = %v, want %v (issues: %v)", fired, tt.wantFired, issues)
			}
		})
	}
}

// builtinRegistration returns the built-in registration for the rule with the given ID, failing
// the test if no such rule exists.
func builtinRegistration(t *testing.T, id string) rules.Registration {
	t.Helper()
	for _, reg := range rules.Builtin() {
		if reg.Rule.ID == id {
			return reg
		}
	}
	t.Fatalf("no built-in rule with ID %q", id)
	return rules.Registration{}
}
