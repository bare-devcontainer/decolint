package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// colorMode is when the text output should be colored, as given by the -color flag.
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

// String returns the mode's name, as used in the -color flag.
func (m colorMode) String() string {
	switch m {
	case colorAlways:
		return "always"
	case colorNever:
		return "never"
	default:
		return "auto"
	}
}

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
//     emits escape sequences where they were declared unwanted; -color=always still forces them.
//   - FORCE_COLOR set to anything but "0" turns color on, for a destination that renders escape
//     sequences without being a terminal, e.g. a CI log.
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
	if force := getenv("FORCE_COLOR"); force != "" && force != "0" {
		return true
	}
	return tty && getenv("TERM") != "dumb"
}

// isTerminal reports whether w is a terminal. A writer that is not a file never is, so a report
// captured in memory or piped into another command stays plain.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
