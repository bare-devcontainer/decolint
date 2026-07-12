package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// MissingRequiredProps reports a Feature's or Template's metadata when it is missing a required
// property ("id", "version", or "name").
type MissingRequiredProps struct{}

// ID implements [linter.Rule].
func (MissingRequiredProps) ID() string { return "missing-required-props" }

// Description implements [linter.Rule].
func (MissingRequiredProps) Description() string {
	return `disallow a Feature's or Template's metadata that is missing a required property ("id", "version", or "name")`
}

// FileTypes implements [linter.Rule].
func (MissingRequiredProps) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Feature, linter.Template}
}

// Platforms implements [linter.Rule].
func (MissingRequiredProps) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (MissingRequiredProps) Paths() []string { return []string{""} }

// Check implements [linter.Rule].
func (MissingRequiredProps) Check(_ *linter.Context, node *linter.Node) []linter.Finding {
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
