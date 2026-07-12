package rules

import (
	"fmt"
	"path/filepath"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// IDDirMismatch reports a Feature's or Template's "id" property when it does not match the name of
// the directory containing its metadata file, per the Dev Container Features/Templates convention.
var IDDirMismatch = &linter.Rule{
	ID:          "id-dir-mismatch",
	Description: `disallow a Feature's or Template's "id" that does not match the name of its containing directory`,
	Category:    linter.CategoryCorrectness,
	FileTypes:   []linter.FileType{linter.Feature, linter.Template},
	Paths:       []string{"/id"},
	Check:       checkIDDirMismatch,
}

func checkIDDirMismatch(ctx *linter.Context, node *linter.Node) []linter.Finding {
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
		Message: fmt.Sprintf("id %q does not match containing directory %q", id, dir),
		Offset:  node.Value.StartOffset,
	}}
}
