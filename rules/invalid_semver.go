package rules

import (
	"fmt"
	"regexp"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// InvalidSemver reports a Feature's or Template's "version" property when its value is not a valid
// semantic version, per the Dev Container Features/Templates specification, which requires
// "version" to follow the semver.org format.
type InvalidSemver struct{}

// ID implements [linter.Rule].
func (InvalidSemver) ID() string { return "invalid-semver" }

// Description implements [linter.Rule].
func (InvalidSemver) Description() string {
	return `disallow a Feature's or Template's "version" that is not a valid semantic version`
}

// FileTypes implements [linter.Rule].
func (InvalidSemver) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Feature, linter.Template}
}

// Platforms implements [linter.Rule].
func (InvalidSemver) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (InvalidSemver) Paths() []string { return []string{"/version"} }

// semverPattern is the official semantic version regular expression published at
// https://semver.org/.
var semverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// Check implements [linter.Rule].
func (r InvalidSemver) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}
	version := lit.String()
	if semverPattern.MatchString(version) {
		return nil
	}
	return []linter.Finding{{
		RuleID:  r.ID(),
		Message: fmt.Sprintf("version %q is not a valid semantic version (see https://semver.org/)", version),
		Offset:  node.Value.StartOffset,
	}}
}
