package rules

import (
	"fmt"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// PinExtensionVersion reports a "customizations.vscode.extensions" entry that does not pin an
// explicit version (e.g. "publisher.name@1.2.3"). Without a pinned version, the VS Code Dev
// Containers extension and GitHub Codespaces always install the latest published version, which is
// not reproducible.
type PinExtensionVersion struct{}

// ID implements [linter.Rule].
func (PinExtensionVersion) ID() string { return "pin-extension-version" }

// Description implements [linter.Rule].
func (PinExtensionVersion) Description() string {
	return `disallow a "customizations.vscode.extensions" entry without an explicit pinned version`
}

// FileTypes implements [linter.Rule].
func (PinExtensionVersion) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Devcontainer}
}

// Platforms implements [linter.Rule].
func (PinExtensionVersion) Platforms() []linter.Platform {
	return []linter.Platform{linter.PlatformVSCode, linter.PlatformCodespaces}
}

// Paths implements [linter.Rule].
func (PinExtensionVersion) Paths() []string {
	return []string{"/customizations/vscode/extensions/*"}
}

// Check implements [linter.Rule].
func (PinExtensionVersion) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}
	ref := lit.String()
	if _, version, ok := strings.Cut(ref, "@"); ok && version != "" {
		return nil
	}
	return []linter.Finding{{
		Message: fmt.Sprintf("extension %q has no explicit version; pin a specific version", ref),
		Offset:  node.Value.StartOffset,
	}}
}
