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
//
// An entry is excused when another entry in the same list pins the same extension ID: the version
// is resolved per extension ID, so the pin decides which one is installed. That is what the merged
// configuration looks like when a Feature contributes an extension unpinned and the
// devcontainer.json pins the same ID, which is the only way to pin a Feature's extension.
var PinExtensionVersion = &linter.Rule{
	ID:          "pin-extension-version",
	Description: `disallow a "customizations.vscode.extensions" entry without an explicit pinned version`,
	LongDescription: `An extension ID on its own installs whatever the marketplace publishes at the moment the container is
created, so two developers on the same devcontainer.json can end up with different formatters, linters, or
language server versions — and an extension update can change the environment without any commit.
Appending a version (` + "`publisher.name@1.2.3`" + `) makes the editor tooling as pinned as the rest of the image.

A Feature can contribute extensions of its own, and its entries cannot be edited from the devcontainer.json.
Listing the same extension ID with a version pins it all the same, since the version is resolved per
extension ID, and the Feature's entry is then not reported.`,
	References: []string{
		`https://containers.dev/supporting#visual-studio-code`,
		`https://code.visualstudio.com/docs/configure/extensions/extension-marketplace`,
	},
	Category:  linter.CategoryReproducibility,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Platforms: []linter.Platform{linter.PlatformVSCode, linter.PlatformCodespaces},
	Paths:     []string{"/customizations/vscode/extensions"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "customizations": {
    "vscode": {
      "extensions": ["golang.go"]
    }
  }
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "customizations": {
    "vscode": {
      "extensions": ["golang.go@0.50.0"]
    }
  }
}
`},
			},
		},
	},
	Check: checkPinExtensionVersion,
}

func checkPinExtensionVersion(_ *linter.Context, node *linter.Node) []linter.Finding {
	arr, ok := node.Value.Value.(*hujson.Array)
	if !ok {
		return nil
	}

	pinned := map[string]bool{}
	for i := range arr.Elements {
		ref, ok := extensionRef(&arr.Elements[i])
		if !ok {
			continue
		}
		if id, version := splitExtensionRef(ref); version != "" {
			pinned[id] = true
		}
	}

	var findings []linter.Finding
	for i := range arr.Elements {
		ref, ok := extensionRef(&arr.Elements[i])
		if !ok {
			continue
		}
		id, version := splitExtensionRef(ref)
		if version != "" || pinned[id] {
			continue
		}
		findings = append(findings, linter.Finding{
			Message: fmt.Sprintf("extension %q has no explicit version; pin a specific version", ref),
			Offset:  arr.Elements[i].StartOffset,
		})
	}
	return findings
}

// extensionRef returns v as the string a "customizations.vscode.extensions" entry is written as. ok
// is false for an entry that is not a string.
func extensionRef(v *hujson.Value) (ref string, ok bool) {
	lit, ok := v.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return "", false
	}
	return lit.String(), true
}

// splitExtensionRef splits an extension entry into the ID it names and the version it pins, empty
// when it pins none. The ID is lower-cased, as extension identifiers are matched
// case-insensitively.
func splitExtensionRef(ref string) (id, version string) {
	id, version, _ = strings.Cut(ref, "@")
	return strings.ToLower(id), version
}
