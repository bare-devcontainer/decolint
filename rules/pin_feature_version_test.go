package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestPinFeatureVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no features property", `{"name": "test"}`, nil},
		{"untagged feature", `{"features": {"ghcr.io/devcontainers/features/node": {}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 15, RuleID: "pin-feature-version", Message: `feature "ghcr.io/devcontainers/features/node" has no explicit version; pin a specific version`},
		}},
		{"latest feature", `{"features": {"ghcr.io/devcontainers/features/node:latest": {}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 15, RuleID: "pin-feature-version", Message: `feature "ghcr.io/devcontainers/features/node:latest" uses the "latest" version; pin a specific version`},
		}},
		{"pinned version", `{"features": {"ghcr.io/devcontainers/features/node:1": {}}}`, nil},
		{"pinned digest", `{"features": {"ghcr.io/devcontainers/features/node@sha256:abc123": {}}}`, nil},
		{"local path feature", `{"features": {"./local-feature": {}}}`, nil},
		{"tarball uri feature", `{"features": {"https://example.com/devcontainer-feature.tgz": {}}}`, nil},
		{"registry port without tag", `{"features": {"localhost:5000/features/foo": {}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 15, RuleID: "pin-feature-version", Message: `feature "localhost:5000/features/foo" has no explicit version; pin a specific version`},
		}},
		{"registry port with tag", `{"features": {"localhost:5000/features/foo:1": {}}}`, nil},
		{"non-object features", `{"features": "invalid"}`, nil},
		{"multiple features mixed", `{"features": {
  "ghcr.io/devcontainers/features/node:1": {},
  "ghcr.io/devcontainers/features/go": {}
}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 3, Col: 3, RuleID: "pin-feature-version", Message: `feature "ghcr.io/devcontainers/features/go" has no explicit version; pin a specific version`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.PinFeatureVersion, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}
