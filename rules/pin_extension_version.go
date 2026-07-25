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
var PinExtensionVersion = &linter.Rule{
	ID:          "pin-extension-version",
	Description: `disallow a "customizations.vscode.extensions" entry without an explicit pinned version`,
	LongDescription: `An extension ID on its own installs whatever the marketplace publishes at the moment the container is
created, so two developers on the same devcontainer.json can end up with different formatters, linters, or
language server versions — and an extension update can change the environment without any commit.
Appending a version ("publisher.name@1.2.3") makes the editor tooling as pinned as the rest of the image.`,
	References: []string{
		"https://github.com/devcontainers/spec/blob/main/docs/specs/supporting-tools.md#visual-studio-code",
		"https://code.visualstudio.com/docs/configure/extensions/extension-marketplace",
	},
	Category:  linter.CategoryReproducibility,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Platforms: []linter.Platform{linter.PlatformVSCode, linter.PlatformCodespaces},
	Paths:     []string{"/customizations/vscode/extensions/*"},
	Check:     checkPinExtensionVersion,
}

func checkPinExtensionVersion(_ *linter.Context, node *linter.Node) []linter.Finding {
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
