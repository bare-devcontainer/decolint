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
	LongDescription: `Applying a Template replaces each "${templateOption:name}" with the value the user chose for the option of
that name. A reference to an option that "options" does not declare is never prompted for, and the
reference implementation substitutes the empty string for it, so a typo silently produces an empty value
in the applied files instead of an error.`,
	References: []string{
		`https://containers.dev/implementors/templates/#the-options-property`,
		`https://github.com/devcontainers/cli`,
	},
	Category:  linter.CategoryCorrectness,
	FileTypes: []linter.FileType{linter.Template},
	Paths:     []string{""},
	Example: linter.Example{
		Bad: linter.Snippet{
			Files: []linter.ExampleFile{
				{
					Path: `devcontainer-template.json`,
					Content: `{
  "id": "dotnet",
  "version": "1.0.0",
  "name": "C# (.NET)",
  "options": {
    "imageVariant": {
      "type": "string",
      "proposals": ["8.0", "9.0"],
      "default": "9.0"
    }
  }
}
`,
				},
				{
					Path: `.devcontainer/devcontainer.json`,
					Content: `{
  "image": "mcr.microsoft.com/devcontainers/dotnet:${templateOption:variant}"
}
`,
				},
			},
		},
		Good: linter.Snippet{
			Files: []linter.ExampleFile{
				{
					Path: `devcontainer-template.json`,
					Content: `{
  "id": "dotnet",
  "version": "1.0.0",
  "name": "C# (.NET)",
  "options": {
    "imageVariant": {
      "type": "string",
      "proposals": ["8.0", "9.0"],
      "default": "9.0"
    }
  }
}
`,
				},
				{
					Path: `.devcontainer/devcontainer.json`,
					Content: `{
  "image": "mcr.microsoft.com/devcontainers/dotnet:${templateOption:imageVariant}"
}
`,
				},
			},
		},
	},
	Check: checkUndefinedTemplateOption,
}

func checkUndefinedTemplateOption(ctx *linter.Context, node *linter.Node) []linter.Finding {
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
