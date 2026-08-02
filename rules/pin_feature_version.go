package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
)

// PinFeatureVersion reports a "features" entry whose key references an OCI Feature without an
// explicit version tag or with the "latest" tag. Such references are not reproducible: the Feature
// they resolve to changes over time. Local path Features (e.g. "./my-feature") and direct tarball
// URIs (e.g. "https://.../devcontainer-feature.tgz") have no version tag to pin and are not
// checked.
var PinFeatureVersion = &linter.Rule{
	ID:          "pin-feature-version",
	Description: `disallow a Feature reference without an explicit version or with the "latest" version`,
	LongDescription: `A Feature reference with no version resolves to "latest", so the container installs whatever the Feature's
author published most recently — the tooling it sets up can change under the project without the
devcontainer.json changing at all. Features are published under their full version as well as
"major.minor" and "major" tags, so a reference can be pinned as tightly as the project wants.`,
	References: []string{
		`https://containers.dev/implementors/features-distribution/#versioning`,
		`https://containers.dev/implementors/features/#referencing-a-feature`,
	},
	Category:  linter.CategoryReproducibility,
	FileTypes: []linter.FileType{linter.Devcontainer},
	Paths:     []string{"/features"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/go": {}
  }
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/go:1.3.2": {}
  }
}
`},
			},
		},
	},
	Check: checkPinFeatureVersion,
}

func checkPinFeatureVersion(_ *linter.Context, node *linter.Node) []linter.Finding {
	var findings []linter.Finding
	for _, f := range ociFeatureRefs(node.Value) {
		problem := unpinnedFeatureVersion(f.ref)
		if problem == "" {
			continue
		}
		findings = append(findings, linter.Finding{
			Message: fmt.Sprintf("feature %q %s", f.ref, problem),
			Offset:  f.offset,
		})
	}
	return findings
}
