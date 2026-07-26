package rules_test

import (
	"net/url"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

// Every built-in rule documents itself on its [linter.Rule]: a short Description, a LongDescription
// explaining why it exists, References that justify it, and an Example whose Bad configuration the
// rule must report and whose Good configuration it must not. These tests are what keeps a new rule
// from landing undocumented or shipping an example that doesn't actually demonstrate the rule, and
// what the documentation site (see cmd/docgen) and "decolint -rules -format=json" are generated from.

func TestBuiltin_Descriptions(t *testing.T) {
	t.Parallel()

	for _, reg := range rules.Builtin() {
		if strings.TrimSpace(reg.Rule.Description) == "" {
			t.Errorf("rule %s has no Description", reg.Rule.ID)
		}
		if strings.TrimSpace(reg.Rule.LongDescription) == "" {
			t.Errorf("rule %s has no LongDescription", reg.Rule.ID)
		}
		if got := reg.Rule.LongDescription; got != strings.TrimSpace(got) {
			t.Errorf("rule %s LongDescription has leading or trailing whitespace", reg.Rule.ID)
		}
	}
}

func TestBuiltin_References(t *testing.T) {
	t.Parallel()

	for _, reg := range rules.Builtin() {
		if len(reg.Rule.References) == 0 {
			t.Errorf("rule %s has no References", reg.Rule.ID)
		}
		for _, ref := range reg.Rule.References {
			// A reference is rendered as a link wherever it is shown, so it has to be a URL a reader
			// can follow on its own, not a bare path or a prose citation.
			u, err := url.Parse(ref)
			if err != nil {
				t.Errorf("rule %s reference %q does not parse: %v", reg.Rule.ID, ref, err)
				continue
			}
			if u.Scheme != "https" || u.Host == "" {
				t.Errorf("rule %s reference %q is not an absolute https URL", reg.Rule.ID, ref)
			}
		}
		if i := duplicateIndex(reg.Rule.References); i >= 0 {
			t.Errorf("rule %s lists reference %q twice", reg.Rule.ID, reg.Rule.References[i])
		}
	}
}

// defaultFileName is the file name a rule's own configuration file is expected at, mirroring
// discovery's naming for each [linter.FileType].
var defaultFileName = map[linter.FileType]string{
	linter.Devcontainer: "devcontainer.json",
	linter.Feature:      "devcontainer-feature.json",
	linter.Template:     "devcontainer-template.json",
}

// TestBuiltin_Examples lints each rule's Example.Bad and Example.Good, exercising the same
// path-matching and traversal logic the linter uses in production: Bad must report the rule and Good
// must not, so an example cannot drift from the rule it documents.
func TestBuiltin_Examples(t *testing.T) {
	t.Parallel()

	for _, reg := range rules.Builtin() {
		t.Run(reg.Rule.ID, func(t *testing.T) {
			t.Parallel()

			// The exec-bit rule reports nothing on Windows by design (see
			// checkFeatureInstallScriptNotExecutable); its Bad example would report zero findings
			// there too, which would otherwise look like example drift rather than the documented
			// platform limitation.
			if reg.Rule.ID == "feature-install-script-not-executable" && runtime.GOOS == "windows" {
				t.Skip("this rule does not run on Windows")
			}

			if len(reg.Rule.Example.Bad.Files) == 0 || len(reg.Rule.Example.Good.Files) == 0 {
				t.Fatalf("rule %s has an empty Example", reg.Rule.ID)
			}

			if issues := lintExample(t, reg.Rule, reg.Rule.Example.Bad); len(issues) == 0 {
				t.Errorf("Example.Bad reports nothing; it must trip %s", reg.Rule.ID)
			}
			if issues := lintExample(t, reg.Rule, reg.Rule.Example.Good); len(issues) > 0 {
				t.Errorf("Example.Good reports %d issue(s), want none: %v", len(issues), issues)
			}
		})
	}
}

// lintExample lints snip with r as the only active rule, registered at [linter.SeverityError], and
// returns what it reports. Every file in snip is visible to a rule that reads sibling files; the one
// named after r's own file type is the one linted.
func lintExample(t *testing.T, r *linter.Rule, snip linter.Snippet) []linter.Issue {
	t.Helper()

	fileName, ok := defaultFileName[r.FileTypes[0]]
	if !ok {
		t.Fatalf("rule %s FileTypes[0] %q has no default file name", r.ID, r.FileTypes[0])
	}

	fsys := fstest.MapFS{}
	var source string
	var found bool
	for _, f := range snip.Files {
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}
		fsys[f.Path] = &fstest.MapFile{Data: []byte(f.Content), Mode: mode}
		if f.Path == fileName {
			source, found = f.Content, true
		}
	}
	if !found {
		t.Fatalf("example has no %s to lint, got %d file(s)", fileName, len(snip.Files))
	}

	doc, err := linter.ParseDocument([]byte(source))
	if err != nil {
		t.Fatalf("parse %s example: %v", fileName, err)
	}

	l := linter.New()
	l.RegisterRule(r, linter.SeverityError)
	return l.LintDocument(fileName, r.FileTypes[0], doc, linter.Dir{FS: fsys, Name: snip.DirName})
}

// duplicateIndex returns the index of the first element of refs that appears earlier in it, or -1
// if every element is unique.
func duplicateIndex(refs []string) int {
	for i, ref := range refs {
		if slices.Contains(refs[:i], ref) {
			return i
		}
	}
	return -1
}
