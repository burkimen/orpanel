//go:build !windows

package main

// tuiCodePageSwitch is a no-op on unix (UTF-8 assumed).
func tuiCodePageSwitch() func() { return func() {} }

// tuiConsoleSize reports the unix console size. The tick loop only needs a
// sane width class; tview recomputes exact geometry per draw from the
// screen, so a fixed fallback is fine here (matches the old behaviour).
func tuiConsoleSize() (int, int, error) {
	return 80, 24, nil
}
