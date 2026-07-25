package rules

import (
	"fmt"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/tailscale/hujson"
)

// UnusedTemplateOption reports a Template option declared in devcontainer-template.json's "options"
// that no file in the Template references as ${templateOption:name}. Such an option can never affect
// the applied template, so it is dead configuration.
var UnusedTemplateOption = &linter.Rule{
	ID:          "unused-template-option",
	Description: "disallow a Template option that no file in the Template references",
	Category:    linter.CategoryStyle,
	FileTypes:   []linter.FileType{linter.Template},
	Paths:       []string{"/options"},
	Check:       checkUnusedTemplateOption,
}

func checkUnusedTemplateOption(ctx *linter.Context, node *linter.Node) []linter.Finding {
	if ctx.Dir.FS == nil {
		return nil
	}
	obj, ok := node.Value.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	refs, err := templateOptionRefs(ctx.Dir.FS)
	if err != nil {
		return nil
	}

	var findings []linter.Finding
	for i := range obj.Members {
		name, ok := obj.Members[i].Name.Value.(hujson.Literal)
		if !ok || name.Kind() != '"' {
			continue
		}
		if _, used := refs[name.String()]; used {
			continue
		}
		findings = append(findings, linter.Finding{
			Message: fmt.Sprintf("option %q is declared but no template file references ${templateOption:%s}", name.String(), name.String()),
			Offset:  obj.Members[i].Name.StartOffset,
		})
	}
	return findings
}
