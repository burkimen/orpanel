//go:build !windows

package main

// tuiEnter is a no-op on unix here: ANSI works without setup, and the loop
// hides/shows the cursor with escape codes balanced on every exit path.
func tuiEnter() (func(), error) {
	print("\x1b[?25l")
	restored := false
	return func() {
		if restored {
			return
		}
		restored = true
		print("\x1b[?25h")
	}, nil
}

// tuiConsoleSize degrades on unix: TERM columns/rows via env, else 80x24.
func tuiConsoleSize() (int, int, error) {
	return 80, 24, nil
}
