package rules_test

import (
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestMissingFeatureInstallScript(t *testing.T) {
	t.Parallel()

	const path = "my-feature/devcontainer-feature.json"
	const src = `{"id": "my-feature", "version": "1.0.0", "name": "My Feature"}`
	want := []linter.Issue{
		{Path: path, Line: 1, Col: 1, RuleID: "missing-feature-install-script", Message: `Feature has no "install.sh" install script next to devcontainer-feature.json`},
	}

	tests := []struct {
		name string
		dir  fstest.MapFS
		want []linter.Issue
	}{
		{"install.sh present", fstest.MapFS{"install.sh": {Data: []byte("#!/bin/sh\n")}}, nil},
		{"install.sh missing", fstest.MapFS{"devcontainer-feature.json": {Data: []byte(src)}}, want},
		{"install.sh is a directory", fstest.MapFS{"install.sh/keep": {Data: []byte("x")}}, want},
		{"install.sh only in a subdirectory", fstest.MapFS{"src/install.sh": {Data: []byte("#!/bin/sh\n")}}, want},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesInDir(t, rules.MissingFeatureInstallScript, linter.SeverityError, path, linter.Feature, src, linter.Dir{FS: tt.dir}, tt.want)
		})
	}

	t.Run("nil directory", func(t *testing.T) {
		t.Parallel()
		assertIssuesAt(t, rules.MissingFeatureInstallScript, linter.SeverityError, path, linter.Feature, src, nil)
	})

	t.Run("unreadable directory reports nothing", func(t *testing.T) {
		t.Parallel()
		assertIssuesInDir(t, rules.MissingFeatureInstallScript, linter.SeverityError, path, linter.Feature, src, linter.Dir{FS: errFS{}}, nil)
	})
}
