package main

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
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
)

// TUI panes.
const (
	tuiPaneActions = iota
	tuiPaneLogs
)

type tuiKey struct {
	r   rune
	esc bool // escape-prefixed sequence (arrows)
	raw string
}

func tuiKeyRune(r rune) tuiKey { return tuiKey{r: r} }

type tuiState struct {
	pane      int
	sel       int
	logOff    int // 0 = auto-scroll
	showHelp  bool
	rows      []tuiRow
	quit      bool
	lastAct   int
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
		{id: tuiActAutostart, key: "a", text: tr("TuiAutostart", "Autostart")},
		{id: tuiActLanguage, key: "l", text: tr("TuiLanguage", "Language")},
		{id: tuiActTheme, key: "t", text: tr("TuiTheme", "Theme")},
		{id: tuiActWebUI, key: "w", text: tr("TuiWebUI", "Web UI")},
		{id: tuiActHelp, key: "?", text: tr("TuiHelp", "Help")},
	}
}

// handleKey is pure: transitions only, no side effects. Callers execute lastAct.
func handleKey(st tuiState, k tuiKey) tuiState {
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
	case k.r == '\t':
		if st.pane == tuiPaneActions {
			st.pane = tuiPaneLogs
		} else {
			st.pane = tuiPaneActions
		}
		st.lastAct = tuiActPaneNext
		return st
	}
	if st.pane == tuiPaneLogs {
		switch {
		case k.r == 'j' || k.esc && k.raw == "[B":
			st.logOff++
			st.lastAct = tuiActScrollDown
		case k.r == 'k' || k.esc && k.raw == "[A":
			if st.logOff > 0 {
				st.logOff--
			}
			st.lastAct = tuiActScrollUp
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
	case k.r == 'j' || k.esc && k.raw == "[B":
		if n > 0 && st.sel < n-1 {
			st.sel++
		}
		st.lastAct = tuiActNone
	case k.r == 'k' || k.esc && k.raw == "[A":
		if st.sel > 0 {
			st.sel--
		}
		st.lastAct = tuiActNone
	case k.r == '\r' || k.r == '\n':
		if n > 0 && st.sel >= 0 && st.sel < n {
			st.lastAct = st.rows[st.sel].id
		} else {
			st.lastAct = tuiActNone
		}
	case k.r == 's':
		st.lastAct = tuiActStart
	case k.r == 'x':
		st.lastAct = tuiActStop
	case k.r == 'r':
		st.lastAct = tuiActRestart
	case k.r == 'u':
		st.lastAct = tuiActUpdate
	case k.r == 'a':
		st.lastAct = tuiActAutostart
	case k.r == 'l':
		st.lastAct = tuiActLanguage
	case k.r == 't':
		st.lastAct = tuiActTheme
	case k.r == 'w':
		st.lastAct = tuiActWebUI
	case k.r == 'i':
		st.lastAct = tuiActInstall
	default:
		st.lastAct = tuiActNone
	}
	return st
}

type tuiSnapshot struct {
	appVer   string
	omniVer  string
	lang     string
	theme    string
	status   string
	probe    string
	port     string
	nodeVer  string
	external bool
	uptime   string
	opPhase  string
	logs     []string
	msg      string
	help     bool
}

func visibleLen(s string) int {
	n := 0
	inEsc := false
	for i := 0; i < len(s); {
		if !inEsc && s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			inEsc = true
			i += 2
			continue
		}
		if inEsc {
			if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
				inEsc = false
			}
			i++
			continue
		}
		_, w := utf8.DecodeRuneInString(s[i:])
		n++
		i += w
	}
	return n
}

func tuiTrunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if visibleLen(s) <= w {
		return s
	}
	if w <= 1 {
		return "…"
	}
	// Truncate by runes, reserving one cell for the ellipsis.
	runes := []rune(s)
	if len(runes) <= w-1 {
		return s
	}
	return string(runes[:w-1]) + "…"
}

func tuiPad(s string, w int) string {
	n := visibleLen(s)
	if n >= w {
		return tuiTrunc(s, w)
	}
	return s + strings.Repeat(" ", w-n)
}

func tuiBox(ascii bool) (tl, tr, bl, br, h, v string) {
	if ascii {
		return "+", "+", "+", "+", "-", "|"
	}
	return "┌", "┐", "└", "┘", "─", "│"
}

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

// tuiLogLines returns the visible log window: last N lines, or scrolled.
func tuiLogLines(logs []string, height, off int) []string {
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

// composeFrame renders one full frame at exactly width x height visible cells.
func composeFrame(st tuiState, snap tuiSnapshot, t map[string]string, width, height int, now time.Time) string {
	_ = now
	if width < 40 {
		width = 40
	}
	if height < 10 {
		height = 10
	}
	ascii := !tuiUTF8()
	tl, tr, bl, br, h, v := tuiBox(ascii)
	trf := func(k, fb string) string {
		if s, ok := t[k]; ok && s != "" {
			return s
		}
		return fb
	}
	var b strings.Builder
	bar := func() string { return tl + strings.Repeat(h, width-2) + tr }
	mid := func(inner string) string { return v + tuiPad(inner, width-2) + v }
	b.WriteString(bar() + "\n")
	b.WriteString(mid(fmt.Sprintf(" OrPanel v%s  OmniRoute %s  %s/%s", snap.appVer, snap.omniVer, snap.lang, snap.theme)) + "\n")
	b.WriteString(mid(fmt.Sprintf(" %s: %s  probe:%s  port:%s  node:%s", trf("TuiStatus", "Status"), snap.status, snap.probe, snap.port, snap.nodeVer)) + "\n")
	if snap.external {
		b.WriteString(mid(" "+trf("TuiExternalNote", "Externally managed")) + "\n")
	}
	if snap.opPhase != "" {
		b.WriteString(mid(" "+snap.opPhase) + "\n")
	}
	b.WriteString(mid(fmt.Sprintf(" %s:", trf("TuiLogs", "Logs"))) + "\n")
	// Reserve: top border + 2 header + status/ext/op + logs label + actions + footer(2) + bottom.
	used := 1 + 3 + 1
	if snap.external {
		used++
	}
	if snap.opPhase != "" {
		used++
	}
	actLines := len(st.rows) + 1 // label + rows
	logH := height - used - actLines - 2 - 1
	if logH < 1 {
		logH = 1
	}
	for _, ln := range tuiLogLines(snap.logs, logH, st.logOff) {
		b.WriteString(mid(" "+tuiTrunc(ln, width-4)) + "\n")
	}
	b.WriteString(mid(fmt.Sprintf(" %s:", trf("TuiActions", "Actions"))) + "\n")
	for i, r := range st.rows {
		mark := "  "
		if st.pane == tuiPaneActions && i == st.sel {
			mark = "> "
		}
		b.WriteString(mid(fmt.Sprintf(" %s[%s] %s", mark, r.key, r.text)) + "\n")
	}
	foot := "q quit · ? help · Tab pane"
	if t, ok := t["TuiPressQ"]; ok && t != "" {
		foot = "q/" + t
	}
	if snap.msg != "" {
		foot = tuiTrunc(snap.msg, width-2)
	}
	b.WriteString(mid(" " + foot) + "\n")
	if st.showHelp {
		b.WriteString(mid(fmt.Sprintf(" %s: s x r u a l t w ? q Tab j/k", trf("TuiHelpTitle", "Keys"))) + "\n")
	}
	b.WriteString(bl + strings.Repeat(h, width-2) + br)
	out := b.String()
	// Normalize to exactly height lines.
	lines := strings.Split(out, "\n")
	for len(lines) < height {
		lines = append(lines, mid(""))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func tuiUTF8() bool {
	return tuiConsoleUTF8()
}
