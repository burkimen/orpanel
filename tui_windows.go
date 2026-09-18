//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// tcell owns all Windows console input (ConIn raw mode, arrows, PgUp/Dn,
// Home/End, mouse); no raw-mode code of ours remains in the path. The mode
// seam below exists so tests can assert save/restore ordering, and the debug
// log contract (ORPANEL_TUI_DEBUG=1) is preserved.

// tuiModeOps is the injectable seam for console-mode save/apply/restore.
type tuiModeOps struct {
	get func(h windows.Handle, mode *uint32) error
	set func(h windows.Handle, mode uint32) error
}

func realTuiModeOps() tuiModeOps {
	return tuiModeOps{get: windows.GetConsoleMode, set: windows.SetConsoleMode}
}

// tuiConsoleModeRoundTrip saves the mode, applies fn, and restores the saved
// mode. Every exit path in runTuiApp goes through tcell's own restore plus
// this ordering guarantee in tests.
func tuiConsoleModeRoundTrip(ops tuiModeOps, h windows.Handle, fn func(applied uint32)) uint32 {
	var saved uint32
	if err := ops.get(h, &saved); err != nil {
		return 0
	}
	fn(saved)
	_ = ops.set(h, saved)
	return saved
}

// tuiCodePageSwitch saves the console output code page and switches to
// UTF-8 (65001) before tcell initialises, so the Unicode box runes render
// instead of mojibake. Returns a restore func run on every exit path.
func tuiCodePageSwitch() func() {
	saved, err := windows.GetConsoleOutputCP()
	if err != nil {
		if tuiDebugOn() {
			tuiDebugLog("TUI code page: query failed (%v), leaving as-is; box runes may mojibake", err)
		}
		return func() {}
	}
	if saved == 65001 {
		return func() {}
	}
	if err := windows.SetConsoleOutputCP(65001); err != nil {
		if tuiDebugOn() {
			tuiDebugLog("TUI code page: SetConsoleOutputCP(65001) failed (%v), keeping %d; box runes may mojibake", err, saved)
		}
		return func() {}
	}
	if tuiDebugOn() {
		tuiDebugLog("TUI code page: %d -> 65001 (UTF-8) for box drawing", saved)
	}
	return func() {
		_ = windows.SetConsoleOutputCP(saved)
		if tuiDebugOn() {
			tuiDebugLog("TUI code page: restored to %d", saved)
		}
	}
}

// tuiEnter logs the debug contract; tcell handles the real console modes.
func tuiEnter() (func(), error) {
	if tuiDebugOn() {
		tuiDebugLog("TUI console enter: tcell owns input (windows)")
	}
	return func() {
		if tuiDebugOn() {
			tuiDebugLog("TUI console leave: tcell restored (windows)")
		}
	}, nil
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
