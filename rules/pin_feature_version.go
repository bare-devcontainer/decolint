package rules

import (
	"fmt"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
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
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}

	var findings []linter.Finding
	for _, m := range obj.Members {
		name, ok := m.Name.Value.(hujson.Literal)
		if !ok || name.Kind() != '"' {
			continue
		}
		ref := name.String()
		if isLocalFeature(ref) || isTarballFeature(ref) {
			continue
		}

		tag, hasTag := refTag(ref)
		switch {
		case !hasTag:
			findings = append(findings, linter.Finding{
				Message: fmt.Sprintf("feature %q has no explicit version; pin a specific version", ref),
				Offset:  m.Name.StartOffset,
			})
		case tag == "latest":
			findings = append(findings, linter.Finding{
				Message: fmt.Sprintf("feature %q uses the \"latest\" version; pin a specific version", ref),
				Offset:  m.Name.StartOffset,
			})
		}
	}
	return findings
}

// isLocalFeature reports whether ref names a Feature by a relative path, which has no version tag
// to pin.
func isLocalFeature(ref string) bool {
	return strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../")
}

// isTarballFeature reports whether ref names a Feature by a direct HTTP(S) URI to a tarball, which
// has no version tag to pin.
func isTarballFeature(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://")
}
