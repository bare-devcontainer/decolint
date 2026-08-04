package rules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
)

// exactFeatureVersion matches a full "major.minor.patch" Feature version, with the optional
// prerelease suffix semver allows and an OCI tag can spell. The build metadata semver also allows
// is not matched: "+" is not a legal character in a tag, so no Feature is published under one.
var exactFeatureVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// PinFeatureExactVersion reports a Feature reference that names something other than one published
// version, in a devcontainer.json's "features" or a Feature's "dependsOn". Unlike
// [PinFeatureVersion], which accepts any version, this rule accepts only a full
// "major.minor.patch", since the shorter tags are reassigned as the Feature is released.
var PinFeatureExactVersion = &linter.Rule{
	ID:          "pin-feature-exact-version",
	Description: `disallow a Feature reference that does not pin a full "major.minor.patch" version`,
	LongDescription: `A Feature is published under its full version and under the "major" and "major.minor" tags, and the
publisher moves those shorter tags with every release. So ` + "`:1`" + ` installs whatever 1.x is current at build
time: the Feature can change what it installs, add options, or change their defaults, and the container
changes with it. Only the full version names one published Feature for good.

A reference pinned by digest is accepted as it already names exact content.`,
	References: []string{
		`https://containers.dev/implementors/features-distribution/#versioning`,
		`https://containers.dev/implementors/features/#referencing-a-feature`,
	},
	Category:  linter.CategoryReproducibility,
	FileTypes: []linter.FileType{linter.Devcontainer, linter.Feature},
	Paths:     []string{"/features", "/dependsOn"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu-24.04",
  "features": {
    "ghcr.io/devcontainers/features/go:1": {}
  }
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer.json`, Content: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu-24.04",
  "features": {
    "ghcr.io/devcontainers/features/go:1.3.2": {}
  }
}
`},
			},
		},
		Note: "A Feature's own `dependsOn` entries are checked the same way, so a Feature pins the\n" +
			"Features it pulls in as tightly as a project pins the ones it asks for.",
	},
	Check: checkPinFeatureExactVersion,
}

func checkPinFeatureExactVersion(_ *linter.Context, node *linter.Node) []linter.Finding {
	var findings []linter.Finding
	for _, f := range ociFeatureRefs(node.Value) {
		// A digest names the content itself, whatever tag it is written alongside.
		if strings.Contains(f.ref, "@") {
			continue
		}
		tag, hasTag := refTag(f.ref)
		switch {
		case !hasTag:
			findings = append(findings, linter.Finding{
				Message: fmt.Sprintf(`feature %q has no explicit version; pin a full "major.minor.patch" version`, f.ref),
				Offset:  f.offset,
			})
		case !exactFeatureVersion.MatchString(tag):
			findings = append(findings, linter.Finding{
				Message: fmt.Sprintf(`feature %q uses version %q; pin a full "major.minor.patch" version`, f.ref, tag),
				Offset:  f.offset,
			})
		}
	}
	return findings
}
