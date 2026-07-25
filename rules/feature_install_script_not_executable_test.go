package rules_test

import (
	"runtime"
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestFeatureInstallScriptNotExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bits are not represented in Windows working trees")
	}
	t.Parallel()

	const path = "my-feature/devcontainer-feature.json"
	const src = `{"id": "my-feature", "version": "1.0.0", "name": "My Feature"}`
	script := []byte("#!/bin/sh\n")
	want := []linter.Issue{
		{Path: path, Line: 1, Col: 1, RuleID: "feature-install-script-not-executable", Message: `"install.sh" is not executable (mode 0644); run "chmod +x install.sh"`},
	}

	tests := []struct {
		name string
		dir  fstest.MapFS
		want []linter.Issue
	}{
		{"not executable", fstest.MapFS{"install.sh": {Data: script, Mode: 0o644}}, want},
		{"executable by all", fstest.MapFS{"install.sh": {Data: script, Mode: 0o755}}, nil},
		{"executable by owner only", fstest.MapFS{"install.sh": {Data: script, Mode: 0o744}}, nil},
		{"install.sh absent", fstest.MapFS{"devcontainer-feature.json": {Data: []byte(src)}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssuesInDir(t, rules.FeatureInstallScriptNotExecutable, linter.SeverityError, path, linter.Feature, src, linter.Dir{FS: tt.dir}, tt.want)
		})
	}

	t.Run("nil directory", func(t *testing.T) {
		t.Parallel()
		assertIssuesAt(t, rules.FeatureInstallScriptNotExecutable, linter.SeverityError, path, linter.Feature, src, nil)
	})
}
