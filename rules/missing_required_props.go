package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// MissingRequiredProps reports a Feature's or Template's metadata when it is missing a required
// property ("id", "version", or "name").
var MissingRequiredProps = &linter.Rule{
	ID:          "missing-required-props",
	Description: `disallow a Feature's or Template's metadata that is missing a required property ("id", "version", or "name")`,
	LongDescription: `"id", "version", and "name" are the only properties either specification requires: the "id" addresses the
artifact, the "version" is what consumers pin to, and the "name" is what a user recognizes it by in a
list. Metadata missing any of them cannot be published as a usable Feature or Template.`,
	References: []string{
		"https://containers.dev/implementors/features/#devcontainer-featurejson-properties",
		"https://containers.dev/implementors/templates/#devcontainer-templatejson-properties",
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Feature, linter.Template},
	Paths:     []string{""},
	Check:     checkMissingRequiredProps,
}

func checkMissingRequiredProps(_ *linter.Context, node *linter.Node) []linter.Finding {
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	var findings []linter.Finding
	for _, name := range []string{"id", "version", "name"} {
		if hasMember(obj, name) {
			continue
		}
		findings = append(findings, linter.Finding{
			Message: fmt.Sprintf("required property %q is missing", name),
			Offset:  node.Value.StartOffset,
		})
	}
	return findings
}
