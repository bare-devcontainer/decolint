package rules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// PinExtensionVersion reports a "customizations.vscode.extensions" entry that does not fix the
// build the container installs: one naming no version, or one naming "prerelease", which tracks the
// extension's newest pre-release build.
//
// An entry is not reported when another entry in the same list pins the same extension ID, since
// the version is resolved per ID. That is what the list looks like once an unpinned entry a Feature
// or the base image contributes and a pinned one for the same extension are merged into it.
var PinExtensionVersion = &linter.Rule{
	ID:          "pin-extension-version",
	Description: `disallow a "customizations.vscode.extensions" entry without an explicit pinned version`,
	LongDescription: `An extension ID on its own installs whatever the marketplace publishes at the moment the container is
created, so two developers on the same devcontainer.json can end up with different formatters, linters, or
language server versions — and an extension update can change the environment without any commit.
Appending a version (` + "`publisher.name@1.2.3`" + `) makes the editor tooling as pinned as the rest of the image.
` + "`publisher.name@prerelease`" + ` is reported as well: it follows the newest pre-release build.

An entry without a version is not reported when another entry in the same list pins the same extension ID,
since the version is resolved per ID. That is how to pin an extension a Feature or the base image
contributes, which ` + "`--merge`" + ` folds into this list: list the ID again with the version you want.`,
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
		if id, version := splitExtensionRef(ref); isPinnedVersion(version) {
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
		if isPinnedVersion(version) || pinned[id] {
			continue
		}
		message := fmt.Sprintf("extension %q has no explicit version; pin a specific version", ref)
		if version == prereleaseVersion {
			message = fmt.Sprintf("extension %q uses the %q version; pin a specific version", ref, prereleaseVersion)
		}
		findings = append(findings, linter.Finding{Message: message, Offset: arr.Elements[i].StartOffset})
	}
	return findings
}

// prereleaseVersion is the version an entry names to track the extension's newest pre-release build.
const prereleaseVersion = "prerelease"

// extensionRefPattern matches an extension entry that names a version, capturing the extension ID
// and the version. It mirrors the editor's own, which is what makes a suffix a version rather than
// part of the ID.
var extensionRefPattern = regexp.MustCompile(`^([^.]+\..+)@(` + prereleaseVersion + `|\d+\.\d+\.\d+(?:-.*)?)$`)

// extensionRef returns v as the string a "customizations.vscode.extensions" entry is written as. ok
// is false for an entry that is not a string.
func extensionRef(v *hujson.Value) (ref string, ok bool) {
	lit, ok := v.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return "", false
	}
	return lit.String(), true
}

// splitExtensionRef splits an extension entry into the extension ID it names and the version it
// selects, as the editor reads it (see [extensionRefPattern]): a suffix that is not one of the
// versions it accepts stays part of the ID, so "publisher.name@latest" names an extension of that
// name rather than a version of "publisher.name". The ID is lower-cased, as extension identifiers
// are matched case-insensitively.
func splitExtensionRef(ref string) (id, version string) {
	if m := extensionRefPattern.FindStringSubmatch(ref); m != nil {
		return strings.ToLower(m[1]), m[2]
	}
	return strings.ToLower(ref), ""
}

// isPinnedVersion reports whether version, as returned by [splitExtensionRef], fixes the build that
// is installed. The pre-release version does not: it tracks the newest pre-release build.
func isPinnedVersion(version string) bool {
	return version != "" && version != prereleaseVersion
}
