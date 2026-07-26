package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestParseColorMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    colorMode
		wantErr bool
	}{
		{"empty defaults to auto", "", colorAuto, false},
		{"auto", "auto", colorAuto, false},
		{"always", "always", colorAlways, false},
		{"never", "never", colorNever, false},
		{"matched case-insensitively", "Always", colorAlways, false},
		{"unknown", "sometimes", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseColorMode(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseColorMode(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseColorMode(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestUseColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode colorMode
		tty  bool
		env  map[string]string
		want bool
	}{
		{name: "auto on a terminal", mode: colorAuto, tty: true, want: true},
		{name: "auto off a terminal", mode: colorAuto, tty: false, want: false},
		{name: "auto on a dumb terminal", mode: colorAuto, tty: true, env: map[string]string{"TERM": "dumb"}, want: false},
		{name: "always off a terminal", mode: colorAlways, tty: false, want: true},
		{name: "never on a terminal", mode: colorNever, tty: true, want: false},
		{name: "NO_COLOR on a terminal", mode: colorAuto, tty: true, env: map[string]string{"NO_COLOR": "1"}, want: false},
		{name: "NO_COLOR empty is not set", mode: colorAuto, tty: true, env: map[string]string{"NO_COLOR": ""}, want: true},
		{name: "NO_COLOR loses to -color=always", mode: colorAlways, tty: true, env: map[string]string{"NO_COLOR": "1"}, want: true},
		{name: "NO_COLOR wins over FORCE_COLOR", mode: colorAuto, tty: true, env: map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"}, want: false},
		{name: "FORCE_COLOR off a terminal", mode: colorAuto, tty: false, env: map[string]string{"FORCE_COLOR": "1"}, want: true},
		{name: "FORCE_COLOR=0 on a terminal", mode: colorAuto, tty: true, env: map[string]string{"FORCE_COLOR": "0"}, want: false},
		{name: "FORCE_COLOR=0 off a terminal", mode: colorAuto, tty: false, env: map[string]string{"FORCE_COLOR": "0"}, want: false},
		{name: "FORCE_COLOR empty is not set", mode: colorAuto, tty: true, env: map[string]string{"FORCE_COLOR": ""}, want: true},
		{name: "FORCE_COLOR loses to -color=never", mode: colorNever, tty: false, env: map[string]string{"FORCE_COLOR": "1"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			getenv := func(name string) string { return tt.env[name] }
			if got := useColor(tt.mode, tt.tty, getenv); got != tt.want {
				t.Errorf("useColor(%v, tty=%v, %v) = %v, want %v", tt.mode, tt.tty, tt.env, got, tt.want)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	t.Parallel()

	// A real terminal cannot be opened portably, so only the negative cases are covered here: the
	// destinations decolint is redirected to in practice.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	file, err := os.Create(filepath.Join(t.TempDir(), "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })

	// os.DevNull is a character device, as a terminal is, so it is only told apart from one by
	// asking the device itself rather than by reading the file mode.
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = devNull.Close() })

	tests := []struct {
		name string
		w    io.Writer
	}{
		{"in-memory buffer", &bytes.Buffer{}},
		{"pipe", w},
		{"regular file", file},
		{os.DevNull, devNull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if isTerminal(tt.w) {
				t.Errorf("isTerminal(%s) = true, want false", tt.name)
			}
		})
	}
}
