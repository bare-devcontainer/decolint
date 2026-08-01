package linter

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tailscale/hujson"
)

// noImageLatestRule is a test double for the rules package's no-image-latest rule: it flags an
// "image" property with no tag or the "latest" tag. Reusing its ID keeps testdata's
// decolint-ignore-next-line comments meaningful without this package depending on rules.
var noImageLatestRule = &Rule{
	ID:        "no-image-latest",
	FileTypes: []FileType{Devcontainer},
	Paths:     []string{"/image"},
	Check: func(_ *Context, node *Node) []Finding {
		lit, ok := node.Value.Value.(hujson.Literal)
		if !ok || lit.Kind() != '"' {
			return nil
		}
		image := lit.String()
		tag, hasTag := "", false
		if i := strings.LastIndex(image, ":"); i >= 0 {
			tag, hasTag = image[i+1:], true
		}
		switch {
		case !hasTag:
			return []Finding{{Message: fmt.Sprintf("image %q has no explicit tag", image), Offset: node.Value.StartOffset}}
		case tag == "latest":
			return []Finding{{Message: fmt.Sprintf("image %q uses the \"latest\" tag", image), Offset: node.Value.StartOffset}}
		}
		return nil
	},
}

// lintSource parses src and applies l's registered rules to it as a file at the given path and of
// the given type, failing the test on any parse error.
func lintSource(t *testing.T, l *Linter, path string, fileType FileType, src string) []Issue {
	t.Helper()
	doc, err := ParseDocument([]byte(src))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	return l.LintDocument(path, fileType, doc, Dir{})
}

func TestLintDocument_Position(t *testing.T) {
	t.Parallel()

	src := `{
  "name": "test",
  "image": "ubuntu:latest"
}`
	l := New()
	l.RegisterRule(noImageLatestRule, SeverityWarn)
	got := lintSource(t, l, "devcontainer.json", Devcontainer, src)
	if len(got) != 1 {
		t.Fatalf("got %d issues, want 1", len(got))
	}
	if got[0].Line != 3 || got[0].Col != 12 {
		t.Errorf("position = %d:%d, want 3:12", got[0].Line, got[0].Col)
	}
}

func TestParseDocument_Error(t *testing.T) {
	t.Parallel()

	if _, err := ParseDocument([]byte(`{`)); err == nil {
		t.Error("ParseDocument on malformed input: got nil error, want parse error")
	}
}

// flagRule is a stub Rule that reports the value at "/flag" when it is true, used to observe
// mutations made to the syntax tree before rules run.
var flagRule = &Rule{
	ID:          "flag-rule",
	Description: "reports a true /flag value",
	FileTypes:   []FileType{Devcontainer},
	Paths:       []string{"/flag"},
	Check: func(_ *Context, node *Node) []Finding {
		if lit, ok := node.Value.Value.(hujson.Literal); ok && lit.Bool() {
			return []Finding{{Message: "flag is true", Offset: node.Value.StartOffset}}
		}
		return nil
	},
}

func TestLintDocument_TreeMutation(t *testing.T) {
	t.Parallel()

	// A synthetic "flag": true member is added whose offsets point at the "name" member of the
	// original source, so the finding must resolve to that position.
	src := "{\n  \"name\": \"test\"\n}"
	doc, err := ParseDocument([]byte(src))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	obj := doc.Tree().Value.(*hujson.Object)
	anchor := obj.Members[0].Name.StartOffset
	obj.Members = append(obj.Members, hujson.ObjectMember{
		Name:  hujson.Value{Value: hujson.String("flag"), StartOffset: anchor, EndOffset: anchor},
		Value: hujson.Value{Value: hujson.Bool(true), StartOffset: anchor, EndOffset: anchor},
	})

	l := New()
	l.RegisterRule(flagRule, SeverityWarn)
	issues := l.LintDocument("devcontainer.json", Devcontainer, doc, Dir{})
	if len(issues) != 1 {
		t.Fatalf("got %d issues %v, want 1", len(issues), issues)
	}
	if issues[0].Line != 2 || issues[0].Col != 3 {
		t.Errorf("position = %d:%d, want 2:3", issues[0].Line, issues[0].Col)
	}
}

func TestLintDocument_ContextDir(t *testing.T) {
	t.Parallel()

	// A stub rule records the Dir it is handed so we can assert LintDocument passes it through
	// unchanged, including the zero case for an in-memory document.
	var got Dir
	dirRule := &Rule{
		ID:        "dir-rule",
		FileTypes: []FileType{Devcontainer},
		Paths:     []string{""},
		Check: func(ctx *Context, _ *Node) []Finding {
			got = ctx.Dir
			return nil
		},
	}

	want := Dir{FS: fstest.MapFS{"install.sh": {Data: []byte("#!/bin/sh\n")}}, Name: "my-feature"}
	for _, tt := range []struct {
		name string
		dir  Dir
	}{
		{"with directory", want},
		{"zero directory", Dir{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseDocument([]byte(`{"name": "test"}`))
			if err != nil {
				t.Fatalf("ParseDocument: %v", err)
			}
			l := New()
			l.RegisterRule(dirRule, SeverityWarn)
			got = Dir{}
			l.LintDocument("devcontainer.json", Devcontainer, doc, tt.dir)
			if got.Name != tt.dir.Name {
				t.Errorf("ctx.Dir.Name = %q, want %q", got.Name, tt.dir.Name)
			}
			if (got.FS == nil) != (tt.dir.FS == nil) {
				t.Errorf("ctx.Dir.FS = %v, want %v", got.FS, tt.dir.FS)
			}
		})
	}
}

// messagesOf returns the Message of every issue, in order, so tests can assert the sort produced by
// LintDocument without depending on exact line/column numbers.
func messagesOf(issues []Issue) []string {
	msgs := make([]string, len(issues))
	for i, iss := range issues {
		msgs[i] = iss.Message
	}
	return msgs
}

func TestLintDocument_SortsByPosition(t *testing.T) {
	t.Parallel()

	// "aa" and "bb" sit on the same line at different columns; "cc" is on a later line.
	src := "{\n  \"x\": \"aa bb\",\n  \"y\": \"cc\"\n}"
	offAA := strings.Index(src, "aa")
	offBB := strings.Index(src, "bb")
	offCC := strings.Index(src, "cc")

	// Emit the findings in reverse position order so the assertion fails unless LintDocument sorts.
	rule := &Rule{
		ID:        "pos-rule",
		FileTypes: []FileType{Devcontainer},
		Paths:     []string{""},
		Check: func(*Context, *Node) []Finding {
			return []Finding{
				{Message: "cc", Offset: offCC},
				{Message: "bb", Offset: offBB},
				{Message: "aa", Offset: offAA},
			}
		},
	}
	l := New()
	l.RegisterRule(rule, SeverityWarn)

	got := messagesOf(lintSource(t, l, "devcontainer.json", Devcontainer, src))
	want := []string{"aa", "bb", "cc"}
	if !slices.Equal(got, want) {
		t.Errorf("issue order = %v, want %v (sorted by line then column)", got, want)
	}
}

func TestLintDocument_SortsByRuleIDAtSamePosition(t *testing.T) {
	t.Parallel()

	// Both rules flag the document root, so their findings share a line and column and must be
	// ordered by RuleID. "zzz-rule" is registered first to prove registration order does not decide.
	src := `{}`
	newRootRule := func(id string) *Rule {
		return &Rule{
			ID:        id,
			FileTypes: []FileType{Devcontainer},
			Paths:     []string{""},
			Check: func(_ *Context, node *Node) []Finding {
				return []Finding{{Message: id, Offset: node.Value.StartOffset}}
			},
		}
	}
	l := New()
	l.RegisterRule(newRootRule("zzz-rule"), SeverityWarn)
	l.RegisterRule(newRootRule("aaa-rule"), SeverityWarn)

	got := messagesOf(lintSource(t, l, "devcontainer.json", Devcontainer, src))
	want := []string{"aaa-rule", "zzz-rule"}
	if !slices.Equal(got, want) {
		t.Errorf("issue order = %v, want %v (sorted by RuleID)", got, want)
	}
}

func TestLintDocument_SortsByMessageAtSamePosition(t *testing.T) {
	t.Parallel()

	// One rule reporting several missing properties at the document root emits findings that share a
	// position and a rule ID, so only the message can order them. They are emitted in reverse so the
	// assertion fails unless the message decides.
	src := `{}`
	want := []string{`"id" is required`, `"name" is required`, `"version" is required`}
	rule := &Rule{
		ID:        "missing-required-props",
		FileTypes: []FileType{Devcontainer},
		Paths:     []string{""},
		Check: func(_ *Context, node *Node) []Finding {
			return []Finding{
				{Message: `"version" is required`, Offset: node.Value.StartOffset},
				{Message: `"name" is required`, Offset: node.Value.StartOffset},
				{Message: `"id" is required`, Offset: node.Value.StartOffset},
			}
		},
	}
	l := New()
	l.RegisterRule(rule, SeverityWarn)

	got := messagesOf(lintSource(t, l, "devcontainer.json", Devcontainer, src))
	if !slices.Equal(got, want) {
		t.Errorf("issue order = %v, want %v (sorted by Message)", got, want)
	}
}

func TestLintDocument_Deduplicates(t *testing.T) {
	t.Parallel()

	// "aa" and "bb" share a line at different columns; "cc" is on a later line, so cases can differ
	// in column alone or in line alone.
	src := "{\n  \"x\": \"aa bb\",\n  \"y\": \"cc\"\n}"
	offAA := strings.Index(src, "aa")
	offBB := strings.Index(src, "bb")
	offCC := strings.Index(src, "cc")

	type finding struct {
		ruleID  string
		message string
		offset  int
	}
	for _, tt := range []struct {
		name     string
		findings []finding
		want     int
	}{
		{
			name:     "identical findings collapse",
			findings: []finding{{"dup-rule", "same", offAA}, {"dup-rule", "same", offAA}},
			want:     1,
		},
		{
			name:     "differing column kept",
			findings: []finding{{"dup-rule", "same", offAA}, {"dup-rule", "same", offBB}},
			want:     2,
		},
		{
			name:     "differing line kept",
			findings: []finding{{"dup-rule", "same", offAA}, {"dup-rule", "same", offCC}},
			want:     2,
		},
		{
			name:     "differing message kept",
			findings: []finding{{"dup-rule", "one", offAA}, {"dup-rule", "two", offAA}},
			want:     2,
		},
		{
			name:     "differing rule ID kept",
			findings: []finding{{"one-rule", "same", offAA}, {"two-rule", "same", offAA}},
			want:     2,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Group the case's findings by rule ID and register one rule per group, so that a case
			// differing only in rule ID reports the same finding from two rules.
			byRule := map[string][]Finding{}
			var ids []string
			for _, f := range tt.findings {
				if _, ok := byRule[f.ruleID]; !ok {
					ids = append(ids, f.ruleID)
				}
				byRule[f.ruleID] = append(byRule[f.ruleID], Finding{Message: f.message, Offset: f.offset})
			}
			l := New()
			for _, id := range ids {
				l.RegisterRule(&Rule{
					ID:        id,
					FileTypes: []FileType{Devcontainer},
					Paths:     []string{""},
					Check: func(*Context, *Node) []Finding {
						return byRule[id]
					},
				}, SeverityWarn)
			}

			if got := lintSource(t, l, "devcontainer.json", Devcontainer, src); len(got) != tt.want {
				t.Errorf("got %d issues %v, want %d", len(got), got, tt.want)
			}
		})
	}
}

func TestIssueString(t *testing.T) {
	t.Parallel()

	issue := Issue{
		Path:     ".devcontainer/devcontainer.json",
		Line:     3,
		Col:      12,
		RuleID:   "no-image-latest",
		Message:  "image has no explicit tag",
		Severity: SeverityWarn,
	}
	want := ".devcontainer/devcontainer.json:3:12: warn: image has no explicit tag (no-image-latest)"
	if got := issue.String(); got != want {
		t.Errorf("Issue.String() = %q, want %q", got, want)
	}
}

func TestHasRules(t *testing.T) {
	t.Parallel()

	l := New()
	if l.HasRules(Devcontainer) {
		t.Error("HasRules on an empty linter = true, want false")
	}
	l.RegisterRule(noImageLatestRule, SeverityWarn)
	if !l.HasRules(Devcontainer) {
		t.Error("HasRules after registering a devcontainer rule = false, want true")
	}
	// The rule applies only to devcontainer files, so other file types remain ruleless.
	if l.HasRules(Feature) {
		t.Error("HasRules(Feature) = true, want false")
	}
}

// TestRegisterRule_OffIsNotApplied covers a rule registered at SeverityOff: it contributes no
// patterns, so HasRules stays false and the rule never runs.
func TestRegisterRule_OffIsNotApplied(t *testing.T) {
	t.Parallel()

	l := New()
	l.RegisterRule(noImageLatestRule, SeverityOff)
	if l.HasRules(Devcontainer) {
		t.Error("HasRules after registering an off rule = true, want false")
	}
	if got := lintSource(t, l, "devcontainer.json", Devcontainer, `{"image": "ubuntu:latest"}`); len(got) != 0 {
		t.Errorf("got %d issues from an off rule, want 0", len(got))
	}
}

// TestLintDocument_IgnoreSuppresses covers the ignore-directive path of LintDocument: a finding on a
// line covered by a decolint-ignore directive is dropped.
func TestLintDocument_IgnoreSuppresses(t *testing.T) {
	t.Parallel()

	src := `{
  // decolint-ignore-next-line no-image-latest
  "image": "ubuntu:latest"
}`
	l := New()
	l.RegisterRule(noImageLatestRule, SeverityWarn)
	if got := lintSource(t, l, "devcontainer.json", Devcontainer, src); len(got) != 0 {
		t.Errorf("got %d issues, want 0 (finding suppressed by ignore directive): %v", len(got), got)
	}
}

// panicRule is a stub Rule whose Check always panics, used to verify that the engine survives a
// defective rule instead of letting it abort the whole run.
var panicRule = &Rule{
	ID:          "panic-rule",
	Description: "always panics",
	FileTypes:   []FileType{Devcontainer},
	Paths:       []string{""},
	Check: func(*Context, *Node) []Finding {
		panic("boom")
	},
}

func TestLintDocument_RulePanicIsRecovered(t *testing.T) {
	t.Parallel()

	l := New()
	l.RegisterRule(panicRule, SeverityError)
	issues := lintSource(t, l, "devcontainer.json", Devcontainer, `{}`)
	if len(issues) != 1 {
		t.Fatalf("got %d issues %v, want 1", len(issues), issues)
	}
	if issues[0].RuleID != "panic-rule" {
		t.Errorf("RuleID = %q, want %q", issues[0].RuleID, "panic-rule")
	}
	if issues[0].Severity != SeverityError {
		t.Errorf("Severity = %v, want %v", issues[0].Severity, SeverityError)
	}
}

// runArgsSpy returns a stub Rule subscribing to path that reports every value it is handed, naming
// how it was reached. It declares every file type deliberately: a rule that declares only some leaves
// LintDocument with no patterns for the rest, which it short-circuits before traversing anything, so
// a test written on such a rule would pass whatever the traversal does with the file types it skips.
func runArgsSpy(id, path string) *Rule {
	return &Rule{
		ID:          id,
		Description: "reports how each value under runArgs was reached",
		FileTypes:   []FileType{Devcontainer, Feature, Template},
		Paths:       []string{path},
		Check: func(_ *Context, node *Node) []Finding {
			if node.Arg == nil {
				return []Finding{{Message: "element " + node.Pointer, Offset: node.Value.StartOffset}}
			}
			return []Finding{{Message: "flag --" + node.Arg.Flag, Offset: node.Value.StartOffset}}
		},
	}
}

// TestLintDocument_RunArgsFileTypes checks that "runArgs" is read as a "docker run" argv only in a
// devcontainer.json. It is not a property of a Feature or a Template, so there the array is an
// ordinary one, walked by index — and so is a "runArgs" that is not an array at all, which a
// devcontainer.json has nothing to read in.
func TestLintDocument_RunArgsFileTypes(t *testing.T) {
	t.Parallel()

	const (
		argv   = `{"runArgs": ["--cap-add=ALL"]}`
		object = `{"runArgs": {"--cap-add": "ALL"}}`
	)
	tests := []struct {
		name     string
		fileType FileType
		src      string
		want     []string
	}{
		{"devcontainer", Devcontainer, argv, []string{"flag --cap-add"}},
		{"feature", Feature, argv, []string{"element /runArgs/0"}},
		{"template", Template, argv, []string{"element /runArgs/0"}},
		{"devcontainer object", Devcontainer, object, nil},
		{"feature object", Feature, object, []string{"element /runArgs/--cap-add"}},
		{"template object", Template, object, []string{"element /runArgs/--cap-add"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := New()
			l.RegisterRule(runArgsSpy("run-args-spy", "/runArgs/*"), SeverityWarn)
			var got []string
			for _, issue := range lintSource(t, l, "config.json", tt.fileType, tt.src) {
				got = append(got, issue.Message)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("messages = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLintDocument_RunArgsFlagPath checks what a "/runArgs/--flag" path matches. In a devcontainer.json
// it is the argv's occurrences of that flag and nothing else; in a Feature or a Template, where
// "runArgs" is no property of the file at all, it is an ordinary member that happens to be spelled
// like the flag. A rule that reports both a property and a flag has to tell the two apart itself,
// since [Node.Arg] is nil for such a member just as it is for the property.
func TestLintDocument_RunArgsFlagPath(t *testing.T) {
	t.Parallel()

	const (
		argv   = `{"runArgs": ["--cap-add=ALL"]}`
		object = `{"runArgs": {"--cap-add": "ALL"}}`
	)
	tests := []struct {
		name     string
		fileType FileType
		src      string
		want     []string
	}{
		{"devcontainer", Devcontainer, argv, []string{"flag --cap-add"}},
		{"devcontainer object", Devcontainer, object, nil},
		{"feature", Feature, argv, nil},
		{"feature object", Feature, object, []string{"element /runArgs/--cap-add"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := New()
			l.RegisterRule(runArgsSpy("run-args-flag-spy", "/runArgs/--cap-add"), SeverityWarn)
			var got []string
			for _, issue := range lintSource(t, l, "config.json", tt.fileType, tt.src) {
				got = append(got, issue.Message)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("messages = %v, want %v", got, tt.want)
			}
		})
	}
}
