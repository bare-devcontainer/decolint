package linter

import (
	"fmt"
	"strings"
	"testing"

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
	return l.LintDocument(path, fileType, doc)
}

func TestIssuePosition(t *testing.T) {
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

func TestLintParseError(t *testing.T) {
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

func TestDocumentTreeMutation(t *testing.T) {
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
	issues := l.LintDocument("devcontainer.json", Devcontainer, doc)
	if len(issues) != 1 {
		t.Fatalf("got %d issues %v, want 1", len(issues), issues)
	}
	if issues[0].Line != 2 || issues[0].Col != 3 {
		t.Errorf("position = %d:%d, want 2:3", issues[0].Line, issues[0].Col)
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

func TestLintRulePanicIsRecovered(t *testing.T) {
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
