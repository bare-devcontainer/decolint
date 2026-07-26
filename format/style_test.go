package format

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
)

func TestStylerWrap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		st   styler
		text string
		want string
	}{
		{name: "enabled", st: styler{enabled: true}, text: "hello", want: "\x1b[1mhello\x1b[0m"},
		{name: "disabled", st: styler{}, text: "hello", want: "hello"},
		{name: "empty text is never decorated", st: styler{enabled: true}, text: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.st.wrap(sgrBold, tt.text); got != tt.want {
				t.Errorf("wrap(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestStylerSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sev  linter.Severity
		want string
	}{
		{name: "error is red", sev: linter.SeverityError, want: "\x1b[31;1mtext\x1b[0m"},
		{name: "warn is yellow", sev: linter.SeverityWarn, want: "\x1b[33;1mtext\x1b[0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := (styler{enabled: true}).severity(tt.sev, "text"); got != tt.want {
				t.Errorf("severity(%v) = %q, want %q", tt.sev, got, tt.want)
			}
		})
	}
}
