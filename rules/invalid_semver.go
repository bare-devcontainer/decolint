package rules

import (
	"fmt"
	"regexp"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// InvalidSemver reports a Feature's or Template's "version" property when its value is not a valid
// semantic version, per the Dev Container Features/Templates specification.
var InvalidSemver = &linter.Rule{
	ID:          "invalid-semver",
	Description: `disallow a Feature's or Template's "version" that is not a valid semantic version`,
	LongDescription: `Publishing a Feature or Template pushes it under tags derived from the "version" components: the full
version, "major.minor", and "major", so consumers can pin as loosely or as tightly as they want. A value
that is not valid semver has no such components, leaving nothing to derive those tags from.`,
	References: []string{
		`https://containers.dev/implementors/features-distribution/#versioning`,
		`https://semver.org/`,
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Feature, linter.Template},
	Paths:     []string{"/version"},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer-feature.json`, Content: `{
  "id": "node",
  "version": "1.0",
  "name": "Node.js"
}
`},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{Path: `devcontainer-feature.json`, Content: `{
  "id": "node",
  "version": "1.0.0",
  "name": "Node.js"
}
`},
			},
		},
	},
	Check: checkInvalidSemver,
}

// semverPattern is the official semantic version regular expression published at
// https://semver.org/.
var semverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

func checkInvalidSemver(_ *linter.Context, node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}
	version := lit.String()
	if semverPattern.MatchString(version) {
		return nil
	}
	return []linter.Finding{{
		Message: fmt.Sprintf("version %q is not a valid semantic version (see https://semver.org/)", version),
		Offset:  node.Value.StartOffset,
	}}
}
