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
	tuiActConfirmOK
	tuiActConfirmCancel
	// Section pseudo-ids: open sub-lists, never dispatched to tuiDoAction.
	tuiActBakim
	tuiActAyarlar
)

// tuiSection selects which action List is visible: top level or a sub-list.
type tuiSection int

const (
	tuiSecTop tuiSection = iota
	tuiSecBakim
	tuiSecAyarlar
)

// TUI panes.
const (
	tuiPaneActions = iota
	tuiPaneLogs
)

type tuiState struct {
	pane    int
	sel     int
	logOff  int // 0 = auto-scroll
	showHelp bool
	confirm int // pending confirm action id, 0 = none
	rows    []tuiRow
	quit    bool
	lastAct int
	sec     tuiSection // visible action List
}

// tuiEntry is one row of an action List. Sections (Bakım/Ayarlar) are
// pseudo-entries: selected by the same index, opened by Enter/click, never
// dispatched to tuiDoAction (see tuiEntryIsSection).
type tuiRow = tuiEntry

type tuiEntry struct {
	id      int
	key     string
	text    string
	section tuiSection // tuiSecTop for state actions, else owning sub-list
}

type tuiKey struct {
	r   rune
	esc bool // escape-prefixed sequence (arrows etc)
	raw string
}

func tuiKeyRune(r rune) tuiKey { return tuiKey{r: r} }

// tuiEntriesFor builds the visible entry list for one section in one state.
// Single source of truth: one List per section renders exactly this slice,
// Enter dispatches entries[i].id, help/overflow read the same slice.
func tuiEntriesFor(sec tuiSection, t map[string]string, snap tuiSnapshot) []tuiEntry {
	tr := func(k, fb string) string {
		if v, ok := t[k]; ok && v != "" {
			return v
		}
		return fb
	}
	state := snap.status
	bak := tuiEntry{id: tuiActBakim, text: "▸ " + tr("TuiBakim", "Bakım")}
	aya := tuiEntry{id: tuiActAyarlar, text: "▸ " + tr("TuiAyarlar", "Ayarlar")}
	web := tuiEntry{id: tuiActWebUI, key: "w", text: tr("TuiWebUI", "Web UI")}
	auto := tuiEntry{id: tuiActAutostart, key: "a", text: tr("TuiAutostart", "Autostart")}
	lang := tuiEntry{id: tuiActLanguage, key: "l", text: tr("TuiLanguage", "Language")}
	them := tuiEntry{id: tuiActTheme, key: "t", text: tr("TuiTheme", "Theme")}
	start := tuiEntry{id: tuiActStart, key: "s", text: tr("TuiStart", "Start")}
	stop := tuiEntry{id: tuiActStop, key: "x", text: tr("TuiStop", "Stop")}
	rest := tuiEntry{id: tuiActRestart, key: "r", text: tr("TuiRestart", "Restart")}
	upd := tuiEntry{id: tuiActUpdate, key: "u", text: tr("TuiUpdate", "Update")}
	rep := tuiEntry{id: tuiActRepair, key: "R", text: tr("TuiRepair", "Repair")}
	inst := tuiEntry{id: tuiActInstall, key: "i", text: tr("TuiInstall", "Install")}
	updAvail := snap.updateAvail
	switch sec {
	case tuiSecBakim:
		out := []tuiEntry{inst, rep, upd}
		if updAvail {
			out = []tuiEntry{upd, inst, rep}
		}
		for i := range out {
			out[i].section = tuiSecBakim
		}
		return out
	case tuiSecAyarlar:
		out := []tuiEntry{auto, lang, them, web}
		for i := range out {
			out[i].section = tuiSecAyarlar
		}
		return out
	default:
		switch {
		case state == "not_installed":
			return []tuiEntry{inst, web, lang, them}
		case state == "running":
			if updAvail {
				return []tuiEntry{stop, rest, upd, bak, aya}
			}
			return []tuiEntry{stop, rest, bak, aya}
		case state == "installing" || state == "op":
			return []tuiEntry{web, lang}
		default: // stopped, unreachable, port_conflict (retry), unknown
			return []tuiEntry{start, bak, aya}
		}
	}
}

// tuiEntryIsSection reports whether id opens a sub-list (never dispatched).
func tuiEntryIsSection(id int) bool {
	return id == tuiActBakim || id == tuiActAyarlar
}

// tuiSectionFor maps a section pseudo-id to its sub-list.
func tuiSectionFor(id int) tuiSection {
	switch id {
	case tuiActBakim:
		return tuiSecBakim
	case tuiActAyarlar:
		return tuiSecAyarlar
	default:
		return tuiSecTop
	}
}

// tuiActionRows keeps the old name for the top-level running list; new code
// should call tuiEntriesFor. Help + legacy tests pin through here.
func tuiActionRows(t map[string]string) []tuiRow {
	return tuiEntriesFor(tuiSecTop, t, tuiSnapshot{status: "running", updateAvail: true})
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
	case k.r == 'q' || k.r == 'Q':
		// Quit always confirms (same gate as Kur/Onar/Güncelle/Durdur/
		// Yeniden Başlat); the dialog says the panel keeps running.
		st.confirm = tuiActQuit
		st.lastAct = tuiActNone
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
	case k.esc && k.raw != "[A" && k.raw != "OA" && k.raw != "[B" && k.raw != "OB" && k.raw != "[5~" && k.raw != "[6~" && k.raw != "[H" && k.raw != "[1~" && k.raw != "[F" && k.raw != "[4~":
		// Esc in a sub-list backs out to top; on top it closes help.
		// (Arrow/paging raws excluded: they carry esc=true and are
		// handled by the pane branches below.)
		if st.sec != tuiSecTop {
			st.sec = tuiSecTop
			st.sel = 0
			st.lastAct = tuiActNone
			return st
		}
		if st.showHelp {
			st.showHelp = false
			st.lastAct = tuiActNone
			return st
		}
		st.lastAct = tuiActNone
		return st
	}
	if st.pane == tuiPaneLogs {
		switch {
		case k.esc && (k.raw == "[B" || k.raw == "OB"):
			st.logOff++
			st.lastAct = tuiActScrollDown
		case k.esc && (k.raw == "[A" || k.raw == "OA"):
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
		default:
			st.lastAct = tuiActNone
		}
		if st.logOff < 0 {
			st.logOff = 0
		}
		return st
	}
	switch {
	case k.esc && (k.raw == "[B" || k.raw == "OB"):
		if n > 0 && st.sel < n-1 {
			st.sel++
		}
		st.lastAct = tuiActNone
	case k.esc && (k.raw == "[A" || k.raw == "OA"):
		if st.sel > 0 {
			st.sel--
		}
		st.lastAct = tuiActNone
	case k.r == '\r' || k.r == '\n':
		if n > 0 && st.sel >= 0 && st.sel < n {
			id := st.rows[st.sel].id
			if tuiEntryIsSection(id) {
				st.sec = tuiSectionFor(id)
				st.sel = 0
				st.lastAct = tuiActNone
			} else if tuiConfirmNeeded(id) || id == tuiActQuit {
				st.confirm = id
				st.lastAct = tuiActNone
			} else {
				st.lastAct = id
			}
		} else {
			st.lastAct = tuiActNone
		}
	default:
		st.lastAct = tuiActNone
	}
	return st
}

type tuiSnapshot struct {
	appVer     string
	theme      string
	status     string
	probe      string
	probeLabel string
	port       string
	nodeVer    string
	external   bool
	updateAvail bool
	uptime     string
	opPhase    string
	logs       []string
	msg        string
	help       bool
	clock      string
	omniVer    string
	lang       string
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
