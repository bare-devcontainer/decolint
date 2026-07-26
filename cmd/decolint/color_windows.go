package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// rendersEscapeSequences reports whether the console behind f renders ANSI escape sequences rather
// than printing them literally, which a Windows console does only with virtual terminal processing
// enabled: Windows Terminal and other ConPTY hosts enable it, a legacy console does not. decolint
// does not enable it itself, since the console mode outlives the process.
func rendersEscapeSequences(f *os.File) bool {
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode); err != nil {
		return false
	}
	return mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0
}
