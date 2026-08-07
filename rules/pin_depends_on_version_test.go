package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestPinDependsOnVersion(t *testing.T) {
	t.Parallel()

	const path = "devcontainer-feature.json"

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no dependsOn property", `{"id": "my-feature"}`, nil},
		{"untagged dependency", `{"dependsOn": {"ghcr.io/devcontainers/features/node": {}}}`, []linter.Issue{
			{Path: path, Line: 1, Col: 16, RuleID: "pin-depends-on-version", Message: `"dependsOn" feature "ghcr.io/devcontainers/features/node" has no explicit version; pin a specific version`},
		}},
		{"latest dependency", `{"dependsOn": {"ghcr.io/devcontainers/features/node:latest": {}}}`, []linter.Issue{
			{Path: path, Line: 1, Col: 16, RuleID: "pin-depends-on-version", Message: `"dependsOn" feature "ghcr.io/devcontainers/features/node:latest" uses the "latest" version; pin a specific version`},
		}},
		{"pinned version", `{"dependsOn": {"ghcr.io/devcontainers/features/node:1": {}}}`, nil},
		{"pinned digest", `{"dependsOn": {"ghcr.io/devcontainers/features/node@sha256:0000000000000000000000000000000000000000000000000000000000000000": {}}}`, nil},
		{"local path dependency", `{"dependsOn": {"./local-feature": {}}}`, nil},
		{"tarball uri dependency", `{"dependsOn": {"https://example.invalid/devcontainer-feature.tgz": {}}}`, nil},
		{"non-object dependsOn", `{"dependsOn": "invalid"}`, nil},
		// A reference in none of the three forms the specification defines names no Feature.
		{"absolute path dependency", `{"dependsOn": {"/absolute/feature": {}}}`, nil},
		{"dependency with no registry", `{"dependsOn": {"no-slash": {}}}`, nil},
		{"installsAfter is not checked", `{"installsAfter": ["ghcr.io/devcontainers/features/node"]}`, nil},
		{"multiple dependencies mixed", `{"dependsOn": {
  "ghcr.io/devcontainers/features/node:1.6.0": {},
  "ghcr.io/devcontainers/features/go": {}
}}`, []linter.Issue{
			{Path: path, Line: 3, Col: 3, RuleID: "pin-depends-on-version", Message: `"dependsOn" feature "ghcr.io/devcontainers/features/go" has no explicit version; pin a specific version`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesAt(t, rules.PinDependsOnVersion, linter.SeverityWarn, path, linter.Feature, tt.src, tt.want)
		})
	}
}
