//go:build !windows

package main

import "os"

// rendersEscapeSequences reports whether the terminal behind f renders ANSI escape sequences rather
// than printing them literally. Outside Windows every terminal does.
func rendersEscapeSequences(*os.File) bool { return true }
