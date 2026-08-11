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
		{"pinned pre-release build", `{"customizations": {"vscode": {"extensions": ["golang.go@0.54.0-beta.1"]}}}`, nil},
		{"prerelease extension", `{"customizations": {"vscode": {"extensions": ["golang.go@prerelease"]}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 47, RuleID: "pin-extension-version", Message: `extension "golang.go@prerelease" uses the "prerelease" version; pin a specific version`},
		}},
		{"non-string extension entry", `{"customizations": {"vscode": {"extensions": [123]}}}`, nil},
		// A contributed unpinned entry, which the merge appends alongside the user's pinned one.
		{"same extension pinned by another entry", `{"customizations": {"vscode": {"extensions": ["golang.go", "golang.go@0.54.0"]}}}`, nil},
		{"same extension pinned by an earlier entry", `{"customizations": {"vscode": {"extensions": ["golang.go@0.54.0", "golang.go"]}}}`, nil},
		{"same extension pinned in another case", `{"customizations": {"vscode": {"extensions": ["golang.Go", "GOLANG.go@0.54.0"]}}}`, nil},
		{"prerelease does not pin the extension", `{"customizations": {"vscode": {"extensions": [
  "golang.go",
  "golang.go@prerelease"
]}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 2, Col: 3, RuleID: "pin-extension-version", Message: `extension "golang.go" has no explicit version; pin a specific version`},
			{Path: "devcontainer.json", Line: 3, Col: 3, RuleID: "pin-extension-version", Message: `extension "golang.go@prerelease" uses the "prerelease" version; pin a specific version`},
		}},
		// A suffix the editor does not read as a version belongs to the ID, so it names another
		// extension entirely and the pinned entry does not cover it.
		{"unread version suffix", `{"customizations": {"vscode": {"extensions": [
  "golang.go@latest",
  "golang.go@0.54.0"
]}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 2, Col: 3, RuleID: "pin-extension-version", Message: `extension "golang.go@latest" has no explicit version; pin a specific version`},
		}},
		{"version without an extension ID", `{"customizations": {"vscode": {"extensions": [
  "",
  "@0.54.0"
]}}}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 2, Col: 3, RuleID: "pin-extension-version", Message: `extension "" has no explicit version; pin a specific version`},
			{Path: "devcontainer.json", Line: 3, Col: 3, RuleID: "pin-extension-version", Message: `extension "@0.54.0" has no explicit version; pin a specific version`},
		}},
		{"non-array extensions", `{"customizations": {"vscode": {"extensions": "invalid"}}}`, nil},
		{"object extensions", `{"customizations": {"vscode": {"extensions": {"golang.go": true}}}}`, nil},
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
			assertIssues(t, rules.PinExtensionVersion, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}
