package main

import (
	"os"
	"regexp"
	"testing"

	"github.com/bare-devcontainer/decolint/rules"
)

// categoryNameLine matches a "- name: xxx" entry in docs/data/categories.yaml. A narrow, hand-rolled
// match for this one field is enough here and avoids a YAML parsing dependency, the same tradeoff
// slug.go makes for Markdown headings.
var categoryNameLine = regexp.MustCompile(`(?m)^- name:\s*(\S+)\s*$`)

// TestCategoriesYAMLCoversEveryRuleCategory checks that docs/data/categories.yaml lists every
// category a built-in rule actually uses. rules/section.html (the rules index) and rule-nav.html
// (its sidebar) both range over hugo.Data.categories and filter rule pages by category, so a rule
// whose category is missing here is published — its own page still exists — but appears in neither
// listing, with nothing failing the build to say so.
func TestCategoriesYAMLCoversEveryRuleCategory(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../docs/data/categories.yaml")
	if err != nil {
		t.Fatalf("read categories.yaml: %v", err)
	}
	declared := map[string]bool{}
	for _, m := range categoryNameLine.FindAllStringSubmatch(string(data), -1) {
		declared[m[1]] = true
	}
	if len(declared) == 0 {
		t.Fatal("found no \"- name: ...\" entries in categories.yaml; the regexp or the file format drifted")
	}

	for _, reg := range rules.Builtin() {
		category := reg.Rule.Category.String()
		if !declared[category] {
			t.Errorf("rule %s has category %q, which docs/data/categories.yaml does not list", reg.Rule.ID, category)
		}
	}
}
