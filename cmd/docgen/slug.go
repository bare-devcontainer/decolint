package main

import (
	"regexp"
	"strings"
)

// slugify computes a Markdown heading's anchor slug the way GitHub (and Hugo's default goldmark
// auto-heading-id extension) does: lowercase, drop everything but letters, digits, spaces and
// hyphens, then turn runs of spaces/hyphens into one hyphen. Verified against every heading in
// README.md to match the anchors already written by hand there.
func slugify(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == ' ', r == '-':
			b.WriteRune(r)
		}
	}
	return strings.Trim(collapseHyphens.ReplaceAllString(b.String(), "-"), "-")
}

var collapseHyphens = regexp.MustCompile(`[\s-]+`)

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
	for _, line := range strings.Split(body, "\n") {
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
	for level = 0; level < len(line) && level < 6 && line[level] == '#'; level++ {
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
