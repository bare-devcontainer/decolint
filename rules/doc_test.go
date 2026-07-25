package rules_test

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
	"github.com/google/go-cmp/cmp"
)

// Every built-in rule is documented by a page under docsDir, published at [rules.DocsURL] and
// linked from every finding decolint reports. These tests are what keeps a rule from landing
// undocumented, a page from drifting from the rule it describes, and an example from claiming
// behavior the rule does not have.

// docsDir holds the rule reference, one Markdown page per rule ID, relative to this package.
const docsDir = "../docs/rules"

// docsFileNames maps a file type to the name a page's examples are linted under, which is the name
// the specification gives that kind of configuration file.
var docsFileNames = map[linter.FileType]string{
	linter.Devcontainer: "devcontainer.json",
	linter.Feature:      "devcontainer-feature.json",
	linter.Template:     "devcontainer-template.json",
}

// unverifiableExamples are the rules whose Bad and Good examples differ in something other than the
// contents of a configuration file — a file's permission bits, or whether it exists at all — and so
// cannot be checked by linting them. Listing them here rather than skipping silently keeps the set
// from growing unnoticed.
var unverifiableExamples = []string{
	"feature-install-script-not-executable",
	"missing-feature-install-script",
}

func TestBuiltin_Descriptions(t *testing.T) {
	t.Parallel()

	for _, reg := range rules.Builtin() {
		if strings.TrimSpace(reg.Rule.Description) == "" {
			t.Errorf("rule %s has no Description", reg.Rule.ID)
		}
	}
}

// TestBuiltin_DocsPages checks that the rule reference and the rule registry describe the same set
// of rules, and that each page's front matter matches the rule it documents. The front matter is
// what the site renders the page's heading and index entry from, so a stale value is published.
func TestBuiltin_DocsPages(t *testing.T) {
	t.Parallel()

	pages := readDocsPages(t)
	var documented []string
	for id := range pages {
		documented = append(documented, id)
	}
	slices.Sort(documented)

	var registered []string
	for _, reg := range rules.Builtin() {
		registered = append(registered, reg.Rule.ID)
	}
	slices.Sort(registered)

	if diff := cmp.Diff(registered, documented); diff != "" {
		t.Errorf("documented rules differ from registered rules (-registered +documented):\n%s", diff)
	}

	for _, reg := range rules.Builtin() {
		page, ok := pages[reg.Rule.ID]
		if !ok {
			continue // already reported by the diff above
		}
		want := map[string]string{
			"title":       reg.Rule.ID,
			"category":    reg.Rule.Category.String(),
			"platforms":   strings.Join(platformNames(reg.Rule.Platforms), ", "),
			"file_types":  strings.Join(fileTypeNames(reg.Rule.FileTypes), ", "),
			"description": reg.Rule.Description,
		}
		got := map[string]string{
			"title":       page.frontMatter["title"],
			"category":    page.frontMatter["category"],
			"platforms":   page.frontMatter["platforms"],
			"file_types":  page.frontMatter["file_types"],
			"description": page.frontMatter["description"],
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("rule %s front matter mismatch (-rule +page):\n%s", reg.Rule.ID, diff)
		}
	}
}

// TestBuiltin_DocsExamples lints each page's Bad and Good examples with only the rule the page
// documents enabled: the Bad example must report it and the Good example must not. It is what stops
// an example from teaching configuration the rule does not actually accept or reject.
func TestBuiltin_DocsExamples(t *testing.T) {
	t.Parallel()

	pages := readDocsPages(t)
	var skipped []string
	for _, reg := range rules.Builtin() {
		page, ok := pages[reg.Rule.ID]
		if !ok {
			continue // reported by TestBuiltin_DocsPages
		}
		if !page.verifiable {
			skipped = append(skipped, reg.Rule.ID)
			continue
		}

		t.Run(reg.Rule.ID, func(t *testing.T) {
			t.Parallel()

			if issues := lintDocsExample(t, reg.Rule, page, page.bad); len(issues) == 0 {
				t.Errorf("Bad example reports nothing; it must trip %s", reg.Rule.ID)
			}
			if issues := lintDocsExample(t, reg.Rule, page, page.good); len(issues) > 0 {
				t.Errorf("Good example reports %d issue(s), want none: %v", len(issues), issues)
			}
		})
	}

	slices.Sort(skipped)
	if diff := cmp.Diff(unverifiableExamples, skipped); diff != "" {
		t.Errorf("unverifiable examples differ from the documented set (-want +got):\n%s", diff)
	}
}

// TestBuiltin_DocsReferences checks the links each page cites as its justification: a reader has to
// be able to follow them, so each must be an absolute https URL, and citing one twice is a mistake.
func TestBuiltin_DocsReferences(t *testing.T) {
	t.Parallel()

	for id, page := range readDocsPages(t) {
		if len(page.references) == 0 {
			t.Errorf("rule %s documents no references", id)
		}
		for _, ref := range page.references {
			u, err := url.Parse(ref)
			if err != nil {
				t.Errorf("rule %s reference %q does not parse: %v", id, ref, err)
				continue
			}
			if u.Scheme != "https" || u.Host == "" {
				t.Errorf("rule %s reference %q is not an absolute https URL", id, ref)
			}
		}
		if i := duplicateIndex(page.references); i >= 0 {
			t.Errorf("rule %s cites reference %q twice", id, page.references[i])
		}
	}
}

func TestDocsURL(t *testing.T) {
	t.Parallel()

	got := rules.DocsURL("no-image-latest")
	want := "https://bare-devcontainer.github.io/decolint/rules/no-image-latest/"
	if got != want {
		t.Errorf("DocsURL = %q, want %q", got, want)
	}
}

// docsPage is one rule's documentation page.
type docsPage struct {
	frontMatter map[string]string
	// bad and good are the files making up each example, in the order the page shows them.
	bad, good []docsFile
	// verifiable reports whether the examples can be checked by linting them; see
	// unverifiableExamples.
	verifiable bool
	references []string
}

// docsFile is one file of an example: the name it is linted under and its contents.
type docsFile struct {
	name, source string
}

// readDocsPages parses every rule page in docsDir, keyed by the rule ID its file is named after.
func readDocsPages(t *testing.T) map[string]docsPage {
	t.Helper()

	entries, err := os.ReadDir(docsDir)
	if err != nil {
		t.Fatalf("read %s: %v", docsDir, err)
	}

	pages := make(map[string]docsPage, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".md" || name == "index.md" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(docsDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		pages[strings.TrimSuffix(name, ".md")] = parseDocsPage(t, name, string(src))
	}
	if len(pages) == 0 {
		t.Fatalf("no rule pages found in %s", docsDir)
	}
	return pages
}

// parseDocsPage parses a rule page: YAML front matter delimited by "---" lines, then "## Bad",
// "## Good" and "## References" sections. Within a section, a fenced jsonc block is one file of the
// example, named by the "### `name`" heading before it or, with no heading, by the rule's own file
// type.
func parseDocsPage(t *testing.T, name, src string) docsPage {
	t.Helper()

	body, ok := strings.CutPrefix(src, "---\n")
	if !ok {
		t.Fatalf("%s: no front matter", name)
	}
	fm, body, ok := strings.Cut(body, "\n---\n")
	if !ok {
		t.Fatalf("%s: front matter is not terminated", name)
	}

	page := docsPage{frontMatter: parseFrontMatter(t, name, fm)}
	page.verifiable = page.frontMatter["example_verify"] != "false"

	var section, fileName, fence string
	var block strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if fence != "" {
			if strings.TrimSpace(line) == fence {
				if fileName != "" {
					file := docsFile{name: fileName, source: block.String()}
					switch section {
					case "Bad":
						page.bad = append(page.bad, file)
					case "Good":
						page.good = append(page.good, file)
					}
				}
				fence, fileName = "", ""
				block.Reset()
				continue
			}
			block.WriteString(line + "\n")
			continue
		}
		switch {
		case strings.HasPrefix(line, "## "):
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		case strings.HasPrefix(line, "### "):
			fileName = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "### ")), "`")
		case strings.HasPrefix(line, "```jsonc"):
			fence = "```"
			if fileName == "" {
				fileName = defaultDocsFileName(t, name, page.frontMatter["file_types"])
			}
		case strings.HasPrefix(line, "```"):
			// A block in another language documents something that is not a configuration file, so
			// it is shown to the reader but never linted.
			fence = "```"
			fileName = ""
		case section == "References" && strings.HasPrefix(line, "- <"):
			page.references = append(page.references, strings.Trim(strings.TrimPrefix(line, "- "), "<>"))
		}
	}
	return page
}

// parseFrontMatter parses the subset of YAML the rule pages use: "key: value", a flow sequence
// "key: [a, b]" kept as its comma-separated contents, and a folded block scalar "key: >-" whose
// indented continuation lines are joined with single spaces.
func parseFrontMatter(t *testing.T, name, src string) map[string]string {
	t.Helper()

	out := map[string]string{}
	var folding string
	for _, line := range strings.Split(src, "\n") {
		if folding != "" {
			if strings.HasPrefix(line, "  ") {
				out[folding] = strings.TrimSpace(out[folding] + " " + strings.TrimSpace(line))
				continue
			}
			folding = ""
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch {
		case value == ">-":
			folding = key
			out[key] = ""
		case strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]"):
			out[key] = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
		default:
			out[key] = value
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s: front matter has no fields", name)
	}
	return out
}

// defaultDocsFileName returns the file name an unlabelled example block is linted under: the name
// of the first file type in the page's front matter, which is the kind of file the rule's own
// examples are written as.
func defaultDocsFileName(t *testing.T, name, fileTypes string) string {
	t.Helper()

	first, _, _ := strings.Cut(fileTypes, ",")
	fileName, ok := docsFileNames[linter.FileType(strings.TrimSpace(first))]
	if !ok {
		t.Fatalf("%s: front matter names no known file type, got %q", name, fileTypes)
	}
	return fileName
}

// lintDocsExample lints one example with rule as the only active rule and returns what it reports.
// Every file of the example is visible to rules that read sibling files; the one named after the
// rule's file type is the one linted.
func lintDocsExample(t *testing.T, rule *linter.Rule, page docsPage, files []docsFile) []linter.Issue {
	t.Helper()

	fileName := docsFileNames[rule.FileTypes[0]]
	fsys := fstest.MapFS{}
	var source string
	var found bool
	for _, f := range files {
		fsys[path.Clean(f.name)] = &fstest.MapFile{Data: []byte(f.source)}
		if f.name == fileName {
			source, found = f.source, true
		}
	}
	if !found {
		t.Fatalf("example has no %s block to lint, got %d file(s)", fileName, len(files))
	}

	doc, err := linter.ParseDocument([]byte(source))
	if err != nil {
		t.Fatalf("parse %s example: %v", fileName, err)
	}

	l := linter.New()
	l.RegisterRule(rule, linter.SeverityError)
	return l.LintDocument(fileName, rule.FileTypes[0], doc, linter.Dir{
		FS:   fsys,
		Name: page.frontMatter["example_dir"],
	})
}

func platformNames(platforms []linter.Platform) []string {
	names := make([]string, len(platforms))
	for i, p := range platforms {
		names[i] = p.String()
	}
	return names
}

func fileTypeNames(fileTypes []linter.FileType) []string {
	names := make([]string, len(fileTypes))
	for i, ft := range fileTypes {
		names[i] = string(ft)
	}
	return names
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
