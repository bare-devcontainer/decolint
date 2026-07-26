package format

import "github.com/bare-devcontainer/decolint/linter"

// SGR (Select Graphic Rendition) parameters, limited to the attributes and eight basic colors every
// ANSI terminal implements. Several are combined with ";" to apply at once, e.g. bold red.
const (
	sgrBold       = "1"
	sgrDim        = "2"
	sgrGreen      = "32"
	sgrBoldRed    = "31;1"
	sgrBoldYellow = "33;1"
)

// styler decorates parts of a text report with ANSI escape sequences. The zero value writes plain
// text, so a caller that never colors its output can pass it as is.
type styler struct{ enabled bool }

// wrap returns text decorated with the SGR parameters sgr, or text unchanged when styling is off.
func (s styler) wrap(sgr, text string) string {
	if !s.enabled || text == "" {
		return text
	}
	return "\x1b[" + sgr + "m" + text + "\x1b[0m"
}

func (s styler) bold(text string) string  { return s.wrap(sgrBold, text) }
func (s styler) dim(text string) string   { return s.wrap(sgrDim, text) }
func (s styler) green(text string) string { return s.wrap(sgrGreen, text) }

// severity decorates text in the color that stands for sev: red for an error, yellow for a warning.
func (s styler) severity(sev linter.Severity, text string) string {
	if sev == linter.SeverityError {
		return s.wrap(sgrBoldRed, text)
	}
	return s.wrap(sgrBoldYellow, text)
}
