package rules

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// UndefinedTemplateOption reports a ${templateOption:name} reference in a Template file whose name is
// not declared in devcontainer-template.json's "options". The reference implementation silently
// substitutes such a reference with the empty string, so the misconfiguration is otherwise invisible.
var UndefinedTemplateOption = &linter.Rule{
	ID:          "undefined-template-option",
	Description: "disallow a `${templateOption:...}` reference to an option not declared in devcontainer-template.json",
	Category:    linter.CategoryCorrectness,
	FileTypes:   []linter.FileType{linter.Template},
	Paths:       []string{""},
	Check:       checkUndefinedTemplateOption,
}

func checkUndefinedTemplateOption(ctx *linter.Context, node *linter.Node) []linter.Finding {
	if ctx.Dir == nil {
		return nil
	}
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	refs, err := templateOptionRefs(ctx.Dir)
	if err != nil {
		return nil
	}

	// Anchor findings at the "options" member if present, else at the document root: the reference
	// itself lives in a different file, whose position this document's offsets cannot address.
	declared := map[string]bool{}
	offset := node.Value.StartOffset
	if options := memberNamed(obj, "options"); options != nil {
		offset = options.Name.StartOffset
		if optObj, ok := options.Value.Value.(*hujson.Object); ok {
			for i := range optObj.Members {
				if name, ok := optObj.Members[i].Name.Value.(hujson.Literal); ok && name.Kind() == '"' {
					declared[name.String()] = true
				}
			}
		}
	}

	var findings []linter.Finding
	for _, name := range slices.Sorted(maps.Keys(refs)) {
		if declared[name] {
			continue
		}
		findings = append(findings, linter.Finding{
			Message: fmt.Sprintf("${templateOption:%s} is referenced in %s but %q is not declared in \"options\"", name, strings.Join(refs[name], ", "), name),
			Offset:  offset,
		})
	}
	return findings
}
