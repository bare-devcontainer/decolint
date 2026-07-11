package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestPinExtensionVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no customizations property", `{"name": "test"}`, nil},
		{"no vscode customization", `{"customizations": {"codespaces": {}}}`, nil},
		{"no extensions property", `{"customizations": {"vscode": {}}}`, nil},
		{"unpinned extension", `{"customizations": {"vscode": {"extensions": ["golang.go"]}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 47, RuleID: "pin-extension-version", Message: `extension "golang.go" has no explicit version; pin a specific version`},
		}},
		{"pinned extension", `{"customizations": {"vscode": {"extensions": ["golang.go@0.54.0"]}}}`, nil},
		{"trailing @ with empty version", `{"customizations": {"vscode": {"extensions": ["golang.go@"]}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 47, RuleID: "pin-extension-version", Message: `extension "golang.go@" has no explicit version; pin a specific version`},
		}},
		{"non-string extension entry", `{"customizations": {"vscode": {"extensions": [123]}}}`, nil},
		{"non-array extensions", `{"customizations": {"vscode": {"extensions": "invalid"}}}`, nil},
		{"multiple extensions mixed", `{"customizations": {"vscode": {"extensions": [
  "golang.go@0.54.0",
  "EditorConfig.EditorConfig"
]}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 3, Col: 3, RuleID: "pin-extension-version", Message: `extension "EditorConfig.EditorConfig" has no explicit version; pin a specific version`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.PinExtensionVersion{}, linter.Warn, tt.src, tt.want)
		})
	}
}
