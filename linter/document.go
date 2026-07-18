package linter

import (
	"fmt"

	"github.com/tailscale/hujson"
)

// Document is a parsed configuration file together with the indexes rule application needs: the
// syntax tree, a byte-offset-to-line resolver, and the ignore directives found in it. Produce one
// with ParseDocument, then apply rules with Linter.LintDocument. The position and ignore indexes
// are internal; the syntax tree is exposed via Tree.
type Document struct {
	tree    *hujson.Value
	pos     *positions
	ignores *ignoreIndex
}

// ParseDocument parses src as HuJSON and precomputes the line index and ignore directives used when
// applying rules. Ignore directives are read from the source as authored, before any mutation of
// the tree returned by Tree.
func ParseDocument(src []byte) (*Document, error) {
	tree, err := hujson.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse HuJSON: %w", err)
	}
	pos := newPositions(src)
	return &Document{tree: &tree, pos: pos, ignores: buildIgnoreIndex(&tree, pos)}, nil
}

// Tree returns the file's HuJSON syntax tree. It may be mutated before LintDocument runs, e.g. to
// merge Feature-contributed properties into the effective configuration, but any node added must
// carry offsets pointing into the original source, since findings are still positioned against
// that source. Ignore directives are read at parse time and unaffected by later mutation.
func (d *Document) Tree() *hujson.Value { return d.tree }
