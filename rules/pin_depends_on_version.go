package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/feature"
	"github.com/bare-devcontainer/decolint/linter"
)

// PinDependsOnVersion reports a "dependsOn" entry of a devcontainer-feature.json whose key
// references an OCI Feature without an explicit version tag or with the "latest" tag. It is the
// [PinFeatureVersion] problem where a Feature declares its own dependencies, one step further from
// the project that ends up installing them.
var PinDependsOnVersion = &linter.Rule{
	ID:          "pin-depends-on-version",
	Description: `disallow a "dependsOn" Feature reference without an explicit version or with the "latest" version`,
	LongDescription: `"dependsOn" names the Features the tooling installs before this one, and it takes the same references
"features" does: an entry with no version resolves to "latest". A Feature published that way installs a
moving dependency into every project that uses it, and the project cannot pin it — the reference is in the
Feature, not in their devcontainer.json. Whatever the dependency changes about the container appears
without the Feature's version changing.`,
	References: []string{
		`https://containers.dev/implementors/features/#referencing-a-feature`,
		`https://containers.dev/implementors/features-distribution/#versioning`,
	},
	Category:  linter.CategoryReproducibility,
	FileTypes: []linter.FileType{linter.Feature},
	Paths:     []string{"/dependsOn"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer-feature.json`, Content: `{
  "id": "my-feature",
  "version": "1.0.0",
  "name": "My Feature",
  "dependsOn": {
    "ghcr.io/devcontainers/features/common-utils": {}
  }
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer-feature.json`, Content: `{
  "id": "my-feature",
  "version": "1.0.0",
  "name": "My Feature",
  "dependsOn": {
    "ghcr.io/devcontainers/features/common-utils:2.6.2": {}
  }
}
`},
			},
		},
		Note: "`installsAfter` is not checked. It only orders Features the configuration already\n" +
			"installs, so the reference selects nothing to pin a version of.",
	},
	Check: checkPinDependsOnVersion,
}

func checkPinDependsOnVersion(_ *linter.Context, node *linter.Node) []linter.Finding {
	var findings []linter.Finding
	for _, f := range featureRefsOfKind(node.Value, feature.KindOCI) {
		tag, hasTag := refTag(f.ref)
		switch {
		case !hasTag:
			findings = append(findings, linter.Finding{
				Message: fmt.Sprintf(`"dependsOn" feature %q has no explicit version; pin a specific version`, f.ref),
				Offset:  f.offset,
			})
		case tag == "latest":
			findings = append(findings, linter.Finding{
				Message: fmt.Sprintf(`"dependsOn" feature %q uses the "latest" version; pin a specific version`, f.ref),
				Offset:  f.offset,
			})
		}
	}
	return findings
}
