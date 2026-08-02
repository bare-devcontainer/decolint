package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// colorMode is when the text output should be colored, as given by the --color flag.
type colorMode int

const (
	// colorAuto colors the output only when it goes to a terminal that is not known to reject
	// escape sequences; see useColor.
	colorAuto colorMode = iota
	// colorAlways colors the output whatever it is written to.
	colorAlways
	// colorNever writes plain text.
	colorNever
)

// parseColorMode parses a color mode name, matched case-insensitively, into a colorMode. An empty
// name yields colorAuto. It returns an error if name does not name a known mode.
func parseColorMode(name string) (colorMode, error) {
	switch strings.ToLower(name) {
	case "", "auto":
		return colorAuto, nil
	case "always":
		return colorAlways, nil
	case "never":
		return colorNever, nil
	default:
		return 0, fmt.Errorf("unknown color mode %q (want one of: auto, always, never)", name)
	}
}

// useColor reports whether the text output should be colored. mode decides on its own unless it is
// colorAuto, in which case the environment does, in this order:
//
//   - NO_COLOR set to a non-empty value turns color off. It wins over FORCE_COLOR, so decolint never
//     emits escape sequences where they were declared unwanted; --color=always still forces them.
//   - FORCE_COLOR decides on its own: "0" turns color off, any other non-empty value turns it on,
//     for a destination that renders escape sequences without being a terminal, e.g. a CI log.
//   - A "dumb" terminal, or a destination that is not a terminal at all, turns color off.
//
// tty reports whether the destination is a terminal (see isTerminal) and getenv reads the
// environment, both passed in so the decision itself is independent of the process.
func useColor(mode colorMode, tty bool, getenv func(string) string) bool {
	switch mode {
	case colorAlways:
		return true
	case colorNever:
		return false
	}
	if getenv("NO_COLOR") != "" {
		return false
	}
	if force := getenv("FORCE_COLOR"); force != "" {
		return force != "0"
	}
	return tty && getenv("TERM") != "dumb"
}

// isTerminal reports whether w is a terminal that renders escape sequences. Anything else is not,
// including a character device such as /dev/null, so a report piped into another command or
// redirected anywhere stays free of escape sequences.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd())) && rendersEscapeSequences(f)
}
