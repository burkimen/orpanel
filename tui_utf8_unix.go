//go:build !windows

package main

// tuiConsoleUTF8 assumes UTF-8 on unix terminals.
func tuiConsoleUTF8() bool { return true }
