package main

import (
	"strings"
)

// TUI action ids returned by handleKey.
const (
	tuiActNone = iota
	tuiActQuit
	tuiActStart
	tuiActStop
	tuiActRestart
	tuiActUpdate
	tuiActRepair
	tuiActInstall
	tuiActAutostart
	tuiActLanguage
	tuiActTheme
	tuiActWebUI
	tuiActHelp
	tuiActScrollUp
	tuiActScrollDown
	tuiActPaneNext
	tuiActConfirmCancel
)

// TUI panes.
const (
	tuiPaneActions = iota
	tuiPaneLogs
)

type tuiKey struct {
	r   rune
	esc bool // escape-prefixed sequence (arrows etc)
	raw string
}

type tuiState struct {
	pane     int
	sel      int
	logOff   int // 0 = auto-scroll
	showHelp bool
	confirm  int // pending confirm action id, 0 = none
	rows     []tuiRow
	quit     bool
	lastAct  int
}

type tuiRow struct {
	id   int
	key  string
	text string
}

func tuiActionRows(t map[string]string) []tuiRow {
	tr := func(k, fb string) string {
		if v, ok := t[k]; ok && v != "" {
			return v
		}
		return fb
	}
	return []tuiRow{
		{id: tuiActStart, key: "s", text: tr("TuiStart", "Start")},
		{id: tuiActStop, key: "x", text: tr("TuiStop", "Stop")},
		{id: tuiActRestart, key: "r", text: tr("TuiRestart", "Restart")},
		{id: tuiActUpdate, key: "u", text: tr("TuiUpdate", "Update")},
		{id: tuiActRepair, key: "R", text: tr("TuiRepair", "Repair")},
		{id: tuiActInstall, key: "i", text: tr("TuiInstall", "Install")},
		{id: tuiActAutostart, key: "a", text: tr("TuiAutostart", "Autostart")},
		{id: tuiActLanguage, key: "l", text: tr("TuiLanguage", "Language")},
		{id: tuiActTheme, key: "t", text: tr("TuiTheme", "Theme")},
		{id: tuiActWebUI, key: "w", text: tr("TuiWebUI", "Web UI")},
		{id: tuiActHelp, key: "?", text: tr("TuiHelp", "Help")},
	}
}

// handleKey is pure: transitions only, no side effects. Callers execute lastAct.
func handleKey(st tuiState, k tuiKey) tuiState {
	// Confirm pending first: Enter confirms, Esc cancels.
	if st.confirm != 0 {
		if k.r == '\r' || k.r == '\n' {
			st.lastAct = st.confirm
			st.confirm = 0
			st.showHelp = false
			return st
		}
		if k.esc {
			st.confirm = 0
			st.lastAct = tuiActConfirmCancel
			return st
		}
		st.lastAct = tuiActNone
		return st
	}
	n := len(st.rows)
	switch {
	case k.r == 'q' || k.r == 'Q' || k.raw == "\x03": // q or Ctrl+C
		st.quit = true
		st.lastAct = tuiActQuit
		return st
	case k.r == '?':
		st.showHelp = !st.showHelp
		st.lastAct = tuiActHelp
		return st
	case k.r == '\t' || k.raw == "backtab":
		if st.pane == tuiPaneActions {
			st.pane = tuiPaneLogs
		} else {
			st.pane = tuiPaneActions
		}
		st.lastAct = tuiActPaneNext
		return st
	case k.esc && (k.raw == "[C" || k.raw == "[D"):
		if st.pane == tuiPaneActions {
			st.pane = tuiPaneLogs
		} else {
			st.pane = tuiPaneActions
		}
		st.lastAct = tuiActPaneNext
		return st
	case k.r == 'h' || k.r == 'H':
		if st.pane == tuiPaneActions {
			st.lastAct = tuiActNone
			return st
		}
		st.pane = tuiPaneActions
		st.lastAct = tuiActPaneNext
		return st
	case k.r == 'l' || k.r == 'L':
		if st.pane == tuiPaneActions {
			if b, ok := tuiBindingByKey(k.r); ok {
				if tuiConfirmNeeded(b.act) {
					st.confirm = b.act
					st.lastAct = tuiActNone
				} else {
					st.lastAct = b.act
				}
				return st
			}
			st.lastAct = tuiActNone
			return st
		}
		st.pane = tuiPaneActions
		st.lastAct = tuiActPaneNext
		return st
	}
	if st.pane == tuiPaneLogs {
		switch {
		case k.r == 'j' || k.esc && (k.raw == "[B" || k.raw == "OB"):
			st.logOff++
			st.lastAct = tuiActScrollDown
		case k.r == 'k' || k.esc && (k.raw == "[A" || k.raw == "OA"):
			if st.logOff > 0 {
				st.logOff--
			}
			st.lastAct = tuiActScrollUp
		case k.raw == "[5~":
			st.logOff += 10
			st.lastAct = tuiActScrollDown
		case k.raw == "[6~":
			st.logOff -= 10
			if st.logOff < 0 {
				st.logOff = 0
			}
			st.lastAct = tuiActScrollUp
		case k.raw == "[H" || k.raw == "[1~":
			st.logOff = 0
			st.lastAct = tuiActScrollUp
		case k.raw == "[F" || k.raw == "[4~":
			st.logOff = 1 << 20
			st.lastAct = tuiActScrollDown
		case k.r == 'G':
			st.logOff = 1 << 20
			st.lastAct = tuiActScrollDown
		case k.r == 'g':
			st.logOff = 0
			st.lastAct = tuiActScrollUp
		default:
			st.lastAct = tuiActNone
		}
		if st.logOff < 0 {
			st.logOff = 0
		}
		return st
	}
	switch {
	case k.r == 'j' || k.esc && (k.raw == "[B" || k.raw == "OB"):
		if n > 0 && st.sel < n-1 {
			st.sel++
		}
		st.lastAct = tuiActNone
	case k.r == 'k' || k.esc && (k.raw == "[A" || k.raw == "OA"):
		if st.sel > 0 {
			st.sel--
		}
		st.lastAct = tuiActNone
	case k.r == '\r' || k.r == '\n':
		if n > 0 && st.sel >= 0 && st.sel < n {
			id := st.rows[st.sel].id
			if tuiConfirmNeeded(id) {
				st.confirm = id
				st.lastAct = tuiActNone
			} else {
				st.lastAct = id
			}
		} else {
			st.lastAct = tuiActNone
		}
	default:
		if b, ok := tuiBindingByKey(k.r); ok && k.r != 0 {
			if tuiConfirmNeeded(b.act) {
				st.confirm = b.act
				st.lastAct = tuiActNone
			} else {
				st.lastAct = b.act
			}
		} else {
			st.lastAct = tuiActNone
		}
	}
	return st
}

type tuiSnapshot struct {
	appVer     string
	omniVer    string
	lang       string
	theme      string
	status     string
	probe      string
	probeLabel string
	port       string
	nodeVer    string
	external   bool
	uptime     string
	opPhase    string
	logs       []string
	msg        string
	help       bool
	clock      string
}

// tuiLogWindow returns the visible log window: last N lines, or scrolled.
func tuiLogWindow(logs []string, height, off int) []string {
	if height <= 0 {
		return nil
	}
	if off <= 0 {
		if len(logs) <= height {
			return logs
		}
		return logs[len(logs)-height:]
	}
	// Manual scroll: off counts lines up from the bottom.
	start := len(logs) - height - off
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > len(logs) {
		end = len(logs)
	}
	return logs[start:end]
}

// tuiLevel classifies a log line for colouring in the live log pane.
func tuiLevel(line string) string {
	up := strings.ToUpper(line)
	switch {
	case strings.Contains(up, "ERROR") || strings.Contains(up, "FAIL"):
		return "err"
	case strings.Contains(up, "WARN"):
		return "warn"
	default:
		return "info"
	}
}
