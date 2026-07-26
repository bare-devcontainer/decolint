//go:build !windows

package main

import (
	"os"
	"testing"
)

func TestRendersEscapeSequences(t *testing.T) {
	t.Parallel()

	if !rendersEscapeSequences(os.Stdout) {
		t.Error("rendersEscapeSequences(os.Stdout) = false, want true")
	}
}
