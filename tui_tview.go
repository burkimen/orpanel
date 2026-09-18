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
	bar       *tview.TextView
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
	a := &tuiApp{app: tview.NewApplication(), t: loadTranslations(getCurrentLang()), follow: true}
	a.status = tview.NewTextView().SetDynamicColors(!tuiNoColor()).SetScrollable(false)
	a.status.SetWrap(false)
	a.logs = tview.NewTextView().SetDynamicColors(!tuiNoColor()).SetScrollable(true)
	a.logs.SetWrap(false)
	a.bar = tview.NewTextView().SetDynamicColors(!tuiNoColor())
	a.bar.SetWrap(false)
	a.header = tview.NewTextView().SetDynamicColors(!tuiNoColor())
	a.header.SetWrap(false)
	a.footer = tview.NewTextView().SetDynamicColors(!tuiNoColor())
	a.footer.SetWrap(false)
	for _, v := range []*tview.TextView{a.status, a.logs, a.bar, a.header, a.footer} {
		v.SetBorder(false)
	}
	a.status.SetTitle(" " + tuiTr("TuiStatus", a.t, "Status") + " ")
	a.logs.SetTitle(" " + tuiTr("TuiLogs", a.t, "Logs") + " ")
	a.bar.SetTitle(" " + tuiTr("TuiActions", a.t, "Actions") + " ")
	a.st = tuiState{pane: tuiPaneActions, rows: tuiActionRows(a.t)}
	a.body = tview.NewFlex()
	a.pages = tview.NewPages()
	a.pages.AddPage("main", a.mainLayout(80), true, true)
	// The Application holds its own root pointer: without SetRoot every
	// draw() returns early (root == nil) and Show() paints only the
	// cleared screen. Set once here so all paths (prod, sim, tests) draw.
	a.app.SetRoot(a.pages, true)
	a.wide = false
	return a
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


// helpLines returns the help modal as whole lines: one row per action
// ("[key] label", destructive marked with *), navigation hints joined
// into whole pairs. Short enough that the modal (lines+6) fits 80x24.
func (a *tuiApp) helpLines() []string {
	var lines []string
	lines = append(lines, tuiTr("TuiHelpActTitle", a.t, "Actions")+" ("+tuiTr("TuiConfirmLegend", a.t, "* needs confirm")+")")
	// Same slice the bar renders and Enter dispatches: st.rows.
	for _, r := range a.st.rows {
		mark := "  "
		if tuiConfirmNeeded(r.id) {
			mark = " *"
		}
		// One row per action, key cap first: phrases never split.
		lines = append(lines, fmt.Sprintf(" [%s] %s%s", r.key, oneLine(r.text), mark))
	}
	lines = append(lines, "")
	// Navigation: two whole-pair rows (select/activate, pane/cancel) plus
	// one short hints row — keys stay discoverable without overflowing.
	var pairs []string
	for _, bd := range tuiFooterBindings(tuiPaneActions) {
		if bd.scope != tuiScopeGlobal && bd.scope != tuiScopePane {
			continue
		}
		lbl := tuiTr(bd.label, a.t, tuiKeyName(bd))
		pairs = append(pairs, tuiKeyName(bd)+" "+lbl)
	}
	lines = append(lines, wrapPairs(pairs, 40)...)
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
	// Fit first (word-wise wrap + height budget), then pad to the widest
	// surviving line so tview's word-wrap has nothing left to reflow: the
	// box sizes to content instead of clipping it mid-word.
	w, h := a.modalFitSize()
	if w > 0 {
		body = fitModalWidth(body, w-2)
		hint = fitModalHint(hint, modalInnerWidth(body))
	}
	if h > 0 {
		body = fitModalBody(body, hint, len(buttons), h)
	}
	body = padToWidth(body)
	m := tview.NewModal().
		SetText(body + "\n\n" + hint).
		AddButtons(buttons).
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

// fitModalWidth wraps overlong lines word-wise so the modal never exceeds
// width (reserving 10 columns for borders and centering margin). Wrapping
// happens only at spaces, so phrases stay intact.
func fitModalWidth(body string, width int) string {
	max := width - 10
	if max < 16 {
		max = 16
	}
	var out []string
	for _, ln := range strings.Split(body, "\n") {
		out = append(out, wrapLineWords(ln, max)...)
	}
	return strings.Join(out, "\n")
}

// fitModalHint wraps the hint line(s) at the same width so the hint row
// never overflows the modal box mid-phrase.
func fitModalHint(hint string, width int) string {
	max := width - 10
	if max < 16 {
		max = 16
	}
	var out []string
	for _, ln := range strings.Split(hint, "\n") {
		out = append(out, wrapLineWords(ln, max)...)
	}
	return strings.Join(out, "\n")
}

func modalInnerWidth(body string) int {
	w := 0
	for _, ln := range strings.Split(body, "\n") {
		if n := len([]rune(ln)); n > w {
			w = n
		}
	}
	return w
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

// padToWidth pads every line with spaces to the widest rune width so the
// modal sizes to its content instead of wrapping mid-word.
func padToWidth(s string) string {
	lines := strings.Split(s, "\n")
	w := 0
	for _, ln := range lines {
		if n := len([]rune(ln)); n > w {
			w = n
		}
	}
	for i, ln := range lines {
		lines[i] = ln + strings.Repeat(" ", w-len([]rune(ln)))
	}
	return strings.Join(lines, "\n")
}

func (a *tuiApp) confirmAction(act int) {
	// Label comes from st.rows (the dispatched slice), never a parallel
	// table: the modal names exactly what Enter is about to run.
	key, lbl := "?", ""
	for _, r := range a.st.rows {
		if r.id == act {
			key, lbl = r.key, r.text
			break
		}
	}
	// The modal names the action, its key, and the safe default (Esc).
	// Single-line action row: the modal box never wraps it mid-phrase.
	body := fmt.Sprintf("[%s] %s?", key, lbl)
	a.showModal(tuiTr("TuiConfirmTitle", a.t, "Confirm"), body, tuiTr("TuiConfirmHint", a.t, "Enter confirm · Esc cancel"), []string{tuiTr("TuiConfirmOK", a.t, "confirm"), tuiTr("TuiConfirmCancel", a.t, "cancel")}, func() {
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
	snap.status = state
	clock := time.Now().Format("15:04:05")
	a.header.SetText(headerText(a.lastW, snap.appVer, snap.omniVer, snap.lang, snap.theme, clock))
	var sb strings.Builder
	rows := [][2]string{
		{tuiTr("TuiStatus", a.t, "Status"), state},
		{tuiTr("HealthLabelVersion", a.t, "Version"), snap.omniVer},
		{tuiTr("HealthLabelPort", a.t, "Port"), snap.port},
		{tuiTr("HealthLabelNode", a.t, "Node"), snap.nodeVer},
	}
	w := 8
	for _, r := range rows {
		if len([]rune(r[0])) > w {
			w = len([]rune(r[0]))
		}
	}
	for _, r := range rows {
		fmt.Fprintf(&sb, "%-*s  %s\n", w, r[0], r[1])
	}
	if tuiCachedPanelServing() {
		fmt.Fprintf(&sb, "%s\n", tuiTr("TuiManagedPanel", a.t, "managed by panel (:20127)"))
	} else {
		fmt.Fprintf(&sb, "%s\n", tuiTr("TuiManagedLocal", a.t, "managed by this process"))
	}
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
	a.logs.Clear()
	logMutex.Lock()
	buf := append([]string(nil), logBuffer...)
	logMutex.Unlock()
	if len(buf) > 300 {
		buf = buf[len(buf)-300:]
	}
	vis := tuiLogWindow(buf, 1<<20, a.st.logOff)
	for _, ln := range vis {
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
	bw := a.lastW
	if bw <= 0 {
		bw = 80
	}
	a.bar.SetText(a.barTextLocked(bw - 2))
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
		if snap.status == "not_installed" || snap.status == tuiTr("HealthBadgeNotInstalled", t, "Not installed") {
			return tuiTr("HealthBadgeNotInstalled", t, "Not installed"), tuiTr("TuiNotInstalled", t, "")
		}
		return tuiTr("HealthBadgeStopped", t, "Stopped"), ""
	default:
		return tuiTr("ProbeUnknown", t, "Checking"), ""
	}
}

// barText renders the action bar with whole-chip clipping: chips that do not
// fit are replaced by an overflow marker ("… +N"), so no chip is ever sliced
// mid-word and no action is undiscoverable (the help modal lists every key).
// The bar TextView also grows to two rows when needed (see mainLayout).
func (a *tuiApp) barText(width int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.barTextLocked(width)
}

// barChips renders one chip per st.rows entry — the same slice arrow
// navigation and Enter dispatch index into. No parallel list may exist:
// rendering, navigation, and dispatch share st.rows as the single source,
// so a hidden chip can never shift the visible labels relative to the data.
func (a *tuiApp) barChips() []string {
	pane, sel := a.st.pane, a.st.sel
	chips := make([]string, 0, len(a.st.rows))
	for i, r := range a.st.rows {
		c := fmt.Sprintf("[%s] %s", r.key, r.text)
		if pane == tuiPaneActions && i == sel {
			if tuiNoColor() {
				c = ">" + c + "<"
			} else {
				c = "[::r]" + c + "[::-]"
			}
		}
		chips = append(chips, c)
	}
	return chips
}

func (a *tuiApp) barTextLocked(width int) string {
	if width <= 0 {
		width = 80
	}
	chips := a.barChips()
	full := strings.Join(chips, "  ")
	if barWidth(full) <= width {
		return full
	}
	// Drop whole chips from the end until the overflow marker fits.
	for n := len(chips) - 1; n > 1; n-- {
		marker := fmt.Sprintf("… +%d", len(chips)-n)
		kept := strings.Join(chips[:n], "  ")
		if barWidth(kept+"  "+marker) <= width {
			return kept + "  " + marker
		}
	}
	return chips[0]
}

// barWidth counts what the terminal actually shows: our chips contain
// literal "[x]" key caps, which tview's TaggedStringWidth consumes as
// zero-width tags — so measure literally, discounting only the real tags
// we emit ([::r], [::-], [-]).
func barWidth(s string) int {
	for _, tag := range []string{"[::r]", "[::-]", "[-]"} {
		s = strings.ReplaceAll(s, tag, "")
	}
	return len([]rune(s))
}

// mainLayout builds the stable root once: header + body + bar + footer.
// Body direction comes from the width class; the log pane always takes the
// remaining height (proportion 1 vs status fixed rows).
func (a *tuiApp) mainLayout(width int) *tview.Flex {
	a.body = tview.NewFlex()
	a.applyBodyClass(width)
	a.header.SetBorder(true)
	a.status.SetBorder(true)
	a.logs.SetBorder(true)
	a.bar.SetBorder(true)
	a.footer.SetBorder(false)
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header, 3, 0, false).
		AddItem(a.body, 0, 1, false).
		AddItem(a.bar, 3, 0, false).
		AddItem(a.footer, 1, 0, false)
	return root
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
		a.status.Focus(nil)
		a.logs.Blur()
		a.status.SetBorderAttributes(focused)
		a.logs.SetBorderAttributes(tcell.AttrNone)
		a.status.SetBorderColor(tcell.ColorDefault)
		a.logs.SetBorderColor(tcell.ColorDefault)
		a.status.SetTitle(" \u25ba " + tuiTr("TuiStatus", a.t, "Status") + " ")
		a.logs.SetTitle(" " + tuiTr("TuiLogs", a.t, "Logs") + " ")
		a.bar.SetTitle(" \u25ba " + tuiTr("TuiActions", a.t, "Actions") + " ")
	} else {
		a.status.Blur()
		a.logs.Focus(nil)
		a.status.SetBorderAttributes(tcell.AttrNone)
		a.logs.SetBorderAttributes(focused)
		a.status.SetBorderColor(tcell.ColorDefault)
		a.logs.SetBorderColor(tcell.ColorDefault)
		a.status.SetTitle(" " + tuiTr("TuiStatus", a.t, "Status") + " ")
		a.logs.SetTitle(" \u25ba " + tuiTr("TuiLogs", a.t, "Logs") + " ")
	}
	a.status.SetBorder(true)
	a.logs.SetBorder(true)
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
			tuiDiagLog("first-draw cells row0=%s title=%s border=%s",
				tuiDiagReadCells(0, 0, 20), tuiDiagReadCells(2, 0, 30), tuiDiagReadCells(0, 3, 20))
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
		tuiDiagLog("watchdog cells row0=%s", tuiDiagReadCells(0, 0, 20))
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

// applyLanguageLocked reloads the translation map + action rows after `l`
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
	a.bar.SetTitle(" " + tuiTr("TuiActions", a.t, "Actions") + " ")
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
	} else {
		a.st.lastAct = tuiActNone
	}
	a.refreshLocked()
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
