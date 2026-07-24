package rules_test

import (
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestUndefinedTemplateOption(t *testing.T) {
	t.Parallel()

	const path = "my-template/devcontainer-template.json"
	const withOptions = `{
  "options": {
    "declared": { "type": "string" }
  }
}`
	const noOptions = `{"id": "my-template"}`

	tests := []struct {
		name string
		src  string
		dir  fstest.MapFS
		want []linter.Issue
	}{
		{
			name: "undeclared reference is flagged at the options member",
			src:  withOptions,
			dir:  fstest.MapFS{".devcontainer/devcontainer.json": {Data: []byte(`{"image": "${templateOption:undeclared}"}`)}},
			want: []linter.Issue{
				{Path: path, Line: 2, Col: 3, RuleID: "undefined-template-option", Message: `${templateOption:undeclared} is referenced in .devcontainer/devcontainer.json but "undeclared" is not declared in "options"`},
			},
		},
		{
			name: "undeclared reference with no options member is flagged at the document root",
			src:  noOptions,
			dir:  fstest.MapFS{".devcontainer/devcontainer.json": {Data: []byte("${templateOption:foo}")}},
			want: []linter.Issue{
				{Path: path, Line: 1, Col: 1, RuleID: "undefined-template-option", Message: `${templateOption:foo} is referenced in .devcontainer/devcontainer.json but "foo" is not declared in "options"`},
			},
		},
		{
			name: "one finding per name lists all referencing files sorted",
			src:  withOptions,
			dir: fstest.MapFS{
				".devcontainer/devcontainer.json": {Data: []byte("${templateOption:missing}")},
				"Dockerfile":                      {Data: []byte("${templateOption:missing}")},
			},
			want: []linter.Issue{
				{Path: path, Line: 2, Col: 3, RuleID: "undefined-template-option", Message: `${templateOption:missing} is referenced in .devcontainer/devcontainer.json, Dockerfile but "missing" is not declared in "options"`},
			},
		},
		{
			name: "references in excluded root files are ignored",
			src:  withOptions,
			dir:  fstest.MapFS{"README.md": {Data: []byte("${templateOption:foo}")}},
			want: nil,
		},
		{
			name: "declared references are not flagged",
			src:  withOptions,
			dir:  fstest.MapFS{".devcontainer/devcontainer.json": {Data: []byte("${templateOption:declared}")}},
			want: nil,
		},
		{
			name: "options that is not an object declares nothing",
			src:  `{"options": "nope"}`,
			dir:  fstest.MapFS{".devcontainer/devcontainer.json": {Data: []byte("${templateOption:foo}")}},
			want: []linter.Issue{
				{Path: path, Line: 1, Col: 2, RuleID: "undefined-template-option", Message: `${templateOption:foo} is referenced in .devcontainer/devcontainer.json but "foo" is not declared in "options"`},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesInDir(t, rules.UndefinedTemplateOption, linter.SeverityError, path, linter.Template, tt.src, tt.dir, tt.want)
		})
	}

	t.Run("unreadable directory reports nothing", func(t *testing.T) {
		t.Parallel()
		assertIssuesInDir(t, rules.UndefinedTemplateOption, linter.SeverityError, path, linter.Template, withOptions, errFS{}, nil)
	})

	t.Run("nil directory", func(t *testing.T) {
		t.Parallel()
		assertIssuesAt(t, rules.UndefinedTemplateOption, linter.SeverityError, path, linter.Template, withOptions, nil)
	})
}
