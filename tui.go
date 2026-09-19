package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// tui.go: terminal setup, main loop, actions, plain fallback.
// Rendering primitives and key handling live in tui_keys.go.

func takeTuiSnapshot() tuiSnapshot {
	cfg := loadConfig()
	h := checkOmniHealth()
	logMutex.Lock()
	logs := append([]string(nil), logBuffer...)
	logMutex.Unlock()
	if len(logs) > 200 {
		logs = logs[len(logs)-200:]
	}
	updateMu.Lock()
	opPhase := ""
	if updatePhaseNow != updateIdle && updatePhaseNow != updateFailed {
		opPhase = string(updatePhaseNow)
	}
	updateMu.Unlock()
	if isOmniOpRunning() && opPhase == "" {
		opPhase = "op"
	}
	// On-demand probe (cached ~2s by the live loop): the tray watchdog is
	// absent in TUI mode, so an empty probe state must not render as truth.
	probe, probeLabel := tuiLiveProbe(h.ProbeStatus, loadTranslations(cfg.Language))
	return tuiSnapshot{
		appVer:   AppVersion,
		omniVer:  h.Version,
		lang:     cfg.Language,
		theme:    cfg.Theme,
		status:   tuiStatusLabel(h.Status, loadTranslations(cfg.Language)),
		probe:    probe,
		probeLabel: probeLabel,
		port:     fmt.Sprintf("%d", OmniPort),
		nodeVer:  h.NodeVersion,
		external: h.ExternallyManaged,
		opPhase:  opPhase,
		logs:     logs,
	}
}

// Probe state lives in worker-maintained caches. The tick goroutine dials;
// the event loop only reads. probeOmniHealth blocks up to its timeout when
// OmniRoute is booting/hung, so calling it on the loop would hitch every
// keypress precisely when the owner is watching.
var (
	tuiProbeCacheMu sync.Mutex
	tuiProbeCache   string
	tuiProbeCacheAt time.Time
	// tuiDumpState pins the machine token for review dumps
	// (ORPANEL_TUI_STATE=running|stopped). Empty = live behaviour.
	tuiDumpStateMu sync.Mutex
	tuiDumpState   string
)

// tuiRefreshProbeCache dials OmniRoute and stores the outcome. WORKER ONLY:
// call from the tick goroutine (or the dump harness), never on the loop.
func tuiRefreshProbeCache(timeout time.Duration) {
	res := probeOmniHealth(timeout)
	tuiProbeCacheMu.Lock()
	defer tuiProbeCacheMu.Unlock()
	switch res.outcome {
	case probeHealthy:
		tuiProbeCache = "healthy"
	case probeDegraded:
		tuiProbeCache = "degraded"
	default:
		tuiProbeCache = "unreachable"
	}
	tuiProbeCacheAt = time.Now()
}

// tuiCachedProbe reads the worker cache. LOOP-SAFE: never dials. Empty
// cache reads as "unknown" (checking), never as down.
func tuiCachedProbe() string {
	tuiProbeCacheMu.Lock()
	defer tuiProbeCacheMu.Unlock()
	if tuiProbeCache == "" {
		return "unknown"
	}
	return tuiProbeCache
}

// tuiLiveProbe returns the cached probe + label. LOOP-SAFE: read-only.
// Labels are localized; unknown stays "checking", never an enum.
func tuiLiveProbe(current string, t map[string]string) (string, string) {
	p := tuiCachedProbe()
	if current == "unknown" && p == "unreachable" {
		p = "unknown"
	}
	return p, tuiProbeLabel(p, t)
}

// tuiProbeWorkerTick is the SINGLE place network probes start. Runs on the
// tick goroutine: refreshes stale caches (~2s omni, ~3s panel, warms the
// hourly npm version lookup), then the caller queues the read-only draw.
func tuiProbeWorkerTick() {
	tuiProbeCacheMu.Lock()
	stale := tuiProbeCache == "" || time.Since(tuiProbeCacheAt) > 2*time.Second
	tuiProbeCacheMu.Unlock()
	if stale {
		tuiRefreshProbeCache(1500 * time.Millisecond)
	}
	tuiPanelMu.Lock()
	pstale := tuiPanelMiss.IsZero() || time.Since(tuiPanelMiss) > 3*time.Second
	tuiPanelMu.Unlock()
	if pstale {
		tuiRefreshPanelCache()
	}
	getOmniLatestVersion()
}

// runTUI runs the full-screen interface (tview/tcell own all input).
// Terminal restore is handled by tcell on every exit path. If stdout is
// not a terminal it prints the plain summary instead.
func runTUI() {
	if script := tuiScriptKeys(); script != nil {
		runTuiScript(script)
		return
	}
	runTuiApp()
}

// tuiScriptKeys reads ORPANEL_TUI_SCRIPT: comma-separated key names
// (down,up,enter,q,?,tab, letters) or a file path with one name per line.
// Test seam only, not a user feature.
func tuiScriptKeys() []string {
	raw := os.Getenv("ORPANEL_TUI_SCRIPT")
	if raw == "" {
		return nil
	}
	if data, err := os.ReadFile(raw); err == nil {
		raw = string(data)
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		for _, line := range strings.Split(p, "\n") {
			if s := strings.TrimSpace(line); s != "" {
				out = append(out, strings.ToLower(s))
			}
		}
	}
	return out
}

// tuiSimKey maps a script name to a tcell key event for the sim harness.
// It flows through the real SetInputCapture closure, not around it.
func tuiSimKey(name string) (tcell.Key, rune) {
	switch name {
	case "down":
		return tcell.KeyDown, 0
	case "up":
		return tcell.KeyUp, 0
	case "left":
		return tcell.KeyLeft, 0
	case "right":
		return tcell.KeyRight, 0
	case "enter":
		return tcell.KeyEnter, 0
	case "tab":
		return tcell.KeyTab, 0
	case "esc", "escape":
		return tcell.KeyEscape, 0
	case "pgup", "pageup":
		return tcell.KeyPgUp, 0
	case "pgdn", "pagedown":
		return tcell.KeyPgDn, 0
	case "home":
		return tcell.KeyHome, 0
	case "end":
		return tcell.KeyEnd, 0
	case "q", "quit":
		return tcell.KeyRune, 'q'
	case "ctrlc", "ctrl-c", "sigint":
		return tcell.KeyCtrlC, 0
	case "?":
		return tcell.KeyRune, '?'
	default:
		if len([]rune(name)) == 1 {
			for _, r := range name {
				return tcell.KeyRune, r
			}
		}
		return tcell.KeyRune, 0
	}
}

// tuiSeedLogs fills the in-process log buffer with representative lines so
// scripted frames demonstrate a full log pane. Test seam only.
func tuiSeedLogs() {
	logMutex.Lock()
	defer logMutex.Unlock()
	logBuffer = nil
	now := time.Now().Format("2006-01-02 15:04:05")
	seeds := [][2]string{
		{"INFO", "panel starting on :20127"},
		{"INFO", "watchdog tick: probing omni health"},
		{"WARN", "port 20128 busy, retrying in 2s"},
		{"INFO", "omniroute answered health probe (200)"},
		{"ERROR", "npm view failed, using cached version"},
		{"INFO", "config saved (lang=tr theme=system)"},
		{"WARN", "node 24.20.0 below recommended 22+, continuing"},
		{"INFO", "autostart state synced with tray"},
		{"INFO", "themes loaded (dark/light)"},
		{"ERROR", "update check timed out, will retry"},
		{"INFO", "log retention sweep removed 0 entries"},
		{"WARN", "recovery backoff active (3 attempts)"},
		{"INFO", "tray tooltip refreshed"},
		{"INFO", "locale tr applied to 42 keys"},
	}
	for i, s2 := range seeds {
		logBuffer = append(logBuffer, "["+now+"] "+s2[0]+": seed line "+s2[1]+
			" #"+string(rune('a'+i)))
	}
}

// simFrame renders the app root at w,h on a simulation screen and reads the
// cell grid back to text. Size comes from the sim screen, never the console.
// NOTE: it draws a.pages directly, bypassing the Application — so frame
// dumps are structurally blind to SetRoot/afterDraw/input-capture wiring,
// and any change to that wiring needs an Application-level test alongside.
// It draws the root primitive directly: app.Draw/app.SetScreen queue on the
// event loop, which does not run in the harness — SetScreen a second time
// blocks forever on screenReplacement, so it must never be called here.
func simFrame(a *tuiApp, sim tcell.SimulationScreen, w, h int) string {
	sim.Init()
	sim.SetSize(w, h)
	sim.Clear()
	// Harness has no event loop, so no afterDraw ever runs: publish the
	// size the production afterDraw would, so header/bar degrade as live.
	a.mu.Lock()
	a.lastW, a.lastH = w, h
	a.mu.Unlock()
	a.applyBodyClass(w)
	a.refresh()
	a.applyFocus()
	a.pages.SetRect(0, 0, w, h)
	a.pages.Draw(sim)
	sim.Show()
	cells, cw, ch := sim.GetContents()
	var b strings.Builder
	for y := 0; y < ch; y++ {
		var row []rune
		for x := 0; x < cw; x++ {
			c := cells[y*cw+x]
			if len(c.Runes) == 0 {
				row = append(row, ' ')
			} else {
				row = append(row, c.Runes[0])
			}
		}
		b.WriteString(strings.TrimRight(string(row), " ") + "\n")
	}
	out := strings.TrimRight(b.String(), "\n")
	return out
}

// simInject feeds one scripted key through the REAL input path: the event is
// dispatched via the shared handleKeyEvent used by the live SetInputCapture
// closure (identical code path, no mode flag). Modal-open keys go through
// the same modal gate the live loop uses.
func simInject(a *tuiApp, sim tcell.SimulationScreen, name string) {
	key, r := tuiSimKey(name)
	if r == 0 && key == tcell.KeyRune {
		return
	}
	ev := tcell.NewEventKey(key, r, tcell.ModNone)
	if front, _ := a.pages.GetFrontPage(); front == "modal" {
		// Same gate as the live loop, including the Ctrl+C escape hatch:
		// modal keys reach the modal's own input handler; anything else
		// is swallowed so no global shortcut fires behind the dialog.
		if ev.Key() == tcell.KeyCtrlC {
			a.app.Stop()
			return
		}
		switch ev.Key() {
		case tcell.KeyEnter, tcell.KeyTab, tcell.KeyBacktab:
		// Route through the modal exactly as the live loop does: the
		// modal owns focus while open, and its handler consumes
		// Enter/Tab internally (focus/buttons), calling SetDoneFunc
		// only on close. a.app.SetFocus in showModal does not take
		// effect without the event loop, so focus the page first.
		a.pages.Focus(func(p tview.Primitive) { a.app.SetFocus(p) })
		if h := a.modalInputHandler(); h != nil {
			h(ev, func(p tview.Primitive) { a.app.SetFocus(p) })
		}
		if front2, _ := a.pages.GetFrontPage(); front2 == "modal" {
			a.pages.Focus(func(p tview.Primitive) { a.app.SetFocus(p) })
		} else {
			a.closeModalSync()
		}
		return
		case tcell.KeyEscape:
		// Esc: modal cancel func closes the page via SetDoneFunc(-1);
		// without the event loop the modal never has focus, so the
		// handler above is a no-op — fall through to the shared close
		// path so dumps and tests converge with the live loop.
		a.pages.Focus(func(p tview.Primitive) { a.app.SetFocus(p) })
		if h := a.modalInputHandler(); h != nil {
			h(ev, func(p tview.Primitive) { a.app.SetFocus(p) })
		}
		if front2, _ := a.pages.GetFrontPage(); front2 == "modal" {
			a.closeModalSync()
		} else {
			a.closeModalSync()
		}
		return
		default:
			return
		}
	}
	if ev.Key() == tcell.KeyCtrlC {
		a.app.Stop()
		return
	}
	a.handleKeyEvent(ev)
	_ = sim
}

// runTuiScript renders the REAL tview app on a tcell simulation screen and
// prints each frame. ORPANEL_TUI_SIZE=WxH overrides the dump size;
// ORPANEL_TUI_SCRIPT_SEED=1 fills the log pane with representative lines.
// Frames come from the same renderer the owner runs (tview + sim screen).
func runTuiScript(names []string) {
	if os.Getenv("ORPANEL_TUI_SCRIPT_SEED") != "" {
		tuiSeedLogs()
	}
	w, h := 80, 24
	if v := os.Getenv("ORPANEL_TUI_SIZE"); v != "" {
		var ww, hh int
		if _, err := fmt.Sscanf(v, "%dx%d", &ww, &hh); err == nil && ww > 0 && hh > 0 {
			w, h = ww, hh
		}
	}
	// ORPANEL_TUI_LANG pins the dump locale (default: whatever is saved).
	// Primary frames are dumped in tr, the owner's locale.
	if lang := os.Getenv("ORPANEL_TUI_LANG"); lang != "" {
		setCurrentLang(lang)
	}
	a := newTuiApp()
	// Panes resolve from the PINNED language: takeTuiSnapshot reads the
	// saved config (es on this machine), which would leave the header
	// token (snap.lang/snap.theme) disagreeing with the Turkish panes.
	// Same language/theme the panes use, so every token agrees.
	if st := os.Getenv("ORPANEL_TUI_STATE"); st == "running" || st == "stopped" {
		tuiProbeCacheMu.Lock()
		if st == "running" {
			tuiProbeCache = "healthy"
		} else {
			tuiProbeCache = "unreachable"
		}
		tuiProbeCacheAt = time.Now()
		tuiProbeCacheMu.Unlock()
		// Machine state for the entry builder: the dump env cannot
		// guarantee a live/dead OmniRoute on demand, so pin it here.
		tuiDumpStateMu.Lock()
		tuiDumpState = st
		tuiDumpStateMu.Unlock()
		defer func() {
			tuiDumpStateMu.Lock()
			tuiDumpState = ""
			tuiDumpStateMu.Unlock()
		}()
		// Seed snap BEFORE the first simFrame: handler + sync paths read
		// a.snap (not the probe) for the entry set, and refreshLocked
		// overwrites a.snap from the live snapshot afterwards.
		a.snap = tuiSnapshot{status: st, updateAvail: true}
		a.syncListsLocked(a.snap)
	}
	// Harness runs on its own goroutine (not the event loop), so the tick
	// below is safe. NOTE: tuiProbeWorkerTick refreshes STALE caches only —
	// the forced probe above is fresh (<2s), so it survives the warmup.
	tuiProbeWorkerTick()
	sim := tcell.NewSimulationScreen("UTF-8")
	for _, n := range names {
		if n == "tick" {
			w2 := w
			if v := os.Getenv("ORPANEL_TUI_SIZE"); v != "" {
				var ww, hh int
				if _, err := fmt.Sscanf(v, "%dx%d", &ww, &hh); err == nil && ww > 0 {
					w2 = ww
				}
			}
			// Worker tick first (dial on this harness goroutine, never the
			// loop), then the read-only refresh inside simFrame.
			tuiProbeWorkerTick()
			a.applyBodyClass(w2)
			a.refresh()
			a.applyFocus()
			fmt.Print(simFrame(a, sim, w, h) + "\n---FRAME---\n")
			continue
		}
		simInject(a, sim, n)
		fmt.Print(simFrame(a, sim, w, h) + "\n---FRAME---\n")
		if n == "q" || a.st.quit {
			break
		}
	}
}

// tuiKeyReader is retired: tcell decodes all console input (see tuiEventKey).
// Kept as documentation of the old byte protocol, not called.

// tuiPanelBase is overrideable in tests (httptest server URL).
var tuiPanelBase = ""

func tuiPanelURL() string {
	if tuiPanelBase != "" {
		return tuiPanelBase
	}
	return "http://127.0.0.1:" + strconv.Itoa(PanelPort)
}

var (
	tuiPanelMu   sync.Mutex
	tuiPanelHit  bool
	tuiPanelMiss time.Time
)

// tuiRefreshPanelCache dials 127.0.0.1:20127 and stores the answer. WORKER
// ONLY: call from the tick goroutine (or the dump harness), never on the
// event loop (dial latency would hitch keys during the boot window).
func tuiRefreshPanelCache() bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(tuiPanelURL() + "/api/status")
	ok := err == nil && resp != nil && resp.StatusCode < 500
	if resp != nil && resp.Body != nil {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
	}
	tuiPanelMu.Lock()
	defer tuiPanelMu.Unlock()
	tuiPanelMiss = time.Now()
	tuiPanelHit = ok
	return ok
}

// tuiCachedPanelServing reads the worker cache. LOOP-SAFE: never dials.
func tuiCachedPanelServing() bool {
	tuiPanelMu.Lock()
	defer tuiPanelMu.Unlock()
	if tuiPanelMiss.IsZero() {
		return false
	}
	if time.Since(tuiPanelMiss) > 3*time.Second {
		return tuiPanelHit
	}
	return tuiPanelHit
}

// tuiPollPanelLogs fetches GET /api/logs?last=N incrementally: last is the
// missed prefix index, the response carries {logs, newIndex}. Returns the
// new lines, the cursor to pass next time, and false on any failure (the
// caller keeps the explicit loading/empty state instead of an empty box).
func tuiPollPanelLogs(last int) ([]string, int, bool) {
	var out struct {
		Logs     []string `json:"logs"`
		NewIndex int      `json:"newIndex"`
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("%s/api/logs?last=%d", tuiPanelURL(), last))
	if err != nil || resp == nil {
		return nil, last, false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, last, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, last, false
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, last, false
	}
	if out.NewIndex < last {
		out.NewIndex = last
	}
	return out.Logs, out.NewIndex, true
}
// tuiPanelEndpoint maps mutating actions to panel API paths. Every action
// with a non-empty endpoint goes through the panel when one is serving.
// /api/omni/reinstall exists on the panel but no TUI action exposes it.
func tuiPanelEndpoint(act int) string {
	switch act {
	case tuiActStart:
		return "/api/start"
	case tuiActStop:
		return "/api/stop"
	case tuiActRestart:
		return "/api/restart"
	case tuiActUpdate:
		return "/api/omni/update"
	case tuiActRepair:
		return "/api/omni/repair"
	case tuiActInstall:
		return "/api/omni/install"
	default:
		return ""
	}
}

// tuiShouldUseClient is pure: client iff the action has a panel endpoint
// AND a panel answers. tuiDoAction consults it; tests assert it per action.
func tuiShouldUseClient(act int, panelAnswers bool) bool {
	return panelAnswers && tuiPanelEndpoint(act) != ""
}

// tuiPanelClient POSTs the action to the serving panel and surfaces failures
// on the message line. Async: the panel runs the op in the background.
func tuiPanelClient(act int, t map[string]string) string {
	tr := func(k, fb string) string {
		if v, ok := t[k]; ok && v != "" {
			return v
		}
		return fb
	}
	ep := tuiPanelEndpoint(act)
	if ep == "" {
		return ""
	}
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPost, tuiPanelURL()+ep, nil)
	if err != nil {
		return tr("TuiOpFailed", "failed") + ": " + err.Error()
	}
	resp, err := client.Do(req)
	if err != nil {
		return tr("TuiOpFailed", "failed") + ": " + err.Error()
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		return tr("TuiOpFailed", "failed") + ": HTTP " + strconv.Itoa(resp.StatusCode)
	}
	return tr("TuiOpStarted", "Started")
}

// tuiDoAction dispatches one action id. Mutating actions (every action with
// a panel endpoint) go through the serving panel when one answers, so the
// tray watchdog never races a local stop/start and two processes never run
// concurrent global npm ops (the op mutex is per-process).
func tuiDoAction(act int, t map[string]string) string {
	tr := func(k, fb string) string {
		if v, ok := t[k]; ok && v != "" {
			return v
		}
		return fb
	}
	if tuiShouldUseClient(act, tuiCachedPanelServing()) {
		return tuiPanelClient(act, t)
	}
	switch act {
	case tuiActStart:
		if isOmniOpRunning() {
			return tr("TuiBusyOp", "busy")
		}
		go startOmniroute()
		return tr("TuiOpStarted", "Started")
	case tuiActStop:
		go stopOmniroute()
		return tr("TuiOpStarted", "Started")
	case tuiActRestart:
		if isOmniOpRunning() {
			return tr("TuiBusyOp", "busy")
		}
		go func() { stopOmniroute(); startOmniroute() }()
		return tr("TuiOpStarted", "Started")
	case tuiActUpdate:
		if isOmniOpRunning() {
			return tr("TuiBusyOp", "busy")
		}
		go runNpmOmni("update", "update", "-g", "omniroute@latest")
		return tr("TuiOpStarted", "Started")
	case tuiActRepair:
		if isOmniOpRunning() {
			return tr("TuiBusyOp", "busy")
		}
		go runNpmOmni("repair", "install", "-g", "omniroute@latest", "--force")
		return tr("TuiOpStarted", "Started")
	case tuiActInstall:
		if isOmniOpRunning() {
			return tr("TuiBusyOp", "busy")
		}
		go runNpmOmni("install", "install", "-g", "omniroute@latest")
		return tr("TuiOpStarted", "Started")
	case tuiActAutostart:
		_ = setAutoStart(!isAutoStartEnabled())
		return tr("TuiAutostart", "Autostart")
	case tuiActLanguage:
		cfg := loadConfig()
		next := "en"
		switch cfg.Language {
		case "en":
			next = "tr"
		case "tr":
			next = "es"
		default:
			next = "en"
		}
		saveConfig(next, cfg.AutoStart)
		return next
	case tuiActTheme:
		cfg := loadConfig()
		next := ThemeLight
		switch cfg.Theme {
		case ThemeLight:
			next = ThemeDark
		case ThemeDark:
			next = ThemeSystem
		default:
			next = ThemeLight
		}
		_ = saveTheme(next)
		// The TUI repaints its own palette immediately (system = terminal
		// defaults, NO_COLOR wins); report what actually changed.
		return tuiThemeMessage(next, t)
	case tuiActWebUI:
		openBrowser("http://localhost:20127")
		return tr("TuiOpenedBrowser", "Opened")
	default:
		return ""
	}
}

// plainSummaryText builds the non-TTY fallback: stable order, no ESC bytes.
func plainSummaryText() string {
	cfg := loadConfig()
	h := checkOmniHealth()
	lines := []string{
		fmt.Sprintf("orpanel v%s", AppVersion),
		fmt.Sprintf("language=%s theme=%s autostart=%v", cfg.Language, cfg.Theme, isAutoStartEnabled()),
		fmt.Sprintf("omniroute status=%s probe=%s port=%d node=%s version=%s", h.Status, h.ProbeStatus, OmniPort, h.NodeVersion, h.Version),
		fmt.Sprintf("update=%s", getUpdateStatus().Phase),
	}
	return strings.Join(lines, "\n")
}

// printPlainSummary is the non-TTY fallback entry point.
func printPlainSummary() {
	fmt.Println(plainSummaryText())
}
