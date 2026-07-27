package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// readme.go turns README.md into the three top-level site pages that mirror it — the landing page,
// Getting started, and Reference — so their prose has exactly one source. Splitting it means
// rewriting the anchor links the README uses to jump within itself: an anchor now on a different
// page must point there instead ("reference.md#config-file"), which the goldmark link render hook
// enabled in hugo.toml resolves to that page's real address.

// readmePages are the pages splitReadme extracts from README.md, keyed by output file name (without
// extension). Each must appear in README.md exactly once, delimited by
// "<!-- decolint:page=NAME -->" and "<!-- decolint:end-page -->" (see splitReadme). Content outside
// any marked page — the title, badges, the "---" before "# Reference", "## Contributing" — is left
// where it is and never touched by docgen.
var readmePages = []string{"_index", "getting-started", "reference"}

const (
	readmePageStartPrefix = "<!-- decolint:page="
	readmePageStartSuffix = " -->"
	readmePageEnd         = "<!-- decolint:end-page -->"
	landingDescription    = "A linter for Dev Container configuration files."
	gettingStartedSummary = "Install decolint, choose what it reports, and wire it into CI."
	referenceSummary      = "Every flag, config file member, and output format decolint supports."
)

// writeReadmePages splits the README at readmePath into the landing, getting-started and reference
// pages and writes them under dir.
func writeReadmePages(readmePath, dir string) error {
	src, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", readmePath, err)
	}
	pages, err := splitReadme(string(src))
	if err != nil {
		return fmt.Errorf("%s: %w", readmePath, err)
	}
	for name, page := range pages {
		path := filepath.Join(dir, name+".md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// splitReadme splits README.md's content src into the landing, getting-started and reference
// pages, keyed by output file name (without extension), each a complete page: front matter, then
// body with cross-page anchors rewritten (see rewriteAnchors).
func splitReadme(src string) (map[string]string, error) {
	lines := strings.Split(src, "\n")

	bodies, err := readmePageBodies(lines)
	if err != nil {
		return nil, err
	}

	// The heading -> page map spans all three bodies, so a link anywhere among them can be resolved
	// to the page it actually lives on, or correctly left alone when it's already on the right one.
	// Built in a fixed order (not a map range) and erroring on a repeat: two pages sharing a heading
	// slug would otherwise resolve to whichever page Go's map iteration visited last, silently and
	// differently from run to run.
	slugPage := map[string]string{}
	for _, name := range readmePages {
		for _, h := range scanHeadings(bodies[name]) {
			if existing, ok := slugPage[h.Slug]; ok {
				return nil, fmt.Errorf("heading %q (slug %q) appears on both %q and %q", h.Text, h.Slug, existing, name)
			}
			slugPage[h.Slug] = name
		}
	}

	// Getting started and Reference are both long enough, with enough ## sections, to be worth a
	// table of contents; see docs/layouts/baseof.html and page-toc.html.
	frontMatter := map[string]string{
		"_index":          "title: decolint\ndescription: " + yamlSingleQuoted(landingDescription),
		"getting-started": "title: Getting started\ndescription: " + yamlSingleQuoted(gettingStartedSummary) + "\ntoc: true",
		"reference":       "title: Reference\ndescription: " + yamlSingleQuoted(referenceSummary) + "\ntoc: true",
	}

	pages := make(map[string]string, len(bodies))
	for _, name := range readmePages {
		rewritten, err := rewriteAnchors(bodies[name], name, slugPage)
		if err != nil {
			return nil, err
		}
		pages[name] = "---\n" + frontMatter[name] + "\n---\n\n" + rewritten + "\n"
	}
	return pages, nil
}

// readmePageBodies extracts the body of each page in readmePages from lines, delimited by
// "<!-- decolint:page=NAME -->" and "<!-- decolint:end-page -->" markers. A generator failing loudly
// when a marker is missing, duplicated, unterminated, or names an unknown page is the right failure
// mode; silently misplacing content is not.
func readmePageBodies(lines []string) (map[string]string, error) {
	bodies := map[string]string{}
	openName, openStart := "", -1
	for i, l := range lines {
		switch {
		case l == readmePageEnd:
			if openName == "" {
				return nil, fmt.Errorf("line %d: %q with no page marker open", i+1, readmePageEnd)
			}
			bodies[openName] = strings.Join(stripRulesTableMarkers(trimBlank(lines[openStart:i])), "\n")
			openName = ""
		case strings.HasPrefix(l, readmePageStartPrefix) && strings.HasSuffix(l, readmePageStartSuffix):
			name := strings.TrimSuffix(strings.TrimPrefix(l, readmePageStartPrefix), readmePageStartSuffix)
			if openName != "" {
				return nil, fmt.Errorf("line %d: page %q starts before page %q ends", i+1, name, openName)
			}
			if !slices.Contains(readmePages, name) {
				return nil, fmt.Errorf("line %d: unknown page %q (want one of %v)", i+1, name, readmePages)
			}
			if _, ok := bodies[name]; ok {
				return nil, fmt.Errorf("line %d: page %q marked more than once", i+1, name)
			}
			openName, openStart = name, i+1
		}
	}
	if openName != "" {
		return nil, fmt.Errorf("page %q has no %q", openName, readmePageEnd)
	}
	for _, name := range readmePages {
		if _, ok := bodies[name]; !ok {
			return nil, fmt.Errorf("README.md has no %s%s%s ... %s for page %q", readmePageStartPrefix, name, readmePageStartSuffix, readmePageEnd, name)
		}
	}
	return bodies, nil
}

// trimBlank drops leading and trailing blank lines from lines.
func trimBlank(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

// stripRulesTableMarkers drops the rulesTableStart/rulesTableEnd lines: bookkeeping for
// updateReadmeRulesTable, meaningless once the table is on its own page rather than sitting in
// README.md.
func stripRulesTableMarkers(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l == rulesTableStart || l == rulesTableEnd {
			continue
		}
		out = append(out, l)
	}
	return out
}

// anchorLink matches a Markdown link whose target is a same-document fragment, e.g. "(#config-file)".
// The character class matches slugify's, so a link to a slug containing an underscore (or a leading
// hyphen, which this class permits at any position) is still recognized as a same-document link.
var anchorLink = regexp.MustCompile(`\]\(#([a-z0-9_-]+)\)`)

// rewriteAnchors rewrites every "(#slug)" link in body that names a heading living on a different
// page than page, to "(<page>.md#slug)", fence-aware so a "#" shown inside an example is never
// touched. It errors if a link names a slug that is not a heading on any marked page: left alone,
// such a link would publish as a dead anchor once the page is split off from whatever the slug names
// (e.g. a marked page linking to "#contributing", which lives outside every page marker) — the same
// failure mode every other marker problem in this generator is caught by, not one exempt from it.
func rewriteAnchors(body, page string, slugPage map[string]string) (string, error) {
	lines := strings.Split(body, "\n")
	fenced := false
	var badSlug string
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		lines[i] = anchorLink.ReplaceAllStringFunc(line, func(m string) string {
			slug := anchorLink.FindStringSubmatch(m)[1]
			target, ok := slugPage[slug]
			if !ok {
				badSlug = slug
				return m
			}
			if target == page {
				return m
			}
			return "](" + target + ".md#" + slug + ")"
		})
		if badSlug != "" {
			return "", fmt.Errorf("page %q links to \"#%s\", which is not a heading on any page", page, badSlug)
		}
	}
	return strings.Join(lines, "\n"), nil
}
