//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

var tuiSavedMode uint32
var tuiModeSaved bool

// tuiEnter enables VT processing, hides the cursor, and returns a restore func.
func tuiEnter() (func(), error) {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err == nil {
		tuiSavedMode = mode
		tuiModeSaved = true
		const enableVirtualTerminalProcessing = 0x0004
		_ = windows.SetConsoleMode(h, mode|enableVirtualTerminalProcessing)
	}
	print("\x1b[?25l")
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		print("\x1b[?25h")
		if tuiModeSaved {
			_ = windows.SetConsoleMode(h, tuiSavedMode)
		}
	}
	return restore, nil
}

// tuiConsoleSize queries the real Windows console size.
func tuiConsoleSize() (int, int, error) {
	var info windows.ConsoleScreenBufferInfo
	h := windows.Handle(os.Stdout.Fd())
	if err := windows.GetConsoleScreenBufferInfo(h, &info); err != nil {
		return 0, 0, err
	}
	w := int(info.Window.Right-info.Window.Left) + 1
	hh := int(info.Window.Bottom-info.Window.Top) + 1
	return w, hh, nil
}
