//go:build windows

package main

import (
	"golang.org/x/sys/windows"
)

// tuiConsoleUTF8 reports whether the console output code page is UTF-8.
// Box drawing falls back to ASCII otherwise.
func tuiConsoleUTF8() bool {
	cp, err := windows.GetConsoleOutputCP()
	if err != nil {
		return false
	}
	return cp == 65001
}
