package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestPinFeatureExactVersion(t *testing.T) {
	t.Parallel()

	issue := func(message string) []linter.Issue {
		return []linter.Issue{{Path: "devcontainer.json", Line: 1, Col: 15, RuleID: "pin-feature-exact-version", Message: message}}
	}

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no features property", `{"name": "test"}`, nil},
		{
			"major only",
			`{"features": {"ghcr.io/devcontainers/features/go:1": {}}}`,
			issue(`feature "ghcr.io/devcontainers/features/go:1" uses version "1"; pin a full "major.minor.patch" version`),
		},
		{
			"major and minor only",
			`{"features": {"ghcr.io/devcontainers/features/go:1.3": {}}}`,
			issue(`feature "ghcr.io/devcontainers/features/go:1.3" uses version "1.3"; pin a full "major.minor.patch" version`),
		},
		{
			"latest",
			`{"features": {"ghcr.io/devcontainers/features/go:latest": {}}}`,
			issue(`feature "ghcr.io/devcontainers/features/go:latest" uses version "latest"; pin a full "major.minor.patch" version`),
		},
		{
			"no version",
			`{"features": {"ghcr.io/devcontainers/features/go": {}}}`,
			issue(`feature "ghcr.io/devcontainers/features/go" has no explicit version; pin a full "major.minor.patch" version`),
		},
		{"full version", `{"features": {"ghcr.io/devcontainers/features/go:1.3.2": {}}}`, nil},
		{"full version with a prerelease", `{"features": {"ghcr.io/devcontainers/features/go:1.3.2-beta.1": {}}}`, nil},
		{"a zero component is a version", `{"features": {"ghcr.io/devcontainers/features/go:0.1.0": {}}}`, nil},
		{
			// semver forbids a leading zero, so no Feature is published under such a version.
			"leading zero",
			`{"features": {"ghcr.io/devcontainers/features/go:01.2.3": {}}}`,
			issue(`feature "ghcr.io/devcontainers/features/go:01.2.3" uses version "01.2.3"; pin a full "major.minor.patch" version`),
		},
		{
			"a v prefix is not a version",
			`{"features": {"ghcr.io/devcontainers/features/go:v1.3.2": {}}}`,
			issue(`feature "ghcr.io/devcontainers/features/go:v1.3.2" uses version "v1.3.2"; pin a full "major.minor.patch" version`),
		},
		{"digest alone", `{"features": {"ghcr.io/devcontainers/features/go@sha256:abc123": {}}}`, nil},
		{"partial version alongside a digest", `{"features": {"ghcr.io/devcontainers/features/go:1@sha256:abc123": {}}}`, nil},
		{"local path feature", `{"features": {"./local-feature": {}}}`, nil},
		{"tarball uri feature", `{"features": {"https://example.invalid/devcontainer-feature.tgz": {}}}`, nil},
		{"non-object features", `{"features": "invalid"}`, nil},
		{
			"registry port without a version",
			`{"features": {"localhost:5000/features/foo": {}}}`,
			issue(`feature "localhost:5000/features/foo" has no explicit version; pin a full "major.minor.patch" version`),
		},
		{"registry port with a full version", `{"features": {"localhost:5000/features/foo:1.0.0": {}}}`, nil},
		{
			// "dependsOn" is a Feature's property; a devcontainer.json asks for Features under
			// "features" alone, so a member spelled that way holds no Feature reference.
			"a dependsOn member of a devcontainer.json is not a Feature reference",
			`{"dependsOn": {"ghcr.io/devcontainers/features/go": {}}}`,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.PinFeatureExactVersion, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}

// TestPinFeatureExactVersion_DependsOn checks the rule where a Feature names the Features it pulls
// in, the other place a Feature reference is written.
func TestPinFeatureExactVersion_DependsOn(t *testing.T) {
	t.Parallel()

	const path = "devcontainer-feature.json"

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{
			"major only",
			`{"dependsOn": {"ghcr.io/devcontainers/features/common-utils:2": {}}}`,
			[]linter.Issue{{Path: path, Line: 1, Col: 16, RuleID: "pin-feature-exact-version", Message: `feature "ghcr.io/devcontainers/features/common-utils:2" uses version "2"; pin a full "major.minor.patch" version`}},
		},
		{"full version", `{"dependsOn": {"ghcr.io/devcontainers/features/common-utils:2.6.2": {}}}`, nil},
		{"no dependsOn property", `{"id": "my-feature"}`, nil},
		{
			// A Feature declares its dependencies under "dependsOn"; the specification gives it no
			// "features" property, so a member spelled that way holds no Feature reference.
			"a features member of a Feature is not a Feature reference",
			`{"features": {"ghcr.io/devcontainers/features/common-utils": {}}}`,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.PinFeatureExactVersion, linter.SeverityWarn, path, linter.Feature, tt.src, tt.want)
		})
	}
}
