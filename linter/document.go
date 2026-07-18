package linter

import (
	"fmt"

	"github.com/tailscale/hujson"
)

// Document is a parsed configuration file together with the indexes rule application needs: the
// syntax tree, a byte-offset-to-line resolver, and the ignore directives found in it. Produce one
// with ParseDocument, then apply rules with Linter.LintDocument. It is opaque; its contents are
// an implementation detail of how rules are applied.
type Document struct {
	tree    *hujson.Value
	pos     *positions
	ignores *ignoreIndex
}

// ParseDocument parses src as HuJSON and precomputes the line index and ignore directives used when
// applying rules. Ignore directives are read from the source as authored, before any Transform runs.
func ParseDocument(src []byte) (*Document, error) {
	tree, err := hujson.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse HuJSON: %w", err)
	}
	pos := newPositions(src)
	return &Document{tree: &tree, pos: pos, ignores: buildIgnoreIndex(&tree, pos)}, nil
}
