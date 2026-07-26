package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// readme.go turns README.md into the three top-level site pages that mirror it — the landing page,
// Getting started, and Reference — so their prose has exactly one source. Splitting it means
// rewriting the anchor links the README uses to jump within itself: an anchor now on a different
// page must point there instead ("reference.md#config-file"), which the goldmark link render hook
// enabled in hugo.toml resolves to that page's real address.

// README section boundaries, matched as exact heading lines. A generator failing loudly when README
// is restructured is the right failure mode; silently misplacing content is not.
const (
	readmeWhyHeading      = "## Why decolint"
	readmeTryHeading      = "## Try it"
	readmeLintingHeading  = "## Linting a Feature or Template"
	readmeReferenceH1     = "# Reference"
	readmeContribHeading  = "## Contributing"
	readmeHorizontalRule  = "---"
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

	whyIdx := findHeadingLine(lines, readmeWhyHeading)
	tryIdx := findHeadingLine(lines, readmeTryHeading)
	lintingIdx := findHeadingLine(lines, readmeLintingHeading)
	refIdx := findHeadingLine(lines, readmeReferenceH1)
	contribIdx := findHeadingLine(lines, readmeContribHeading)
	if whyIdx < 0 || tryIdx < 0 || lintingIdx < 0 || refIdx < 0 || contribIdx < 0 {
		return nil, fmt.Errorf("could not find all expected section headings (why=%d try=%d linting=%d reference=%d contributing=%d)",
			whyIdx, tryIdx, lintingIdx, refIdx, contribIdx)
	}
	if whyIdx >= tryIdx || tryIdx >= lintingIdx || lintingIdx >= refIdx || refIdx >= contribIdx {
		return nil, fmt.Errorf("section headings are not in the expected order (why=%d try=%d linting=%d reference=%d contributing=%d)",
			whyIdx, tryIdx, lintingIdx, refIdx, contribIdx)
	}

	// Landing runs from the first content line after the title and badges through the end of "Why
	// decolint"; getting-started picks up at "Try it" and runs to just before the "---" separator
	// that precedes "# Reference"; reference runs from just after "# Reference" to just before
	// "Contributing", which belongs to the project rather than to decolint's own documentation.
	contentStart := skipTitleAndBadges(lines)

	bodies := map[string]string{
		"_index":          strings.Join(trimBlank(lines[contentStart:tryIdx]), "\n"),
		"getting-started": strings.Join(trimHorizontalRule(trimBlank(lines[tryIdx:refIdx])), "\n"),
		"reference":       strings.Join(stripRulesTableMarkers(trimBlank(lines[refIdx+1:contribIdx])), "\n"),
	}

	// The heading -> page map spans all three bodies, so a link anywhere among them can be resolved
	// to the page it actually lives on, or correctly left alone when it's already on the right one.
	slugPage := map[string]string{}
	for name, body := range bodies {
		for _, h := range scanHeadings(body) {
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
	for name, body := range bodies {
		rewritten := rewriteAnchors(body, name, slugPage)
		pages[name] = "---\n" + frontMatter[name] + "\n---\n\n" + rewritten + "\n"
	}
	return pages, nil
}

// findHeadingLine returns the index of the line in lines that is exactly the ATX heading want
// (e.g. "## Why decolint"), or -1 if there is none.
func findHeadingLine(lines []string, want string) int {
	for i, l := range lines {
		if l == want {
			return i
		}
	}
	return -1
}

// skipTitleAndBadges returns the index of the first line of body prose: past the "# " title and any
// immediately following badge lines ("[![...").
func skipTitleAndBadges(lines []string) int {
	i := 0
	for i < len(lines) && !strings.HasPrefix(lines[i], "# ") {
		i++
	}
	i++ // past the title line itself
	for i < len(lines) && (strings.TrimSpace(lines[i]) == "" || strings.HasPrefix(strings.TrimSpace(lines[i]), "[![")) {
		i++
	}
	return i
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

// trimHorizontalRule drops a trailing "---" line (and the blank lines around it, already stripped
// by trimBlank) — the separator README.md draws between the guide and "# Reference", which has
// nothing to do with the guide's own last section.
func trimHorizontalRule(lines []string) []string {
	if len(lines) > 0 && lines[len(lines)-1] == readmeHorizontalRule {
		return trimBlank(lines[:len(lines)-1])
	}
	return lines
}

// anchorLink matches a Markdown link whose target is a same-document fragment, e.g. "(#config-file)".
var anchorLink = regexp.MustCompile(`\]\(#([a-z0-9-]+)\)`)

// rewriteAnchors rewrites every "(#slug)" link in body that names a heading living on a different
// page than page, to "(<page>.md#slug)", fence-aware so a "#" shown inside an example is never
// touched. A slug not found in slugPage (nothing in README.md's current content triggers this; see
// writeReadmePages) is left alone rather than guessed at.
func rewriteAnchors(body, page string, slugPage map[string]string) string {
	lines := strings.Split(body, "\n")
	fenced := false
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
			if !ok || target == page {
				return m
			}
			return "](" + target + ".md#" + slug + ")"
		})
	}
	return strings.Join(lines, "\n")
}
