package rules

import (
	"fmt"
	"path/filepath"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// IDDirMismatch reports a Feature's or Template's "id" property when it does not match the name of
// the directory containing its metadata file, per the Dev Container Features/Templates convention.
type IDDirMismatch struct{}

// ID implements [linter.Rule].
func (IDDirMismatch) ID() string { return "id-dir-mismatch" }

// Description implements [linter.Rule].
func (IDDirMismatch) Description() string {
	return `disallow a Feature's or Template's "id" that does not match the name of its containing directory`
}

// FileTypes implements [linter.Rule].
func (IDDirMismatch) FileTypes() []linter.FileType {
	return []linter.FileType{linter.Feature, linter.Template}
}

// Platforms implements [linter.Rule].
func (IDDirMismatch) Platforms() []linter.Platform { return nil }

// Paths implements [linter.Rule].
func (IDDirMismatch) Paths() []string { return []string{"/id"} }

// Check implements [linter.Rule].
func (r IDDirMismatch) Check(ctx *linter.Context, node *linter.Node) []linter.Finding {
	lit, ok := node.Value.Value.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return nil
	}
	id := lit.String()
	dir := filepath.Base(filepath.Dir(ctx.Path))
	if id == dir {
		return nil
	}
	return []linter.Finding{{
		RuleID:  r.ID(),
		Message: fmt.Sprintf("id %q does not match containing directory %q", id, dir),
		Offset:  node.Value.StartOffset,
	}}
}
