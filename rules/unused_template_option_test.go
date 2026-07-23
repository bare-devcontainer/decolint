package rules_test

import (
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestUnusedTemplateOption(t *testing.T) {
	t.Parallel()

	const path = "my-template/devcontainer-template.json"
	const src = `{
  "options": {
    "used": { "type": "string" },
    "unused": { "type": "string" }
  }
}`
	usedIssue := linter.Issue{Path: path, Line: 3, Col: 5, RuleID: "unused-template-option", Message: `option "used" is declared but no template file references ${templateOption:used}`}
	unusedIssue := linter.Issue{Path: path, Line: 4, Col: 5, RuleID: "unused-template-option", Message: `option "unused" is declared but no template file references ${templateOption:unused}`}

	tests := []struct {
		name string
		dir  fstest.MapFS
		want []linter.Issue
	}{
		{
			name: "referenced option is not flagged",
			dir:  fstest.MapFS{".devcontainer/devcontainer.json": {Data: []byte(`{"image": "${templateOption:used}"}`)}},
			want: []linter.Issue{unusedIssue},
		},
		{
			name: "whitespace around the name counts as a use",
			dir:  fstest.MapFS{".devcontainer/devcontainer.json": {Data: []byte(`{"image": "${templateOption: used }"}`)}},
			want: []linter.Issue{unusedIssue},
		},
		{
			name: "references in excluded root files do not count",
			dir: fstest.MapFS{
				"README.md":                  {Data: []byte("${templateOption:used}")},
				"NOTES.md":                   {Data: []byte("${templateOption:used}")},
				"devcontainer-template.json": {Data: []byte("${templateOption:used}")},
			},
			want: []linter.Issue{usedIssue, unusedIssue},
		},
		{
			name: "a reference in a subdirectory README counts as a use",
			dir:  fstest.MapFS{"docs/README.md": {Data: []byte("${templateOption:used}")}},
			want: []linter.Issue{unusedIssue},
		},
		{
			name: "references under .git do not count",
			dir:  fstest.MapFS{".git/config": {Data: []byte("${templateOption:used}")}},
			want: []linter.Issue{usedIssue, unusedIssue},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesInDir(t, rules.UnusedTemplateOption, linter.SeverityError, path, linter.Template, src, tt.dir, tt.want)
		})
	}

	t.Run("no options member", func(t *testing.T) {
		t.Parallel()
		dir := fstest.MapFS{".devcontainer/devcontainer.json": {Data: []byte("${templateOption:used}")}}
		assertIssuesInDir(t, rules.UnusedTemplateOption, linter.SeverityError, path, linter.Template, `{"id": "my-template"}`, dir, nil)
	})

	t.Run("options is not an object", func(t *testing.T) {
		t.Parallel()
		dir := fstest.MapFS{".devcontainer/devcontainer.json": {Data: []byte("${templateOption:used}")}}
		assertIssuesInDir(t, rules.UnusedTemplateOption, linter.SeverityError, path, linter.Template, `{"options": "nope"}`, dir, nil)
	})

	t.Run("unreadable directory reports nothing", func(t *testing.T) {
		t.Parallel()
		assertIssuesInDir(t, rules.UnusedTemplateOption, linter.SeverityError, path, linter.Template, src, errFS{}, nil)
	})

	t.Run("nil directory", func(t *testing.T) {
		t.Parallel()
		assertIssuesAt(t, rules.UnusedTemplateOption, linter.SeverityError, path, linter.Template, src, nil)
	})
}
