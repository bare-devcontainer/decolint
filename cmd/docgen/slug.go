package main

import "strings"

// slugify computes a Markdown heading's anchor slug the way Hugo's default goldmark
// auto-heading-id extension does for a heading made of ASCII letters, digits, underscores, hyphens
// and spaces: lowercase, map each space to a hyphen, and drop every other character — with no
// collapsing of repeated hyphens or spaces, and no trimming of a leading or trailing hyphen (a
// heading like "-format" keeps its dash). Verified against Hugo's own output for every heading in
// README.md, including edge cases (an underscore, a leading hyphen) that a heading there doesn't
// currently exercise. Hugo's real algorithm also keeps non-ASCII letters (e.g. "ü"), which no
// heading in this repository uses, so that case is intentionally out of scope here.
func slugify(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// heading is one ATX heading found by scanHeadings: its level (1 for "#", 2 for "##", ...), text,
// and computed slug.
type heading struct {
	Level int
	Text  string
	Slug  string
}

// scanHeadings returns every ATX heading ("# ", "## ", ...) in body, skipping fenced code blocks so
// a "#" inside an example is never mistaken for one. README.md has none today, but a generator that
// assumed so would fail silently the day it does.
func scanHeadings(body string) []heading {
	var out []heading
	fenced := false
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		level, text, ok := atxHeading(line)
		if !ok {
			continue
		}
		out = append(out, heading{Level: level, Text: text, Slug: slugify(text)})
	}
	return out
}

// atxHeading parses line as an ATX heading ("# Title" through "###### Title"), returning its level
// and text with any trailing "#"s trimmed.
func atxHeading(line string) (level int, text string, ok bool) {
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0, "", false
	}
	text = strings.TrimSpace(strings.TrimRight(line[level+1:], "#"))
	if text == "" {
		return 0, "", false
	}
	return level, text, true
}
