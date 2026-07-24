package linter

import (
	"fmt"
	"strconv"

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

// Position converts a byte offset into the source into a 1-based line and column (the column in
// bytes). It positions findings that originate outside the rule walk, such as schema-validation
// diagnostics, against the same source the rules report against.
func (d *Document) Position(offset int) (line, col int) {
	return d.pos.lineCol(offset)
}

// OffsetAt returns the byte offset of the tree node at the given instance location, expressed as
// unescaped JSON Pointer segments (object member names, or array indices in decimal). It resolves
// the location the same way [walk] builds one, following object members by name and array elements
// by index. When a segment cannot be resolved — the location points into a value that is not an
// object or array, or names a missing member — it returns the offset of the deepest ancestor it did
// resolve, so a diagnostic is never left unpositioned. The empty location yields the document root's
// offset. It reads the tree as given; any mutation (see [Document.Tree]) must happen after.
func (d *Document) OffsetAt(loc []string) int {
	v := d.tree
	for _, seg := range loc {
		next := childValue(v, seg)
		if next == nil {
			break
		}
		v = next
	}
	return v.StartOffset
}

// childValue returns the member named seg of an object, or the element at index seg of an array, or
// nil when v is neither or the segment does not resolve.
func childValue(v *hujson.Value, seg string) *hujson.Value {
	switch t := v.Value.(type) {
	case *hujson.Object:
		for i := range t.Members {
			name, ok := t.Members[i].Name.Value.(hujson.Literal)
			if ok && name.String() == seg {
				return &t.Members[i].Value
			}
		}
	case *hujson.Array:
		if idx, err := strconv.Atoi(seg); err == nil && idx >= 0 && idx < len(t.Elements) {
			return &t.Elements[idx]
		}
	}
	return nil
}
