//go:build !windows

package main

// tcell owns all unix console input; no raw-mode code of ours remains.
// tuiEnter is kept for the debug log contract: with ORPANEL_TUI_DEBUG=1 it
// logs enter/leave lines, otherwise it is a no-op returning a no-op restore.
func tuiEnter() (func(), error) {
	if tuiDebugOn() {
		tuiDebugLog("TUI console enter: tcell owns input (unix)")
	}
	return func() {
		if tuiDebugOn() {
			tuiDebugLog("TUI console leave: tcell restored (unix)")
		}
	}, nil
}
// tuiCodePageSwitch is a no-op on unix (UTF-8 assumed).
func tuiCodePageSwitch() func() { return func() {} }

// tuiConsoleSize is used for pre-layout math; tview recomputes per draw.
