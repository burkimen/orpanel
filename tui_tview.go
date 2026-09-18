package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
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

func (a *tuiApp) footerKeys() string {
	return a.footerText(80)
}

func (a *tuiApp) helpBody() string {
	var b strings.Builder
	b.WriteString(tuiTr("TuiHelpNavTitle", a.t, "Navigation") + "\n")
	b.WriteString(" " + tuiTr("TuiHintScrollLine", a.t, "arrows/j/k select") + "\n")
	b.WriteString(" " + tuiTr("TuiNavHint", a.t, "Tab switch pane") + "\n\n")
	b.WriteString(tuiTr("TuiHelpActTitle", a.t, "Actions") + " (" + tuiTr("TuiConfirmLegend", a.t, "* needs confirm") + ")\n")
	for _, bd := range tuiActionBindings() {
		lbl := tuiTr(bd.label, a.t, string(bd.key))
		if bd.confirm {
			fmt.Fprintf(&b, " %c  %s *\n", bd.key, lbl)
		} else {
			fmt.Fprintf(&b, " %c  %s\n", bd.key, lbl)
		}
	}
	b.WriteString("\n" + tuiTr("TuiHelpNavTitle", a.t, "Navigation") + "\n")
	for _, bd := range tuiFooterBindings(tuiPaneActions) {
		if bd.scope != tuiScopeGlobal && bd.scope != tuiScopePane {
			continue
		}
		lbl := tuiTr(bd.label, a.t, tuiKeyName(bd))
		fmt.Fprintf(&b, " %s  %s\n", tuiKeyName(bd), lbl)
	}
	return strings.TrimRight(b.String(), "\n")
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

// showModal opens a centred modal over the stable root. The main page stays
// mounted; closing removes the overlay page — no SetRoot, so focus and the
// selected action index in a.st survive untouched. Focus moves into the
// modal so Enter/Esc/Tab reach its buttons via the normal tview path.
func (a *tuiApp) showModal(title, body, hint string, buttons []string, onOK func(), onClose func()) {
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

func (a *tuiApp) showHelp() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.showHelpLocked()
}

func (a *tuiApp) showHelpLocked() {
	a.st.showHelp = true
	a.showModal(tuiTr("TuiHelpTitle", a.t, "Keys"), a.helpBody(), "Esc "+tuiTr("TuiClose", a.t, "Close"), []string{tuiTr("TuiClose", a.t, "Close")}, nil, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.st.showHelp = false
	})
}

func (a *tuiApp) confirmAction(act int) {
	b, _ := tuiBindingByAct(act)
	lbl := tuiTr(b.label, a.t, string(b.key))
	a.showModal(tuiTr("TuiConfirmTitle", a.t, "Confirm"), lbl+"?", tuiTr("TuiConfirmHint", a.t, "Enter confirm · Esc cancel"), []string{tuiTr("TuiConfirmOK", a.t, "confirm"), tuiTr("TuiConfirmCancel", a.t, "cancel")}, func() {
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
	a.header.SetText(fmt.Sprintf(" OrPanel v%s  OmniRoute %s  %s/%s  %s", snap.appVer, snap.omniVer, snap.lang, snap.theme, clock))
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
			fmt.Fprintf(a.logs, "[red]%s[white]\n", tview.Escape(ln))
		case "warn":
			fmt.Fprintf(a.logs, "[yellow]%s[white]\n", tview.Escape(ln))
		default:
			fmt.Fprintf(a.logs, "[gray]%s[white]\n", tview.Escape(ln))
		}
	}
	if a.st.logOff <= 0 && a.follow {
		a.logs.ScrollToEnd()
	}
	a.bar.SetText(a.barTextLocked())
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

// barText renders the action bar; chips never wrap mid-chip (joined with
// two spaces, tview wraps only at spaces since Wrap(false) keeps words).
// The selected chip is highlighted; focus on the bar is shown in its title.
func (a *tuiApp) barText(width int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.barTextLocked()
}

func (a *tuiApp) barTextLocked() string {
	pane, sel := a.st.pane, a.st.sel
	var sb strings.Builder
	for i, b := range tuiActionBindings() {
		lbl := tuiTr(b.label, a.t, string(b.key))
		c := fmt.Sprintf("[%c] %s", b.key, lbl)
		if pane == tuiPaneActions && i == sel {
			if tuiNoColor() {
				c = ">" + c + "<"
			} else {
				c = "[black:yellow]" + c + "[white:-]"
			}
		}
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(c)
	}
	return sb.String()
}

// screenSize reads the size from the active screen (simulation or console).
// Falls back to 80x24 only when the screen reports zero.
func (a *tuiApp) screenSize(screen tcell.Screen) (int, int) {
	if screen != nil {
		if w, h := screen.Size(); w > 0 && h > 0 {
			return w, h
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lastW > 0 && a.lastH > 0 {
		return a.lastW, a.lastH
	}
	return 80, 24
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

func (a *tuiApp) applyFocusLocked() {
	if a.st.pane == tuiPaneActions {
		a.status.SetBorderColor(tcell.ColorYellow)
		a.logs.SetBorderColor(tcell.ColorWhite)
		a.status.SetTitle(" \u25ba " + tuiTr("TuiStatus", a.t, "Status") + " ")
		a.logs.SetTitle(" " + tuiTr("TuiLogs", a.t, "Logs") + " ")
		a.bar.SetTitle(" \u25ba " + tuiTr("TuiActions", a.t, "Actions") + " ")
	} else {
		a.status.SetBorderColor(tcell.ColorWhite)
		a.logs.SetBorderColor(tcell.ColorYellow)
		a.status.SetTitle(" " + tuiTr("TuiStatus", a.t, "Status") + " ")
		a.logs.SetTitle(" \u25ba " + tuiTr("TuiLogs", a.t, "Logs") + " ")
		a.bar.SetTitle(" " + tuiTr("TuiActions", a.t, "Actions") + " ")
	}
}

// layout keeps the old name for callers; it returns the stable pages root.
func (a *tuiApp) layout() tview.Primitive {
	return a.pages
}

// runTuiApp is the tview event loop: tcell decodes all input, redraws happen
// on every key AND a 1s tick, action results show immediately. The tree is
// built once; per-draw only content refreshes — SetRoot is never called
// again (SetRoot clears the screen and re-focuses the first child, which
// used to snap the selection back on every tick).
func runTuiApp() {
	restoreCP := tuiCodePageSwitch()
	defer restoreCP()
	a := newTuiApp()
	a.refresh()
	a.applyFocus()
	getSize := func() (int, int) {
		if w, h, err := tuiConsoleSize(); err == nil && w > 0 && h > 0 {
			return w, h
		}
		return 80, 24
	}
	a.app.SetAfterDrawFunc(func(screen tcell.Screen) {
		if w, h := screen.Size(); w > 0 && h > 0 {
			a.mu.Lock()
			a.lastW, a.lastH = w, h
			a.mu.Unlock()
			a.applyBodyClass(w)
		}
	})
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()
	// Probes run on the TICK goroutine, never the event loop: dial first
	// (may block ~1.5s during the omni boot window), then queue the
	// read-only content refresh onto the loop via QueueUpdateDraw.
	go func() {
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
	// Warm the caches once before the first draw so the initial frame has
	// real values (runTuiApp's own goroutine, not the loop — no keys yet).
	tuiProbeWorkerTick()
	// Input capture runs ON the event-loop goroutine (application.go: the
	// loop calls `event = inputCapture(event)` directly). Application.Draw
	// is QueueUpdate (blocking on the same loop), so ANY Draw call here
	// deadlocks on the first keypress. Never call Draw from this path:
	// returning nil makes the loop call a.draw() itself; returning ev
	// forwards to the root primitive then draws. QueueUpdateDraw is only
	// safe from other goroutines (the 1s tick below).
	a.setupInputCapture()
	if err := a.app.Run(); err != nil {
		printPlainSummary()
	}
}

// setupInputCapture installs the single key path (capture runs ON the event
// loop; it must never block on Draw — returning nil draws via a.draw()).
func (a *tuiApp) setupInputCapture() {
	a.app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
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
		if a.st.quit {
			a.app.Stop()
			return nil
		}
		return out
	})
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
			a.showHelpLocked()
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
