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
	Category:    linter.CategoryCorrectness,
	FileTypes:   []linter.FileType{linter.Feature, linter.Template},
	Paths:       []string{"/version"},
	Check:       checkInvalidSemver,
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
