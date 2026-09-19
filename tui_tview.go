package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// tui_tview.go: the tview application. tcell owns all console input
// (Windows ConIn raw mode, arrows, PgUp/Dn, Home/End, mouse), so none of
// our old raw-mode code remains in the path. Key handling still flows
// through handleKey (pure) driven by tuiEventKey, keeping unit tests valid.

// tuiNoColor reports whether colour output is disabled.
func tuiNoColor() bool {
	if v := os.Getenv("NO_COLOR"); v != "" {
		return true
	}
	return false
}

// tuiProbeLabel maps internal probe states to localized human labels.
// Unknown values never leak enums: they read as "checking".
func tuiProbeLabel(probe string, t map[string]string) string {
	tr := func(k, fb string) string {
		if v, ok := t[k]; ok && v != "" {
			return v
		}
		return fb
	}
	switch probe {
	case "healthy":
		return tr("ProbeHealthy", "Running")
	case "degraded":
		return tr("ProbeDegraded", "Not responding")
	case "unreachable":
		return tr("ProbeUnreachable", "Down")
	case "starting":
		return tr("ProbeStarting", "Starting")
	case "recovering":
		return tr("ProbeRecovering", "Recovering")
	default:
		return tr("ProbeUnknown", "Checking")
	}
}

// tuiStatusLabel maps internal omni status to localized human labels.
func tuiStatusLabel(status string, t map[string]string) string {
	tr := func(k, fb string) string {
		if v, ok := t[k]; ok && v != "" {
			return v
		}
		return fb
	}
	switch status {
	case "running":
		return tr("HealthBadgeRunning", "Running")
	case "stopped":
		return tr("HealthBadgeStopped", "Stopped")
	case "not_installed":
		return tr("HealthBadgeNotInstalled", "Not installed")
	case "port_conflict":
		return tr("HealthBadgePortConflict", "Port conflict")
	case "corrupt":
		return tr("HealthBadgeCorrupt", "Corrupt")
	case "installing":
		return tr("HealthBadgeInstalling", "Installing")
	default:
		return tr("ProbeUnknown", "Checking")
	}
}

type tuiApp struct {
	app       *tview.Application
	pages     *tview.Pages
	body      *tview.Flex
	status    *tview.TextView
	logs      *tview.TextView
	header    *tview.TextView
	footer    *tview.TextView
	mu        sync.Mutex // guards st/msg/probe/focus-class below; event loop vs tick/test
	msg       string
	msgAt     time.Time
	st        tuiState
	t         map[string]string
	follow    bool
	probe     string
	probeAt   time.Time
	progPhase string
	wide      bool // layout class: true when width >= 100
	lastW     int
	lastH     int
	snap      tuiSnapshot // last rendered snapshot (sim harness reads it)
	ttooSmall string      // cached too-small text for the sim guard
	// TUI 2.0: one List per section. The visible List shows exactly the
	// entries of tuiEntriesFor(sec); Enter/click dispatch entries[i].id.
	listTop *tview.List
	listBak *tview.List
	listAya *tview.List
	hover   int // hovered row in the visible List, -1 = none (OUR code)
	hoverList *tview.List // which List the hover belongs to
	// Client-mode log cursor: last newIndex served by /api/logs.
	logCursor int
	logReady  bool // false until the first client poll answers
}
// tuiStateSnapshot copies selection-relevant state for race-safe test reads.
func (a *tuiApp) stateSnapshot() tuiState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.st
}

// msgSnapshot copies the message line for race-safe test reads.
func (a *tuiApp) msgSnapshot() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.msg
}

// setPaneForTest switches the focused pane in tests.
func (a *tuiApp) setPaneForTest(p int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.st.pane = p
}

func tuiTr(key string, t map[string]string, fb string) string {
	if v, ok := t[key]; ok && v != "" {
		return v
	}
	return fb
}

// headerText degrades by dropping whole fields (never slicing words): full
// line at wide widths, version+identity only when narrow.
func headerText(width int, appVer, omniVer, lang, theme, clock string) string {
	full := fmt.Sprintf(" OrPanel v%s  OmniRoute %s  %s/%s  %s", appVer, omniVer, lang, theme, clock)
	if width <= 0 || len([]rune(full)) <= width {
		return full
	}
	short := fmt.Sprintf(" OrPanel v%s  OmniRoute %s", appVer, omniVer)
	if len([]rune(short)) <= width {
		return short
	}
	return fmt.Sprintf(" OrPanel v%s", appVer)
}

func newTuiApp() *tuiApp {
	a := &tuiApp{app: tview.NewApplication(), t: loadTranslations(getCurrentLang()), follow: true, hover: -1}
	a.status = tview.NewTextView().SetDynamicColors(!tuiNoColor()).SetScrollable(false)
	a.status.SetWrap(false)
	a.logs = tview.NewTextView().SetDynamicColors(!tuiNoColor()).SetScrollable(true)
	a.logs.SetWrap(false)
	a.header = tview.NewTextView().SetDynamicColors(!tuiNoColor())
	a.header.SetWrap(false)
	a.footer = tview.NewTextView().SetDynamicColors(!tuiNoColor())
	a.footer.SetWrap(false)
	for _, v := range []*tview.TextView{a.status, a.logs, a.header, a.footer} {
		v.SetBorder(false)
	}
	a.status.SetTitle(" " + tuiTr("TuiStatus", a.t, "Status") + " ")
	a.logs.SetTitle(" " + tuiTr("TuiLogs", a.t, "Logs") + " ")
	a.setListTitlesLocked(" " + tuiTr("TuiActions", a.t, "Actions") + " ")
	a.st = tuiState{pane: tuiPaneActions, rows: tuiActionRows(a.t)}
	a.listTop = newActionList(a)
	a.listBak = newActionList(a)
	a.listAya = newActionList(a)
	a.body = tview.NewFlex()
	a.pages = tview.NewPages()
	a.pages.AddPage("main", a.mainLayout(80), true, true)
	// The Application holds its own root pointer: without SetRoot every
	// draw() returns early (root == nil) and Show() paints only the
	// cleared screen. Set once here so all paths (prod, sim, tests) draw.
	a.app.SetRoot(a.pages, true)
	a.applyThemeLocked()
	a.syncListsLocked(tuiSnapshot{status: "running", updateAvail: true})
	a.wide = false
	return a
}

// newActionList builds one action List: keyboard nav + Enter + click +
// wheel all come from the List; selection is SetSelectedStyle reverse.
func newActionList(a *tuiApp) *tview.List {
	l := tview.NewList().ShowSecondaryText(false)
	l.SetWrapAround(false)
	l.SetSelectedStyle(tcell.StyleDefault.Reverse(true))
	l.SetSelectedFunc(func(index int, _ string, _ string, _ rune) {
		a.activateListIndex(index)
	})
	l.SetBorder(true)
	return l
}

// footerText is CONTEXTUAL: hints for the focused pane only, built from the
// keymap table (global + pane-local entries). It never duplicates the action
// bar. Each hint is one "key label" chip; overlong footers drop whole chips.
func (a *tuiApp) footerText(width int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.footerTextLocked(width)
}

func (a *tuiApp) footerTextLocked(width int) string {
	if width <= 0 {
		width = 80
	}
	var chips []string
	for _, b := range tuiFooterBindings(a.st.pane) {
		lbl := tuiTr(b.hint, a.t, "")
		if lbl == "" {
			lbl = tuiTr(b.label, a.t, tuiKeyName(b))
		}
		chips = append(chips, tuiKeyName(b)+" "+lbl)
	}
	joined := strings.Join(chips, " · ")
	for len([]rune(joined)) > width && len(chips) > 2 {
		chips = chips[:len(chips)-1]
		joined = strings.Join(chips, " · ")
	}
	return joined
}


// helpLines returns the help modal from the SAME entry data the Lists
// render and Enter dispatches: current section entries with * marks, then
// the visible navigation vocabulary. One line per entry, left aligned, in
// stable columns (key / label / * on the SAME line), grouped like the panes
// (state actions, then Bakım, then Ayarlar). Never wraps mid-word: callers
// fit whole lines or drop them, never reflow inside a word.
func (a *tuiApp) helpLines() []string {
	var lines []string
	lines = append(lines, tuiTr("TuiHelpActTitle", a.t, "Actions")+" ("+tuiTr("TuiConfirmLegend", a.t, "* needs confirm")+")")
	// Stable column widths so key/label/* read as a grid. Widths come
	// from the data (longest key/label in THIS list), not constants.
	// Groups mirror the panes: state actions, then Bakım …, then
	// Ayarlar … — section rows print their OWN sub-list entries
	// (Bakım ▸ Kur *), never just the ▸ opener, so help names every
	// action the panes can run.
	// NO letter keycaps anywhere (owner decision): rows are `label  [*]`
	// with the marker on the same line, grouped like the panes.
	lblW := 0
	type hrow struct {
		label, mark string
	}
	mkrow := func(e tuiEntry, prefix string) hrow {
		mark := " "
		if tuiConfirmNeeded(e.id) {
			mark = "*"
		}
		return hrow{label: oneLine(prefix + e.text), mark: mark}
	}
	var groups [][]hrow
	// Top group: state actions only (▸ openers live with their sub-list
	// group below, so no action is listed twice).
	var cur []hrow
	for _, r := range a.st.rows {
		if tuiEntryIsSection(r.id) {
			continue
		}
		cur = append(cur, mkrow(r, ""))
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	snap := a.snap
	for _, sec := range []tuiSection{tuiSecBakim, tuiSecAyarlar} {
		// Only list a section group when its ▸ opener is visible.
		open := false
		for _, r := range a.st.rows {
			if tuiSectionFor(r.id) == sec && tuiEntryIsSection(r.id) {
				open = true
				break
			}
		}
		if !open {
			continue
		}
		var sub []hrow
		for _, e := range tuiEntriesFor(sec, a.t, snap) {
			pfx := tuiTr("TuiBakim", a.t, "Bakım") + " ▸ "
			if sec == tuiSecAyarlar {
				pfx = tuiTr("TuiAyarlar", a.t, "Ayarlar") + " ▸ "
			}
			sub = append(sub, mkrow(e, pfx))
		}
		if len(sub) > 0 {
			groups = append(groups, sub)
		}
	}
	var rows []hrow
	for _, g := range groups {
		rows = append(rows, g...)
	}
	for _, r := range rows {
		if len([]rune(r.label)) > lblW {
			lblW = len([]rune(r.label))
		}
	}
	for gi, g := range groups {
		for _, r := range g {
			lines = append(lines, fmt.Sprintf(" %-*s %s", lblW, r.label, r.mark))
		}
		if gi < len(groups)-1 {
			lines = append(lines, "")
		}
	}
	lines = append(lines, "")
	// Navigation block: one pair per line, never joined mid-phrase.
	lines = append(lines, tuiTr("TuiHelpNavTitle", a.t, "Navigation"))
	for _, bd := range tuiFooterBindings(tuiPaneActions) {
		if bd.scope != tuiScopeGlobal && bd.scope != tuiScopePane {
			continue
		}
		lbl := tuiTr(bd.label, a.t, tuiKeyName(bd))
		lines = append(lines, fmt.Sprintf(" %-*s %s", 10, tuiKeyName(bd), lbl))
	}
	lines = append(lines, tuiTr("TuiMouseNote", a.t, "mouse on: click selects, wheel scrolls, Shift selects text"))
	return lines
}

// wrapPairs joins pairs with " · ", breaking only between pairs.
func wrapPairs(pairs []string, width int) []string {
	var out []string
	cur := ""
	for _, p := range pairs {
		if cur == "" {
			cur = " " + p
			continue
		}
		if len([]rune(cur+" · "+p)) > width {
			out = append(out, cur)
			cur = " " + p
			continue
		}
		cur += " · " + p
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (a *tuiApp) helpBody() string {
	return strings.Join(a.helpLines(), "\n")
}

// oneLine collapses embedded newlines so each binding stays on one row.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// closeModalSync removes the modal page and clears overlay-scoped state so
// it matches reality. Single close path for the modal done func (live
// Esc/buttons) and the dump-harness Esc fallback — no stale showHelp.
func (a *tuiApp) closeModalSync() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pages.RemovePage("modal")
	a.st.showHelp = false
	a.st.confirm = 0
	a.st.lastAct = tuiActNone
	a.refreshLocked()
	a.applyFocusLocked()
}
func (a *tuiApp) showModal(title, body, hint string, buttons []string, onOK func(), onClose func()) {
	// Word-boundary fit only: compute the longest line that fits, break
	// between words, never inside a word. tview.Modal centers text and
	// re-wraps on its own box width, so pre-wrapping mid-word (or padding
	// lines wider than the box) is what split "arkaplanda" and the help
	// rows before. Here: wrap at spaces, keep one line per entry.
	w, h := a.modalFitSize()
	_ = w
	// NO pre-wrap: tview.Modal centers the text and WordWraps it to its
	// own box (word boundaries + CJK-aware widths). Every pre-wrap we
	// tried (screen width, button width, widest line) disagreed with the
	// box by a few cells and the box re-wrapped mid-word ("arka/planda").
	// The box width derives from the longest line, so unwrapped text
	// sizes the box to fit and no second wrap can split a word.
	if h > 0 {
		body = fitModalBody(body, hint, len(buttons), h)
	}
	// Bracket-style buttons with the safe default marked: the owner asked
	// for "[ onayla ]   [ vazgeç ]" with the default visually distinct.
	// tview.Button renders its label verbatim; focus (first button gets
	// it via SetFocus(0) below) drives reverse video, and the "►" prefix
	// marks the default even with colours off.
	br := make([]string, len(buttons))
	for i, b := range buttons {
		if i == len(buttons)-1 {
			br[i] = "► [ " + b + " ]"
		} else {
			br[i] = "[ " + b + " ]"
		}
	}
	m := tview.NewModal().
		SetText(body + "\n\n" + hint).
		AddButtons(br).
		SetDoneFunc(func(i int, label string) {
			if onClose != nil {
				onClose()
			}
			if i == 0 && onOK != nil {
				onOK()
			}
			a.closeModalSync()
		})
	m.SetTitle(" " + title + " ")
	m.SetBorder(true)
	m.SetFocus(len(br) - 1)
	if !a.pages.HasPage("modal") {
		a.pages.AddPage("modal", m, true, true)
	} else {
		a.pages.ShowPage("modal")
	}
	a.app.SetFocus(m)
}
// modalFitSize returns the usable screen size for a modal, or 0s when
// unknown (production fills it in on the first draw).
func (a *tuiApp) modalFitSize() (int, int) {
	return a.lastW, a.lastH
}

// fitModalBody keeps the first lines that fit in height (reserving 6 rows
// for modal chrome: borders, buttons, hint) and replaces the cut tail with
// a "+N more — see full list" marker line.
func fitModalBody(body, hint string, nButtons, height int) string {
	lines := strings.Split(body, "\n")
	hintLines := 0
	if hint != "" {
		hintLines = len(strings.Split(hint, "\n"))
	}
	budget := height - 6 - 2 - hintLines
	if budget < 4 {
		budget = 4
	}
	_ = nButtons
	if len(lines) <= budget {
		return strings.Join(lines, "\n")
	}
	kept := lines[:budget-1]
	marker := fmt.Sprintf("… +%d more", len(lines)-(budget-1))
	return strings.Join(append(kept, marker), "\n")
}

// wrapModalWords wraps every line at word boundaries to fit max runes:
// break between words, never inside a word. A single overlong word is kept
// whole (the modal centers it; slicing "arkaplanda" is worse than overflow).
func wrapModalWords(s string, max int) string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		out = append(out, wrapLineWords(ln, max)...)
	}
	return strings.Join(out, "\n")
}

// wrapLineWords breaks one line at spaces only; a single overlong word is
// kept whole (the modal centers, it never slices a word).
func wrapLineWords(ln string, max int) []string {
	if len([]rune(ln)) <= max {
		return []string{ln}
	}
	words := strings.Fields(ln)
	if len(words) <= 1 {
		return []string{ln}
	}
	var out []string
	cur := words[0]
	for _, w := range words[1:] {
		if len([]rune(cur+" "+w)) > max {
			out = append(out, cur)
			cur = w
			continue
		}
		cur += " " + w
	}
	return append(out, cur)
}

func (a *tuiApp) confirmAction(act int) {
	// Label comes from st.rows (the dispatched slice), never a parallel
	// table: the modal names exactly what Enter is about to run.
	lbl := ""
	for _, r := range a.st.rows {
		if r.id == act {
			lbl = r.text
			break
		}
	}
	// The modal names the action and the safe default (Esc). No letter
	// keycap anywhere (owner decision): "Durdur?" not "[x] Durdur?".
	// Single-line action row: the modal box never wraps it mid-phrase.
	body := fmt.Sprintf("%s?", lbl)
	if act == tuiActQuit {
		// Quit dialog: panel/tray keep running; safe default is
		// cancel — Enter lands on cancel, Tab+Enter is needed to quit.
		// Hint INSIDE the body (not the modal hint row): the button row
		// clips to the longest button and the separate hint row re-wraps
		// against a narrower box ("arka/planda"). Body text renders full.
		qbody := tuiTr("TuiQuitMsg", a.t, "Close the TUI? The panel and tray keep running.") + "\n" + tuiTr("TuiConfirmHint", a.t, "Enter confirm · Esc cancel")
		// Pre-split at the sentence boundary: the modal box derives its
		// width from the longest line and WordWraps the rest, so a
		// single 66-rune sentence re-wraps mid-word at box width. Two
		// short lines fit the box whole — a sentence split is a word
		// boundary by definition, never "arka/planda".
		qbody = splitQuitSentences(qbody)
		a.showModal(tuiTr("TuiQuitTitle", a.t, "Quit"), qbody, "", []string{tuiTr("TuiQuitOK", a.t, "quit"), tuiTr("TuiConfirmCancel", a.t, "cancel")}, func() {
			a.mu.Lock()
			a.st.quit = true
			a.mu.Unlock()
			a.app.Stop()
		}, func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			a.st.confirm = 0
			a.st.lastAct = tuiActNone
		})
		return
	}
	// Hint lives INSIDE the body text (not tview's button row): the button
	// row clips to the longest button, but body text always renders full.
	body += "\n" + tuiTr("TuiConfirmHint", a.t, "Enter confirm · Esc cancel")
	a.showModal(tuiTr("TuiConfirmTitle", a.t, "Confirm"), body, "", []string{tuiTr("TuiConfirmOK", a.t, "confirm"), tuiTr("TuiConfirmCancel", a.t, "cancel")}, func() {
		msg := tuiDoAction(act, a.t)
		a.setMsg(msg)
	}, func() {
		// Esc-cancel also lands here (done func i<0): leave nothing that
		// a later Enter could re-fire. handleKeyEvent already zeroed
		// st.confirm when it opened the dialog.
		a.mu.Lock()
		defer a.mu.Unlock()
		a.st.confirm = 0
		a.st.lastAct = tuiActNone
	})
}

// splitQuitSentences pre-splits a two-sentence quit message at the sentence
// boundary so the modal box (sized from the longest line) fits each line
// whole. WordWrap would otherwise re-wrap the 66-rune sentence mid-word.
// Only splits on "? " / ". " / "! " followed by an uppercase letter.
func splitQuitSentences(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, ln := range lines {
		out = append(out, splitOneSentence(ln)...)
	}
	return strings.Join(out, "\n")
}

// splitOneSentence splits one line at sentence ends ("? "/"! " always;
// ". " only when followed by an uppercase letter, so "C:\path\x.exe"
// and version numbers never split).
func splitOneSentence(ln string) []string {
	runes := []rune(ln)
	for i := 0; i+2 < len(runes); i++ {
		if (runes[i] == '?' || runes[i] == '!') && runes[i+1] == ' ' {
			return []string{string(runes[: i+1]), string(runes[i+2:])}
		}
		if runes[i] == '.' && runes[i+1] == ' ' && runes[i+2] >= 'A' && runes[i+2] <= 'Z' {
			return []string{string(runes[: i+1]), string(runes[i+2:])}
		}
	}
	return []string{ln}
}

func (a *tuiApp) setMsg(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setMsgLocked(s)
}

func (a *tuiApp) setMsgLocked(s string) {
	a.msg = s
	a.msgAt = time.Now()
}

func (a *tuiApp) refresh() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.refreshLocked()
}

func (a *tuiApp) refreshLocked() {
	snap := takeTuiSnapshot()
	// LOOP-SAFE: reads the worker-maintained probe cache only. The tick
	// goroutine (tuiProbeWorkerTick) does the dialing; this never blocks.
	probe := tuiCachedProbe()
	if snap.probe == "unknown" && probe == "unreachable" && snap.status != "port_conflict" {
		probe = "unknown"
	}
	a.probe = probe
	a.probeAt = time.Now()
	// Status truth comes from the probe: healthy answers win over any
	// stale port_conflict state. "Port conflict" renders only when the port
	// is busy AND nothing answers the probe.
	snap.probe = probe
	snap.probeLabel = tuiProbeLabel(probe, a.t)
	state, stateNote := tuiDisplayState(snap, probe, a.t)
	// Entries switch on the MACHINE token (running/stopped/...); the
	// display label is localized (Çalışıyor/Durdu/...) and must not
	// overwrite it before syncListsLocked. Status pane uses `state`.
	machine := snap.status
	// Review-dump override (ORPANEL_TUI_STATE): the dump env cannot
	// guarantee a live/dead OmniRoute on demand. Live path unaffected
	// (tuiDumpState empty unless the dump harness sets it).
	tuiDumpStateMu.Lock()
	if tuiDumpState == "running" || tuiDumpState == "stopped" {
		machine = tuiDumpState
		if tuiDumpState == "running" {
			state = tuiTr("HealthBadgeRunning", a.t, "Running")
			snap.probe = "healthy"
		} else {
			state = tuiTr("HealthBadgeStopped", a.t, "Stopped")
			snap.probe = "unreachable"
		}
	}
	tuiDumpStateMu.Unlock()
	clock := time.Now().Format("15:04:05")
	// Header token uses the PINNED language when dumping (ORPANEL_TUI_LANG):
	// snap.lang comes from the saved config (es on this machine) while the
	// panes render a.t — the token must agree with the panes. Theme shows
	// the saved theme (the palette in use); live path unchanged.
	hlang := snap.lang
	if dl := os.Getenv("ORPANEL_TUI_LANG"); dl != "" {
		hlang = dl
	}
	a.header.SetText(headerText(a.lastW, snap.appVer, snap.omniVer, hlang, snap.theme, clock))
	// Fixed 11-cell label column (§1) with badge + tepsi + yönetim rows.
	var sb strings.Builder
	badge := "○"
	if snap.probe == "healthy" {
		badge = "●"
	}
	fmt.Fprintf(&sb, "%-11s %s %s\n", tuiTr("TuiStatus", a.t, "Status"), badge, state)
	fmt.Fprintf(&sb, "%-11s %s\n", tuiTr("HealthLabelVersion", a.t, "Version"), snap.omniVer)
	fmt.Fprintf(&sb, "%-11s %s\n", tuiTr("HealthLabelPort", a.t, "Port"), snap.port)
	fmt.Fprintf(&sb, "%-11s %s\n", tuiTr("HealthLabelNode", a.t, "Node"), snap.nodeVer)
	trayOn := tuiTr("TuiOff", a.t, "Off")
	if isAutoStartEnabled() {
		trayOn = tuiTr("TuiOn", a.t, "On")
	}
	fmt.Fprintf(&sb, "%-11s %s\n", tuiTr("TuiTray", a.t, "Tray"), trayOn)
	mgmtVal := tuiTr("TuiManagedLocal", a.t, "this process")
	if tuiCachedPanelServing() {
		mgmtVal = tuiTr("TuiManagedPanel", a.t, "panel (:20127)")
	}
	fmt.Fprintf(&sb, "%-11s %s\n", tuiTr("TuiMgmt", a.t, "managed"), mgmtVal)
	if snap.external && (snap.probe == "healthy" || snap.probe == "starting") {
		fmt.Fprintf(&sb, "%s\n", tuiTr("TuiExternalNote", a.t, "Externally managed"))
	}
	if stateNote != "" {
		fmt.Fprintf(&sb, "%s\n", stateNote)
	}
	if snap.opPhase != "" {
		fmt.Fprintf(&sb, "%s\n", snap.opPhase)
	}
	a.status.SetText(strings.TrimRight(sb.String(), "\n"))
	snap.status = machine
	a.syncListsLocked(snap)
	a.renderLogsLocked(snap)
	fw := a.lastW
	if fw <= 0 {
		fw = 80
	}
	foot := a.footerTextLocked(fw - 2)
	if a.msg != "" {
		if time.Since(a.msgAt) < 8*time.Second {
			foot = a.msg
		} else {
			a.msg = ""
		}
	}
	a.footer.SetText(foot)
	a.snap = snap
}
// renderLogsLocked paints the log pane. Client mode polls
// GET /api/logs?last=N incrementally (last = served newIndex); direct mode
// uses the in-process buffer only. Never an empty box: loading/empty lines.
func (a *tuiApp) renderLogsLocked(snap tuiSnapshot) {
	a.logs.Clear()
	if tuiCachedPanelServing() {
		title := " " + tuiTr("TuiLogs", a.t, "Logs") + " (panel) "
		lines, cursor, ok := tuiPollPanelLogs(a.logCursor)
		if !ok {
			if !a.logReady {
				a.logs.SetTitle(title)
				fmt.Fprintln(a.logs, tuiTr("TuiLogLoading", a.t, "loading…"))
				return
			}
			lines = nil
		} else {
			a.logCursor, a.logReady = cursor, true
		}
		a.logs.SetTitle(title)
		if len(lines) == 0 {
			fmt.Fprintln(a.logs, tuiTr("TuiLogEmpty", a.t, "no logs — panel just started"))
			return
		}
		a.writeLogLinesLocked(lines)
		return
	}
	a.logReady = false
	a.logs.SetTitle(" " + tuiTr("TuiLogs", a.t, "Logs") + " ")
	logMutex.Lock()
	buf := append([]string(nil), logBuffer...)
	logMutex.Unlock()
	if len(buf) > 300 {
		buf = buf[len(buf)-300:]
	}
	a.writeLogLinesLocked(tuiLogWindow(buf, 1<<20, a.st.logOff))
}

// writeLogLinesLocked colours + writes lines, then follows the tail.
func (a *tuiApp) writeLogLinesLocked(lines []string) {
	for _, ln := range lines {
		lvl := tuiLevel(ln)
		if tuiNoColor() {
			fmt.Fprintln(a.logs, ln)
			continue
		}
		switch lvl {
		case "err":
			fmt.Fprintf(a.logs, "[red]%s[-]\n", tview.Escape(ln))
		case "warn":
			fmt.Fprintf(a.logs, "[yellow]%s[-]\n", tview.Escape(ln))
		default:
			fmt.Fprintf(a.logs, "%s\n", tview.Escape(ln))
		}
	}
	if a.st.logOff <= 0 && a.follow {
		a.logs.ScrollToEnd()
	}
}
// tuiDisplayState derives the shown state from the on-demand probe.
// Healthy probe wins: running (+ external note). Port conflict only when
// the port is busy and nothing answers. Starting only while our child is
// launching; otherwise unreachable reads as stopped.
func tuiDisplayState(snap tuiSnapshot, probe string, t map[string]string) (string, string) {
	switch probe {
	case "healthy":
		return tuiTr("HealthBadgeRunning", t, "Running"), ""
	case "starting":
		return tuiTr("ProbeStarting", t, "Starting"), ""
	case "degraded":
		return tuiTr("ProbeDegraded", t, "Not responding"), ""
	case "unreachable":
		if snap.status == "port_conflict" || snap.status == tuiTr("HealthBadgePortConflict", t, "Port conflict") {
			return tuiTr("HealthBadgePortConflict", t, "Port conflict"), ""
		}
		return tuiTr("HealthBadgeStopped", t, "Stopped"), ""
	default:
		return tuiTr("ProbeUnknown", t, "Checking"), ""
	}
}


// tuiThemeIsSystem reports whether cfg.Theme selects terminal defaults.
func tuiThemeIsSystem() bool {
	cfg := loadConfig()
	return cfg.Theme == ThemeSystem || cfg.Theme == ""
}

// applyThemeLocked paints our own palette: dark/light get explicit
// background/foreground/border/selection/level colours; system uses the
// terminal defaults with NO brand colours; NO_COLOR wins over everything
// while keeping markers/borders distinguishable (reverse stays, colours go).
func (a *tuiApp) applyThemeLocked() {
	if tuiNoColor() {
		for _, l := range []*tview.List{a.listTop, a.listBak, a.listAya} {
			if l == nil {
				continue
			}
			l.SetSelectedStyle(tcell.StyleDefault.Reverse(true))
			l.SetMainTextColor(tcell.ColorDefault)
		}
		return
	}
	cfg := loadConfig()
	switch cfg.Theme {
	case ThemeLight:
		sel := tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite).Reverse(true)
		for _, l := range []*tview.List{a.listTop, a.listBak, a.listAya} {
			if l == nil {
				continue
			}
			l.SetSelectedStyle(sel)
			l.SetMainTextColor(tcell.ColorBlack)
		}
		a.logs.SetTextColor(tcell.ColorBlack)
	case ThemeDark:
		sel := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack).Reverse(true)
		for _, l := range []*tview.List{a.listTop, a.listBak, a.listAya} {
			if l == nil {
				continue
			}
			l.SetSelectedStyle(sel)
			l.SetMainTextColor(tcell.ColorWhite)
		}
		a.logs.SetTextColor(tcell.ColorWhite)
	default: // system: terminal defaults, no brand colours at all
		for _, l := range []*tview.List{a.listTop, a.listBak, a.listAya} {
			if l == nil {
				continue
			}
			l.SetSelectedStyle(tcell.StyleDefault.Reverse(true))
			l.SetMainTextColor(tcell.ColorDefault)
		}
		a.logs.SetTextColor(tcell.ColorDefault)
	}
}

// tuiSelectionColors resolves the List selection style for one theme:
// the observable contract behind applyThemeLocked (frame dumps carry no
// colour, so tests pin through here): dark/light resolve differently,
// system resolves to the terminal default (no brand colours).
func tuiSelectionColors(theme string) tcell.Color {
	switch theme {
	case ThemeLight:
		return tcell.ColorBlack
	case ThemeDark:
		return tcell.ColorWhite
	default:
		return tcell.ColorDefault
	}
}

// applyTheme is the locked wrapper for non-loop callers.
func (a *tuiApp) applyTheme() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applyThemeLocked()
}

// tuiThemeMessage reports what the last theme press changed (TUI palette +
// web UI theme are saved together; the TUI keeps the terminal palette only
// in system mode).
func tuiThemeMessage(next string, t map[string]string) string {
	return tuiTr("TuiThemeMsg", t, "web UI theme") + ": " + next + " (" + tuiTr("TuiThemeNote", t, "TUI uses the terminal palette") + ")"
}

// syncListsLocked rebuilds every section List from tuiEntriesFor: each
// List shows exactly its own slice, Enter/click dispatch entries[i].id,
// help/overflow read the same slice. st.rows always mirrors the VISIBLE
// list so legacy handleKey/confirm/help paths index the same data.
// Caller holds a.mu.
func (a *tuiApp) syncListsLocked(snap tuiSnapshot) {
	a.st.rows = tuiEntriesFor(a.st.sec, a.t, snap)
	if a.st.sel >= len(a.st.rows) {
		a.st.sel = len(a.st.rows) - 1
	}
	if a.st.sel < 0 {
		a.st.sel = 0
	}
	for _, sec := range []tuiSection{tuiSecTop, tuiSecBakim, tuiSecAyarlar} {
		entries := tuiEntriesFor(sec, a.t, snap)
		l := a.listForSectionLocked(sec)
		if l == nil {
			continue
		}
		l.Clear()
		for i, e := range entries {
			lbl := e.text
			if tuiConfirmNeeded(e.id) {
				lbl += " *"
			}
			// Hover is OUR marker (tview has none): a trailing " ›" on
			// the hovered row of the visible List, so the " *" confirm
			// column the help grid pins stays byte-identical between
			// List items and helpLines (selection is reverse video).
			if sec == a.st.sec && i == a.hover && a.hoverList == l {
				lbl += " ›"
			}
			l.AddItem(lbl, "", 0, nil)
		}
	}
	a.retitleListsLocked()
	if vis := a.visibleListLocked(); vis != nil {
		vis.SetCurrentItem(a.st.sel)
	}
}

// listForSectionLocked returns the List owned by sec.
func (a *tuiApp) listForSectionLocked(sec tuiSection) *tview.List {
	switch sec {
	case tuiSecBakim:
		return a.listBak
	case tuiSecAyarlar:
		return a.listAya
	default:
		return a.listTop
	}
}

// retitleListsLocked names each List with its section + count.
func (a *tuiApp) retitleListsLocked() {
	base := tuiTr("TuiActions", a.t, "Actions")
	if a.listTop != nil {
		a.listTop.SetTitle(" " + base + " ")
	}
	bak, aya := tuiTr("TuiBakim", a.t, "Bakım"), tuiTr("TuiAyarlar", a.t, "Ayarlar")
	if a.listBak != nil {
		a.listBak.SetTitle(" " + bak + " ")
	}
	if a.listAya != nil {
		a.listAya.SetTitle(" " + aya + " ")
	}
}

// visibleListLocked returns the List for the current section.
func (a *tuiApp) visibleListLocked() *tview.List {
	switch a.st.sec {
	case tuiSecBakim:
		return a.listBak
	case tuiSecAyarlar:
		return a.listAya
	default:
		return a.listTop
	}
}

// activateListIndex dispatches List index i: sections open, confirms gate,
// plain actions run. Called by List.SetSelectedFunc (Enter AND click share
// the path, so render/help/dispatch can never diverge).
func (a *tuiApp) activateListIndex(index int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if index < 0 || index >= len(a.st.rows) {
		return
	}
	id := a.st.rows[index].id
	a.st.sel = index
	if tuiEntryIsSection(id) {
		a.st.sec = tuiSectionFor(id)
		a.st.sel = 0
		a.syncListsLocked(a.snap)
		a.refreshLocked()
		a.applyFocusLocked()
		return
	}
	if tuiConfirmNeeded(id) || id == tuiActQuit {
		a.st.confirm = id
		a.confirmAction(id)
		a.st.confirm = 0
		a.refreshLocked()
		a.applyFocusLocked()
		a.st.lastAct = tuiActNone
		return
	}
	a.st.lastAct = id
	a.refreshLocked()
	a.applyFocusLocked()
}


// mainLayout builds the stable root once: header + body + actions + footer.
// The body holds status/logs; the visible action List sits below it as its
// own focusable pane (double border + ► when focused). No TextView bar.
func (a *tuiApp) mainLayout(width int) *tview.Flex {
	a.body = tview.NewFlex()
	a.applyBodyClass(width)
	a.header.SetBorder(true)
	a.status.SetBorder(true)
	a.logs.SetBorder(true)
	a.footer.SetBorder(false)
	a.syncActionPaneLocked()
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header, 3, 0, false).
		AddItem(a.body, 0, 1, false).
		AddItem(a.actionPane(), 0, 1, false).
		AddItem(a.footer, 1, 0, false)
	return root
}

// actionPane returns the visible section List wrapped for layout. The List
// itself carries the border + title so focus, hover and selection all live
// in one primitive.
func (a *tuiApp) actionPane() tview.Primitive {
	return a.visibleListLocked()
}

// syncActionPaneLocked re-parents the visible List after a section switch.
// tview.Flex has no replace call: remove by identity, then add.
func (a *tuiApp) syncActionPaneLocked() {
	root := a.pages.GetPage("main")
	flex, ok := root.(*tview.Flex)
	if !ok || flex == nil {
		return
	}
	for _, l := range []*tview.List{a.listTop, a.listBak, a.listAya} {
		if l != nil {
			flex.RemoveItem(l)
		}
	}
	vis := a.visibleListLocked()
	flex.AddItem(vis, 0, 1, a.st.pane == tuiPaneActions)
	a.applyFocusLocked()
}

func (a *tuiApp) applyFocusLocked() {
	// Focus differs by border STYLE (double vs single, drawn by tview from
	// pane focus) AND by color AND by the ► marker in the title — so it
	// survives light palettes, collapsed colors, and NO_COLOR alike.
	focused := tcell.AttrBold
	if tuiNoColor() {
		focused = tcell.AttrNone
	}
	if a.st.pane == tuiPaneActions {
		if vis := a.visibleListLocked(); vis != nil {
			a.app.SetFocus(vis)
		} else {
			a.status.Focus(nil)
		}
		a.logs.Blur()
		a.status.SetBorderAttributes(focused)
		a.logs.SetBorderAttributes(tcell.AttrNone)
		a.status.SetBorderColor(tcell.ColorDefault)
		a.logs.SetBorderColor(tcell.ColorDefault)
		a.status.SetTitle(" \u25ba " + tuiTr("TuiStatus", a.t, "Status") + " ")
		a.logs.SetTitle(" " + tuiTr("TuiLogs", a.t, "Logs") + " ")
		a.setListTitlesLocked(" \u25ba " + tuiTr("TuiActions", a.t, "Actions") + " ")
	} else {
		a.status.Blur()
		a.logs.Focus(nil)
		a.status.SetBorderAttributes(tcell.AttrNone)
		a.logs.SetBorderAttributes(focused)
		a.status.SetBorderColor(tcell.ColorDefault)
		a.logs.SetBorderColor(tcell.ColorDefault)
		a.status.SetTitle(" " + tuiTr("TuiStatus", a.t, "Status") + " ")
		a.logs.SetTitle(" \u25ba " + tuiTr("TuiLogs", a.t, "Logs") + " ")
		a.setListTitlesLocked(" " + tuiTr("TuiActions", a.t, "Actions") + " ")
	}
	a.status.SetBorder(true)
	a.logs.SetBorder(true)
}

// setListTitlesLocked retitles every section List (only one is visible).
func (a *tuiApp) setListTitlesLocked(title string) {
	for _, l := range []*tview.List{a.listTop, a.listBak, a.listAya} {
		if l != nil {
			l.SetTitle(title)
			l.SetBorder(true)
		}
	}
}

// applyBodyClass switches stacked/two-column ONLY on class flips and keeps
// selection + focus from app state (sel/pane live in a.st, never in Flex).
func (a *tuiApp) applyBodyClass(width int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	wide := width >= 100
	if a.body == nil {
		a.body = tview.NewFlex()
	}
	if wide == a.wide && a.body.GetItemCount() == 2 {
		return
	}
	a.wide = wide
	a.body.Clear()
	if wide {
		a.body.SetDirection(tview.FlexColumn).
			AddItem(a.status, 0, 1, false).
			AddItem(a.logs, 0, 2, false)
	} else {
		a.body.SetDirection(tview.FlexRow).
			AddItem(a.status, 9, 0, false).
			AddItem(a.logs, 0, 1, false)
	}
	a.applyFocusLocked()
}

// applyFocus marks pane borders + titles from a.st without re-rooting.
func (a *tuiApp) applyFocus() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applyFocusLocked()
}

// runTuiApp is the tview event loop: tcell decodes all input, redraws happen
// on every key AND a 1s tick, action results show immediately. The tree is
// built once; per-draw only content refreshes — SetRoot is never called
// again (SetRoot clears the screen and re-focuses the first child, which
// used to snap the selection back on every tick).
func runTuiApp() {
	tuiDiagLog("runTuiApp entry")
	tuiDiagConsoleState("entry")
	var restoreCP func()
	if os.Getenv("ORPANEL_TUI_NO_RAW") == "1" {
		tuiDiagLog("NO_RAW hatch: skipping code-page switch")
		restoreCP = func() {}
	} else {
		restoreCP = tuiCodePageSwitch()
	}
	defer restoreCP()
	tuiDiagConsoleState("after-own-setup")
	tuiDiagLog("newTuiApp start")
	a := newTuiApp()
	// Mouse comes from the library: one call requests the terminal mouse
	// protocol; List/TextView/Modal handlers do the rest (§3). OUR hover
	// is separate (SetMouseCapture below): motion → › marker repaint.
	a.app.EnableMouse(true)
	a.setupMouseCapture()
	tuiDiagLog("newTuiApp done (root set at construction)")
	a.refresh()
	tuiDiagLog("refresh done")
	a.applyFocus()
	tuiDiagLog("applyFocus done")
	if w, h, err := tuiConsoleSize(); err == nil {
		tuiDiagLog("pre-Run console size: %dx%d", w, h)
	} else {
		tuiDiagLog("pre-Run console size err: %v", err)
	}
	getSize := func() (int, int) {
		if w, h, err := tuiConsoleSize(); err == nil && w > 0 && h > 0 {
			return w, h
		}
		return 80, 24
	}
	var afterDraws int64
	a.app.SetAfterDrawFunc(func(screen tcell.Screen) {
		n := atomic.AddInt64(&afterDraws, 1)
		w, h := screen.Size()
		tuiDiagLog("afterDraw #%d size=%dx%d", n, w, h)
		if w > 0 && h > 0 {
			a.mu.Lock()
			a.lastW, a.lastH = w, h
			a.mu.Unlock()
			a.applyBodyClass(w)
		}
	tuiDiagLog("afterDraw #%d exit", n)
	if n == 1 {
		go func() {
			time.Sleep(300 * time.Millisecond)
			// Bar row sits 2 rows above the bottom (bar + footer rows);
			// log row is the first body row below the header (row 4).
			// Action pane: rows above the bar (hh-4, hh-5, hh-6 hold the
			// last entries); status pane: rows 4-9 of the left column.
			// Env-gated via tuiDiagReadCells (inert when unset).
			_, hh := screen.Size()
			tuiDiagLog("first-draw cells row0=%s title=%s border=%s bar=%s logrow=%s",
				tuiDiagReadCells(0, 0, 20), tuiDiagReadCells(2, 0, 30), tuiDiagReadCells(0, 3, 20),
				tuiDiagReadCells(0, hh-3, 60), tuiDiagReadCells(0, 4, 60))
			tuiDiagLog("first-draw actions act0=%s act1=%s act2=%s act3=%s",
				tuiDiagReadCells(0, hh-7, 60), tuiDiagReadCells(0, hh-6, 60),
				tuiDiagReadCells(0, hh-5, 60), tuiDiagReadCells(0, hh-4, 60))
			tuiDiagLog("first-draw status srow0=%s srow1=%s srow2=%s srow3=%s srow4=%s srow5=%s",
				tuiDiagReadCells(0, 4, 40), tuiDiagReadCells(0, 5, 40), tuiDiagReadCells(0, 6, 40),
				tuiDiagReadCells(0, 7, 40), tuiDiagReadCells(0, 8, 40), tuiDiagReadCells(0, 9, 40))
			tuiDiagConsoleState("after-first-draw")
		}()
	}
	})
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()
	// Probes run on the TICK goroutine, never the event loop: dial first
	// (may block ~1.5s during the omni boot window), then queue the
	// read-only content refresh onto the loop via QueueUpdateDraw.
	go func() {
		// Warm-up off the startup path: first paint must never wait on net.
		tuiDiagLog("warm-up probe start")
		tuiProbeWorkerTick()
		tuiDiagLog("warm-up probe end")
		a.app.QueueUpdateDraw(func() {
			tuiDiagLog("first-paint queued refresh")
			w, _ := getSize()
			a.applyBodyClass(w)
			a.refresh()
			a.applyFocus()
		})
		for range tick.C {
			tuiProbeWorkerTick()
			a.app.QueueUpdateDraw(func() {
				w, _ := getSize()
				a.applyBodyClass(w)
				a.refresh()
				a.applyFocus()
			})
		}
	}()
	// Watchdog: if no draw in 5s, dump stacks (cheap fallback instrument).
	go func() {
		time.Sleep(5 * time.Second)
		n := atomic.LoadInt64(&afterDraws)
		tuiDiagLog("watchdog: afterDraws=%d", n)
		tuiDiagLog("watchdog cells row0=%s bar=%s", tuiDiagReadCells(0, 0, 20), tuiDiagReadCells(0, 21, 60))
		if n == 0 {
			tuiDiagDumpStacks("no-afterDraw-in-5s")
		}
	}()
	// Input capture runs ON the event-loop goroutine (application.go: the
	// loop calls `event = inputCapture(event)` directly). Application.Draw
	// is QueueUpdate (blocking on the same loop), so ANY Draw call here
	// deadlocks on the first keypress. Never call Draw from this path:
	// returning nil makes the loop call a.draw() itself; returning ev
	// forwards to the root primitive then draws. QueueUpdateDraw is only
	// safe from other goroutines (the 1s tick below).
	a.setupInputCapture()
	tuiDiagLog("Run() entry")
	tuiDiagConsoleState("pre-Run")
	if err := a.app.Run(); err != nil {
		tuiDiagLog("Run() err: %v", err)
		printPlainSummary()
	}
	tuiDiagLog("Run() returned")
}

// setupInputCapture installs the single key path (capture runs ON the event
// loop; it must never block on Draw — returning nil draws via a.draw()).
func (a *tuiApp) setupInputCapture() {
	a.app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		// Ctrl+C is the escape hatch: forward unchanged so tview's own
		// branch stops the app (deferred tcell Fini + code-page restore
		// run). Never swallow it — not even with a modal up.
		if ev.Key() == tcell.KeyCtrlC {
			return ev
		}
		// Modal up: Enter/Esc/Tab belong to tview.Modal (buttons/focus);
		// swallow every other key so no global shortcut fires behind it.
		if name, _ := a.pages.GetFrontPage(); name == "modal" {
			switch ev.Key() {
			case tcell.KeyEnter, tcell.KeyEscape, tcell.KeyTab, tcell.KeyBacktab:
				return ev
			default:
				return nil
			}
		}
		out := a.handleKeyEvent(ev)
		a.mu.Lock()
		quit := a.st.quit
		a.mu.Unlock()
		if quit {
			a.app.Stop()
			return nil
		}
		return out
	})
}

// setupMouseCapture installs OUR hover: application-level motion handling.
// The library has NO hover (List.MouseHandler answers clicks only), so we
// inspect *tcell.EventMouse motion (button none, position changed), map the
// cursor row to the visible List entry, and repaint only that row's ›
// marker; selection stays the library's. No motion events → no hover,
// clicks/wheel/keyboard unaffected. Caller: runTuiApp (not tests directly).
func (a *tuiApp) setupMouseCapture() {
	a.app.SetMouseCapture(func(ev *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		if action != tview.MouseMove {
			return ev, action
		}
		a.handleMouseMove(ev)
		return ev, action
	})
}

// handleMouseMove maps a motion event to a visible-List row. Pure position
// math under a.mu; safe to call from tests with synthetic events.
func (a *tuiApp) handleMouseMove(ev *tcell.EventMouse) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ev == nil || ev.Buttons() != tcell.ButtonNone {
		return
	}
	l := a.visibleListLocked()
	if l == nil {
		a.hover, a.hoverList = -1, nil
		return
	}
	x, y := ev.Position()
	idx := listIndexAt(l, x, y)
	if idx < 0 {
		if a.hover != -1 {
			a.hover, a.hoverList = -1, nil
			a.syncListsLocked(a.snap)
		}
		return
	}
	if idx == a.hover && a.hoverList == l {
		return
	}
	a.hover, a.hoverList = idx, l
	a.syncListsLocked(a.snap)
}

// listIndexAt maps screen coords to a List row via its inner rect.
func listIndexAt(l *tview.List, x, y int) int {
	if l == nil {
		return -1
	}
	rx, ry, _, h := l.GetInnerRect()
	i := y - ry
	_ = rx
	if i < 0 || i >= h {
		return -1
	}
	if i >= l.GetItemCount() {
		return -1
	}
	return i
}

// so titles, chips, footer and help switch language immediately (no restart).
// Caller holds a.mu.
func (a *tuiApp) applyLanguageLocked(next string) {
	a.t = loadTranslations(next)
	a.st.rows = tuiActionRows(a.t)
	if a.st.sel >= len(a.st.rows) {
		a.st.sel = len(a.st.rows) - 1
	}
	if a.st.sel < 0 {
		a.st.sel = 0
	}
	a.status.SetTitle(" " + tuiTr("TuiStatus", a.t, "Status") + " ")
	a.logs.SetTitle(" " + tuiTr("TuiLogs", a.t, "Logs") + " ")
	a.setListTitlesLocked(" " + tuiTr("TuiActions", a.t, "Actions") + " ")
	a.refreshLocked()
	a.applyFocusLocked()
}
// modalInputHandler returns the open modal's input handler, or nil.
func (a *tuiApp) modalInputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		return nil
	}
	p := a.pages.GetPage("modal")
	if p == nil {
		return nil
	}
	return p.InputHandler()
}

// handleKeyEvent is the single key path for live AND scripted input (no mode
// flag: both call this exact function). It updates primitives only; the
// event loop performs the draw after return. lastAct is consumed here and
// cleared so a later key or tick can never re-fire it.
func (a *tuiApp) handleKeyEvent(ev *tcell.EventKey) *tcell.EventKey {
	k := tuiEventKey(ev)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.st = handleKey(a.st, k)
	if vis := a.visibleListLocked(); vis != nil && a.st.sel >= 0 && a.st.sel < vis.GetItemCount() && vis.GetCurrentItem() != a.st.sel {
		vis.SetCurrentItem(a.st.sel)
	}
	if a.st.quit {
		return nil
	}
	if k.r == '?' {
		if a.st.showHelp {
			a.st.showHelp = true
			a.showModal(tuiTr("TuiHelpTitle", a.t, "Keys"), a.helpBody(), "Esc "+tuiTr("TuiClose", a.t, "Close"), []string{tuiTr("TuiClose", a.t, "Close")}, nil, func() {
				a.mu.Lock()
				defer a.mu.Unlock()
				a.st.showHelp = false
			})
		} else {
			a.pages.RemovePage("modal")
		}
		a.refreshLocked()
		a.applyFocusLocked()
		return nil
	}
	if a.st.confirm != 0 {
		act := a.st.confirm
		if a.st.lastAct == tuiActConfirmCancel {
			a.st.confirm = 0
			a.pages.RemovePage("modal")
			a.refreshLocked()
			a.applyFocusLocked()
			a.st.lastAct = tuiActNone
			return nil
		}
		if tuiEntryIsSection(act) {
			a.st.sec = tuiSectionFor(act)
			a.st.sel = 0
			a.st.confirm = 0
			a.syncActionPaneLocked()
			a.refreshLocked()
			a.applyFocusLocked()
			a.st.lastAct = tuiActNone
			return nil
		}
		a.confirmAction(act)
		a.st.confirm = 0
		a.refreshLocked()
		a.applyFocusLocked()
		a.st.lastAct = tuiActNone
		return nil
	}
	if a.st.lastAct != tuiActNone && a.st.lastAct != tuiActHelp && a.st.lastAct != tuiActPaneNext &&
		a.st.lastAct != tuiActScrollUp && a.st.lastAct != tuiActScrollDown {
		act := a.st.lastAct
		a.st.lastAct = tuiActNone
		a.mu.Unlock()
		msg := tuiDoAction(act, a.t)
		a.mu.Lock()
		a.setMsgLocked(msg)
		// Language switch takes effect immediately: reload map + rows.
		if act == tuiActLanguage {
			if cfg := loadConfig(); cfg.Language != "" {
				a.applyLanguageLocked(cfg.Language)
			}
		}
		// Theme switch repaints our palette immediately (system = terminal
		// defaults, NO_COLOR wins inside applyThemeLocked).
		if act == tuiActTheme {
			a.applyThemeLocked()
		}
	} else {
		a.st.lastAct = tuiActNone
	}
	// Rebuild the visible List from the new state: top-level set depends on
	// running/stopped (snapshot), and theme/language change the labels.
	a.syncListsLocked(a.snap)
	a.refreshLocked()
	// refreshLocked rebuilds rows but never swaps the shown List: each
	// section owns a DIFFERENT List (no header rows by design), so a
	// section change must re-parent the visible one here. simFrame's
	// refresh does the same via this path (section keys flow through
	// handleKeyEvent), keeping dumps and live converged.
	a.syncActionPaneLocked()
	a.applyFocusLocked()
	return nil
}
// tuiDebugOn reports whether console-mode debug logging is enabled.
func tuiDebugOn() bool {
	return os.Getenv("ORPANEL_TUI_DEBUG") == "1"
}

// tuiDebugLog writes console-mode lines only when ORPANEL_TUI_DEBUG=1.
func tuiDebugLog(format string, args ...interface{}) {
	if tuiDebugOn() {
		writeLog(format, args...)
	}
}
