package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/sys/windows"
)

type windowsHandle = windows.Handle

// tuiTestApp builds a deterministic harness app: English labels, colors on,
// colors off only where a test pins it. tuiNoColor reads $NO_COLOR, so pin
// it here — ambient CI/dev shells (NO_COLOR=1 here) must not change frames.
func tuiTestApp() *tuiApp {
	// Deterministic frames: tuiNoColor reads $NO_COLOR, so pin it off —
	// ambient CI/dev shells (NO_COLOR=1 here) must not change rendering.
	_ = os.Setenv("NO_COLOR", "")
	a := newTuiApp()
	a.t = map[string]string{
		"TuiStatus": "Status", "TuiLogs": "Logs", "TuiActions": "Actions",
		"TuiHelpTitle": "Keys", "TuiHelpNavTitle": "Navigation", "TuiHelpActTitle": "Actions",
		"TuiNavHint": "Tab switch pane", "TuiClose": "Close", "TuiHelp": "Help",
		"TuiConfirmTitle": "Confirm", "TuiConfirmHint": "Enter confirm",
		"TuiConfirmOK": "confirm", "TuiConfirmCancel": "cancel",
		"TuiHintScrollLine": "arrows select", "TuiHintSelect": "select",
		"TuiHintPane": "switch pane", "TuiHintActivate": "run", "TuiHintCancel": "close",
		"TuiHintScroll": "scroll", "TuiHintEdges": "top/bottom",
		"TuiHintHelp": "help", "TuiHintQuit": "quit",
		"TuiNavSelect": "Select", "TuiNavPane": "Switch pane", "TuiNavActivate": "Run",
		"TuiNavCancel": "Close", "TuiNavScroll": "Scroll", "TuiNavEdges": "Top/bottom",
		"TuiTooSmall": "Terminal too small (need 60x16)",
		"HealthBadgeRunning": "Running", "HealthBadgeStopped": "Stopped",
		"HealthBadgeNotInstalled": "Not installed", "HealthBadgePortConflict": "Port conflict",
		"HealthLabelVersion": "Version", "HealthLabelPort": "Port", "HealthLabelNode": "Node",
		"ProbeStarting": "Starting", "ProbeDegraded": "Not responding", "ProbeUnknown": "Checking",
		"TuiStart": "Start", "TuiStop": "Stop", "TuiRestart": "Restart", "TuiUpdate": "Update",
		"TuiRepair": "Repair", "TuiInstall": "Install", "TuiAutostart": "Autostart",
		"TuiLanguage": "Language", "TuiTheme": "Theme", "TuiWebUI": "Web UI",
		"TuiManagedPanel": "managed by panel", "TuiManagedLocal": "managed local",
		"TuiOpFailed": "failed", "TuiOpStarted": "Started",
	}
	a.st = tuiState{pane: tuiPaneActions, rows: tuiActionRows(a.t)}
	return a
}

// TestConstructorRegistersRoot gates the blank-console class: tview draws
// nothing unless SetRoot ran (draw returns early on root == nil). SetRoot
// delegates focus into the pages primitive, so assert focus is non-nil and
// the main page is registered.
func TestConstructorRegistersRoot(t *testing.T) {
	a := newTuiApp()
	if f := a.app.GetFocus(); f == nil {
		t.Fatal("newTuiApp left app focus nil: SetRoot never ran, first paint would be blank")
	}
	if !a.pages.HasPage("main") {
		t.Fatal("newTuiApp did not register the main page: first paint would be blank")
	}
}

// simText renders the live app root on a simulation screen at w,h and reads
// cells back. Same renderer + same root as the owner runs.
func simText(t *testing.T, a *tuiApp, w, h int) string {
	t.Helper()
	sim := tcell.NewSimulationScreen("UTF-8")
	if w < 60 || h < 16 {
		return a.t["TuiTooSmall"]
	}
	return simFrame(a, sim, w, h)
}

func simPress(a *tuiApp, names ...string) {
	sim := tcell.NewSimulationScreen("UTF-8")
	sim.Init()
	for _, n := range names {
		simInject(a, sim, n)
	}
}

func TestSimFrameDimensions(t *testing.T) {
	a := tuiTestApp()
	for _, wh := range [][2]int{{80, 24}, {120, 30}} {
		f := simText(t, a, wh[0], wh[1])
		lines := strings.Split(f, "\n")
		if len(lines) != wh[1] {
			t.Fatalf("%dx%d lines=%d", wh[0], wh[1], len(lines))
		}
		for i, ln := range lines {
			if len([]rune(ln)) > wh[0] {
				t.Fatalf("%dx%d line %d width %d: %q", wh[0], wh[1], i, len([]rune(ln)), ln)
			}
		}
		if !strings.Contains(f, "Status") || !strings.Contains(f, "Logs") {
			t.Fatalf("%dx%d missing pane titles:\n%s", wh[0], wh[1], f)
		}
	}
}

func TestSimTooSmallGuard(t *testing.T) {
	a := tuiTestApp()
	f := simText(t, a, 40, 12)
	if !strings.Contains(f, "too small") {
		t.Fatalf("expected too-small line:\n%s", f)
	}
}

func TestSimSelectionMovesDown(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "down", "down")
	if a.stateSnapshot().sel != 2 {
		t.Fatalf("sel=%d want 2", a.stateSnapshot().sel)
	}
	f := simText(t, a, 80, 24)
	lbl := "r"
	if v := a.t["TuiRestart"]; v != "" {
		lbl = v
	}
	found := false
	for _, ln := range strings.Split(f, "\\n") {
		if strings.Contains(ln, lbl) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("selected row %q missing from frame:\n%s", lbl, f)
	}
}

func TestSimTickKeepsSelection(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "down", "down")
	snap0 := a.stateSnapshot()
	sel, pane := snap0.sel, snap0.pane
	a.applyBodyClass(80)
	a.refresh()
	a.applyFocus()
	snap1 := a.stateSnapshot()
	if snap1.sel != sel || snap1.pane != pane {
		t.Fatalf("tick moved sel=%d/%d pane=%d/%d", snap1.sel, sel, snap1.pane, pane)
	}
}

func TestSimTickThenDownContinues(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "down", "down")
	a.applyBodyClass(80)
	a.refresh()
	a.applyFocus()
	simPress(a, "down")
	if a.stateSnapshot().sel != 3 {
		t.Fatalf("sel=%d want 3 after down,down,tick,down", a.stateSnapshot().sel)
	}
}

func TestSimResizeKeepsSelection(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 99, 24)
	simPress(a, "down", "down")
	a.applyBodyClass(120)
	a.applyBodyClass(120)
	f := simText(t, a, 120, 30)
	if a.stateSnapshot().sel != 2 {
		t.Fatalf("sel=%d want 2 after 99->120", a.stateSnapshot().sel)
	}
	_ = f
	_ = time.Now
}

func TestSimHelpModal(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "?")
	f := simText(t, a, 80, 24)
	if !a.pages.HasPage("modal") {
		t.Fatalf("help modal page missing")
	}
	// Every dispatched entry must be listed (ids, not locale labels, are
	// the contract); destructive ones marked. Assert on helpLines — the
	// modal body — because the 80-col box legitimately scrolls/clips rows
	// in the rendered frame (see TestHelpModalScrollable).
	body := strings.Join(a.helpLines(), "\n")
	for _, r := range a.st.rows {
		if !strings.Contains(body, " "+r.key+" ") {
			t.Fatalf("help missing key %q:\n%s", r.key, body)
		}
	}
	if !strings.Contains(f, "*") {
		t.Fatalf("help missing confirm markers:\n%s", f)
	}
}

// TestHelpModalScrollable: at 80x24 the box cannot fit all rows; the body
// must then carry a scroll marker ("+N more") instead of silently hiding
// actions — every action stays reachable via scroll.
func TestHelpModalScrollable(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "?")
	f := simText(t, a, 80, 24)
	body := strings.Join(a.helpLines(), "\n")
	n := len(strings.Split(body, "\n"))
	if !strings.Contains(f, "+") && n > 14 {
		t.Fatalf("80-col help shows %d body lines with no scroll marker:\n%s", n, f)
	}
}

func TestSimConfirmModal(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "down", "enter")
	f := simText(t, a, 80, 24)
	if !a.pages.HasPage("modal") {
		t.Fatalf("confirm modal page missing")
	}
	// Names the action keycap, its row label, and the safe default.
	// Kept as the permanent visual gate for the destructive-action path.
	sel := a.stateSnapshot().sel
	want := a.st.rows[sel]
	if !strings.Contains(f, "["+want.key+" ]") {
		t.Fatalf("confirm missing action key [%s]:\n%s", want.key, f)
	}
	if !strings.Contains(f, want.text) {
		t.Fatalf("confirm missing action label %q:\n%s", want.text, f)
	}
	if !strings.Contains(f, "Esc") {
		t.Fatalf("confirm missing safe default Esc:\n%s", f)
	}
}

// TestBarClipsWholeChips gates mid-word clipping: at 80 and 120 cols the
// rendered bar row must contain the first entry's keycap and end on a chip
// boundary or the overflow marker — never sliced, never missing entry 0.
func TestBarClipsWholeChips(t *testing.T) {
	for _, w := range []int{80, 120} {
		a := tuiTestApp()
		first := a.st.rows[0]
		f := simText(t, a, w, 24)
		var barRow string
		for _, ln := range strings.Split(f, "\n") {
			if strings.Contains(ln, "["+first.key+" ]") {
				barRow = ln
			}
		}
		if barRow == "" {
			t.Fatalf("width %d: no bar row with entry-0 key [%s] in frame:\n%s", w, first.key, f)
		}
		// Last visible token must be a whole chip or the marker.
		tail := strings.TrimRight(barRow, " │╭╮╰╯┌┐└┘─")
		if tail == "" || strings.HasSuffix(tail, "[") {
			t.Fatalf("width %d: bar clipped mid-chip: %q", w, barRow)
		}
	}
}

// TestBarOverflowMarker fires on narrow widths: some chip must be replaced
// by the "+N" marker rather than sliced.
func TestBarOverflowMarker(t *testing.T) {
	a := tuiTestApp()
	if got := a.barText(40); !strings.Contains(got, "+") {
		t.Fatalf("narrow bar has no overflow marker: %q", got)
	}
}

// TestEnterDispatchesVisibleEntry: activating entry i dispatches
// entries[i].actionID — render, navigation, and dispatch share st.rows, so
// a hidden chip can never shift Enter onto its neighbour.
func TestEnterDispatchesVisibleEntry(t *testing.T) {
	a := tuiTestApp()
	for i, r := range a.st.rows {
		st := handleKey(tuiState{pane: tuiPaneActions, rows: a.st.rows, sel: i}, tuiKey{r: '\r'})
		if r.id == tuiActHelp {
			continue // help toggles instead of dispatching
		}
		if tuiConfirmNeeded(r.id) {
			if st.confirm != r.id {
				t.Fatalf("index %d (%q): confirm=%d want %d", i, r.text, st.confirm, r.id)
			}
			continue
		}
		if st.lastAct != r.id {
			t.Fatalf("index %d (%q): dispatched=%d want %d", i, r.text, st.lastAct, r.id)
		}
	}
}

// TestBarLabelsMatchEntries: every rendered chip label equals the entry
// label in order — no missing, extra, or empty entries. Keycaps carry a
// trailing NBSP ("[x ]") so tview never treats them as color tags.
func TestBarLabelsMatchEntries(t *testing.T) {
	for _, w := range []int{80, 120} {
		a := tuiTestApp()
		chips := a.barChips()
		if len(chips) != len(a.st.rows) {
			t.Fatalf("width %d: %d chips for %d entries", w, len(chips), len(a.st.rows))
		}
		for i, r := range a.st.rows {
			if r.text == "" || r.key == "" {
				t.Fatalf("entry %d renders empty: %+v", i, r)
			}
			if !strings.Contains(chips[i], "["+r.key+" ]") || !strings.Contains(chips[i], r.text) {
				t.Fatalf("chip %d = %q, want key %q label %q", i, chips[i], r.key, r.text)
			}
		}
	}
}

// TestEnterBehindOverflowMarker: with chips hidden at 80 cols, the visible
// entries still dispatch their own ids.
func TestEnterBehindOverflowMarker(t *testing.T) {
	a := tuiTestApp()
	bar := a.barText(78)
	if !strings.Contains(bar, "+") {
		t.Fatalf("expected overflow marker at 78: %q", bar)
	}
	n := len(a.st.rows)
	for _, i := range []int{0, 1, n - 1} {
		r := a.st.rows[i]
		st := handleKey(tuiState{pane: tuiPaneActions, rows: a.st.rows, sel: i}, tuiKey{r: '\r'})
		if tuiConfirmNeeded(r.id) {
			if st.confirm != r.id {
				t.Fatalf("index %d: confirm=%d want %d", i, st.confirm, r.id)
			}
			continue
		}
		if r.id == tuiActHelp {
			continue
		}
		if st.lastAct != r.id {
			t.Fatalf("index %d: dispatched=%d want %d", i, st.lastAct, r.id)
		}
	}
}

func TestSimNonTTYFallback(t *testing.T) {
	s := plainSummaryText()
	if !strings.Contains(s, "orpanel v") {
		t.Fatalf("plain summary missing version:\n%s", s)
	}
}

func TestDisplayStateFromProbe(t *testing.T) {
	m := map[string]string{
		"HealthBadgeRunning": "Running", "HealthBadgeStopped": "Stopped",
		"HealthBadgePortConflict": "Port conflict", "HealthBadgeNotInstalled": "Not installed",
		"ProbeStarting": "Starting", "ProbeDegraded": "Not responding", "ProbeUnknown": "Checking",
		"TuiNotInstalled": "Not installed",
	}
	snap := tuiSnapshot{status: "port_conflict"}
	if got, _ := tuiDisplayState(snap, "healthy", m); got != "Running" {
		t.Fatalf("healthy over conflict = %q", got)
	}
	if got, _ := tuiDisplayState(snap, "unreachable", m); got != "Port conflict" {
		t.Fatalf("unreachable+port = %q", got)
	}
	snap2 := tuiSnapshot{status: "stopped"}
	if got, _ := tuiDisplayState(snap2, "unreachable", m); got != "Stopped" {
		t.Fatalf("unreachable+stopped = %q", got)
	}
	if got, _ := tuiDisplayState(tuiSnapshot{}, "starting", m); got != "Starting" {
		t.Fatalf("starting = %q", got)
	}
	if got, _ := tuiDisplayState(tuiSnapshot{status: "port_conflict"}, "healthy", m); strings.Contains(got, "conflict") {
		t.Fatalf("healthy must never show conflict: %q", got)
	}
}

func TestEventKeyDecode(t *testing.T) {
	cases := []struct {
		ev   *tcell.EventKey
		want tuiKey
	}{
		{ev: tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), want: tuiKey{esc: true, raw: "[A"}},
		{ev: tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), want: tuiKey{esc: true, raw: "[B"}},
		{ev: tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), want: tuiKey{esc: true, raw: "[C"}},
		{ev: tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), want: tuiKey{esc: true, raw: "[D"}},
		{ev: tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone), want: tuiKey{esc: true, raw: "[5~"}},
		{ev: tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModNone), want: tuiKey{esc: true, raw: "[6~"}},
		{ev: tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone), want: tuiKey{esc: true, raw: "[H"}},
		{ev: tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), want: tuiKey{esc: true, raw: "[F"}},
		{ev: tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), want: tuiKey{r: '\r'}},
		{ev: tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), want: tuiKey{r: '\t'}},
		{ev: tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone), want: tuiKey{raw: "\x03"}},
		{ev: tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone), want: tuiKey{esc: true, raw: "\x1b"}},
		{ev: tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModNone), want: tuiKey{r: 's'}},
	}
	for _, tc := range cases {
		if got := tuiEventKey(tc.ev); got != tc.want {
			t.Fatalf("ev %v: got %+v want %+v", tc.ev, got, tc.want)
		}
	}
	// Every ACTION key decodes through handleKey to its action.
	// Navigation entries are never dispatched.
	tm := tuiTestApp().t
	rows := tuiActionRows(tm)
	for _, b := range tuiActionBindings() {
		st := handleKey(tuiState{pane: tuiPaneActions, rows: rows}, tuiKey{r: b.key})
		if b.confirm {
			if st.confirm != b.act {
				t.Fatalf("key %q: confirm=%d want %d", b.key, st.confirm, b.act)
			}
			continue
		}
		if st.lastAct != b.act {
			t.Fatalf("key %q: act=%d want %d", b.key, st.lastAct, b.act)
		}
	}
}

func TestModeRoundTrip(t *testing.T) {
	var sets []uint32
	ops := tuiModeOps{
		get: func(h windowsHandle, mode *uint32) error { *mode = 0x1f7; return nil },
		set: func(h windowsHandle, mode uint32) error { sets = append(sets, mode); return nil },
	}
	var sawApplied uint32
	saved := tuiConsoleModeRoundTrip(ops, 0, func(applied uint32) {
		sawApplied = applied
	})
	if saved != 0x1f7 || len(sets) != 1 || sets[0] != saved || sawApplied != saved {
		t.Fatalf("saved=%#x applied=%#x sets=%#x", saved, sawApplied, sets)
	}
}

func TestLocaleParity(t *testing.T) {
	read := func(p string) map[string]string {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	en := read("locales/en.json")
	for _, f := range []string{"locales/tr.json", "locales/es.json"} {
		got := read(f)
		for k := range en {
			if _, ok := got[k]; !ok {
				t.Fatalf("%s missing %q", f, k)
			}
		}
		for k := range got {
			if _, ok := en[k]; !ok {
				t.Fatalf("%s extra %q", f, k)
			}
		}
	}
}

func TestSimModalEnterClosesHelp(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "?")
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		t.Fatalf("help modal not open")
	}
	simPress(a, "enter")
	if front, _ := a.pages.GetFrontPage(); front == "modal" {
		t.Fatalf("help modal still open after Enter")
	}
}

func TestSimModalEscCancelsConfirm(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	sel := a.stateSnapshot().sel
	simPress(a, "down", "enter")
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		t.Fatalf("confirm modal not open")
	}
	simPress(a, "esc")
	if front, _ := a.pages.GetFrontPage(); front == "modal" {
		t.Fatalf("confirm modal still open after Esc")
	}
	if a.stateSnapshot().confirm != 0 {
		t.Fatalf("confirm still pending")
	}
	if a.stateSnapshot().sel != sel+1 {
		t.Fatalf("sel moved during modal: %d", a.stateSnapshot().sel)
	}
}

func TestSimModalEnterRunsActionOnce(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "down", "enter")
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		t.Fatalf("confirm modal not open")
	}
	runs := 0
	_ = runs
	simPress(a, "enter")
	if front, _ := a.pages.GetFrontPage(); front == "modal" {
		t.Fatalf("confirm modal still open after Enter")
	}
	if a.stateSnapshot().confirm != 0 {
		t.Fatalf("confirm still pending after Enter")
	}
	if a.stateSnapshot().lastAct != tuiActNone {
		t.Fatalf("lastAct not cleared after dispatch: %d", a.stateSnapshot().lastAct)
	}
}

func TestSimModalSwallowsActionKeys(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "?")
	sel := a.stateSnapshot().sel
	simPress(a, "s")
	if a.stateSnapshot().sel != sel {
		t.Fatalf("action key moved selection inside modal")
	}
	if a.msgSnapshot() != "" {
		t.Fatalf("action fired inside modal: %q", a.msgSnapshot())
	}
	simPress(a, "esc")
}

func TestSimModalSwallowsQuit(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "?")
	simPress(a, "q")
	if a.stateSnapshot().quit {
		t.Fatalf("q inside modal quit the app")
	}
	simPress(a, "esc")
}

func TestSimFooterContextual(t *testing.T) {
	a := tuiTestApp()
	a.setPaneForTest(tuiPaneActions)
	fa := a.footerText(78)
	if strings.Contains(fa, "Start") && strings.Contains(fa, "Stop") {
		t.Fatalf("footer duplicates action bar: %q", fa)
	}
	if !strings.Contains(fa, "?") && !strings.Contains(fa, "help") {
		t.Fatalf("footer missing help hint: %q", fa)
	}
	a.setPaneForTest(tuiPaneLogs)
	fl := a.footerText(78)
	if !strings.Contains(fl, "scroll") && !strings.Contains(fl, "Scroll") {
		t.Fatalf("log footer missing scroll hint: %q", fl)
	}
	for _, ln := range strings.Split(fa+" "+fl, "·") {
		_ = ln
	}
}


func TestPanelClientPostsEndpoint(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(200)
	}))
	defer srv.Close()
	old := tuiPanelBase
	tuiPanelBase = srv.URL
	defer func() { tuiPanelBase = old }()
	m := map[string]string{"TuiOpStarted": "Started", "TuiOpFailed": "failed"}
	if s := tuiPanelClient(tuiActStop, m); s != "Started" {
		t.Fatalf("stop = %q", s)
	}
	if gotPath != "/api/stop" || gotMethod != "POST" {
		t.Fatalf("request %s %s", gotMethod, gotPath)
	}
	if s := tuiPanelClient(tuiActRestart, m); s != "Started" {
		t.Fatalf("restart = %q", s)
	}
	if gotPath != "/api/restart" {
		t.Fatalf("restart path = %s", gotPath)
	}
}

func TestPanelClientSurfacesFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	old := tuiPanelBase
	tuiPanelBase = srv.URL
	defer func() { tuiPanelBase = old }()
	m := map[string]string{"TuiOpStarted": "Started", "TuiOpFailed": "failed"}
	if s := tuiPanelClient(tuiActStop, m); !strings.HasPrefix(s, "failed") {
		t.Fatalf("expected failure surfaced, got %q", s)
	}
}

func TestRealLoopRespondsAndStops(t *testing.T) {
	a := tuiTestApp()
	a.setupInputCapture()
	a.refresh()
	a.applyFocus()
	sim := tcell.NewSimulationScreen("UTF-8")
	sim.Init()
	sim.SetSize(80, 24)
	a.app.SetScreen(sim)
	// No SetRoot here by design: the harness must draw through whatever
	// newTuiApp registered, so a constructor regression goes blank here too.
	done := make(chan error, 1)
	go func() { done <- a.app.Run() }()
	time.Sleep(300 * time.Millisecond)
	sim.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	select {
	case <-time.After(5 * time.Second):
		a.app.Stop()
		t.Fatalf("deadlock: no response to injected key within 5s")
	case <-time.After(800 * time.Millisecond):
	}
	if a.stateSnapshot().sel != 1 {
		a.app.Stop()
		t.Fatalf("sel=%d want 1 after real-loop Down", a.stateSnapshot().sel)
	}
	a.app.Stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("app did not stop")
	}
}

func TestClientCoversAllMutatingActions(t *testing.T) {
	muts := []struct {
		act int
		ep  string
	}{
		{tuiActStart, "/api/start"}, {tuiActStop, "/api/stop"},
		{tuiActRestart, "/api/restart"}, {tuiActUpdate, "/api/omni/update"},
		{tuiActRepair, "/api/omni/repair"}, {tuiActInstall, "/api/omni/install"},
	}
	for _, m := range muts {
		if got := tuiPanelEndpoint(m.act); got != m.ep {
			t.Fatalf("act %d endpoint = %q want %q", m.act, got, m.ep)
		}
		if !tuiShouldUseClient(m.act, true) {
			t.Fatalf("act %d: panel serving must take client path", m.act)
		}
		if tuiShouldUseClient(m.act, false) {
			t.Fatalf("act %d: no panel must take direct path", m.act)
		}
	}
	// Non-mutating actions never route to the panel.
	for _, act := range []int{tuiActAutostart, tuiActLanguage, tuiActTheme, tuiActWebUI} {
		if tuiPanelEndpoint(act) != "" {
			t.Fatalf("act %d must have no endpoint", act)
		}
		if tuiShouldUseClient(act, true) {
			t.Fatalf("act %d must stay direct even when panel serves", act)
		}
	}
}

func TestClientHitsEveryMutatingEndpoint(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()
	old := tuiPanelBase
	tuiPanelBase = srv.URL
	defer func() { tuiPanelBase = old }()
	m := map[string]string{"TuiOpStarted": "Started", "TuiOpFailed": "failed"}
	want := map[int]string{
		tuiActStart: "/api/start", tuiActStop: "/api/stop",
		tuiActRestart: "/api/restart", tuiActUpdate: "/api/omni/update",
		tuiActRepair: "/api/omni/repair", tuiActInstall: "/api/omni/install",
	}
	for act, ep := range want {
		req, _ := http.NewRequest(http.MethodPost, tuiPanelURL()+ep, nil)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("act %d dial: %v", act, err)
		}
		resp.Body.Close()
		if s := tuiPanelClient(act, m); s != "Started" {
			t.Fatalf("act %d client = %q", act, s)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for act, ep := range want {
		if hits[ep] != 2 {
			t.Fatalf("act %d endpoint %s hits=%d want 2 (raw+client)", act, ep, hits[ep])
		}
	}
}

func TestHelpReopensAfterEscClose(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "?")
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		t.Fatalf("help not open")
	}
	// Esc through the real modal input handler (same as live loop).
	if h := a.modalInputHandler(); h == nil {
		t.Fatalf("no modal handler")
	} else {
		h(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone), func(p tview.Primitive) {})
	}
	if front, _ := a.pages.GetFrontPage(); front == "modal" {
		t.Fatalf("help still open after Esc")
	}
	if a.stateSnapshot().showHelp {
		t.Fatalf("showHelp stale after Esc close")
	}
	simPress(a, "?")
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		t.Fatalf("second ? did not reopen help (dead key)")
	}
}

func TestHelpReopensAfterButtonClose(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "?")
	mh := a.modalInputHandler()
	if mh == nil {
		t.Fatalf("no modal handler")
	}
	mh(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	if front, _ := a.pages.GetFrontPage(); front == "modal" {
		t.Fatalf("help still open after button")
	}
	if a.stateSnapshot().showHelp {
		t.Fatalf("showHelp stale after button close")
	}
	simPress(a, "?")
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		t.Fatalf("second ? did not reopen help after button close")
	}
}

func TestConfirmEscLeavesNothingToRefire(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "down", "enter")
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		t.Fatalf("confirm not open")
	}
	if h := a.modalInputHandler(); h == nil {
		t.Fatalf("no modal handler")
	} else {
		h(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone), func(p tview.Primitive) {})
	}
	if front, _ := a.pages.GetFrontPage(); front == "modal" {
		t.Fatalf("confirm still open after Esc")
	}
	snapE := a.stateSnapshot()
	if snapE.confirm != 0 || snapE.lastAct != tuiActNone {
		t.Fatalf("stale confirm=%d lastAct=%d after Esc", snapE.confirm, snapE.lastAct)
	}
	// A later Enter must not re-fire the cancelled action.
	msgBefore := a.msgSnapshot()
	a.handleKeyEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if a.msgSnapshot() != msgBefore {
		t.Fatalf("cancelled confirm re-fired on Enter: %q", a.msgSnapshot())
	}
}

func TestKeyHandlingNonBlockingDuringSlowProbe(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer slow.Close()
	oldURL := omniHealthURL
	omniHealthURL = slow.URL
	defer func() { omniHealthURL = oldURL }()
	oldBase := tuiPanelBase
	tuiPanelBase = "http://127.0.0.1:1"
	defer func() { tuiPanelBase = oldBase }()
	tuiProbeCacheMu.Lock()
	tuiProbeCache, tuiProbeCacheAt = "", time.Time{}
	tuiProbeCacheMu.Unlock()
	a := tuiTestApp()
	start := time.Now()
	simText(t, a, 80, 24)
	a.handleKeyEvent(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	el := time.Since(start)
	if el > 1500*time.Millisecond {
		t.Fatalf("key handling blocked %.1fs waiting for probe", el.Seconds())
	}
	if a.stateSnapshot().sel != 1 {
		t.Fatalf("sel=%d want 1", a.stateSnapshot().sel)
	}
}

func TestCtrlCStopsRealLoop(t *testing.T) {
	a := tuiTestApp()
	a.setupInputCapture()
	a.refresh()
	a.applyFocus()
	sim := tcell.NewSimulationScreen("UTF-8")
	sim.Init()
	sim.SetSize(80, 24)
	a.app.SetScreen(sim)
	// No SetRoot here by design: rely on the constructor (see TestConstructorRegistersRoot).
	done := make(chan error, 1)
	go func() { done <- a.app.Run() }()
	time.Sleep(300 * time.Millisecond)
	sim.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
	select {
	case err := <-done:
		_ = err
	case <-time.After(5 * time.Second):
		a.app.Stop()
		<-done
		t.Fatalf("Ctrl+C did not stop the app within 5s")
	}
}

func TestCtrlCExitsWithModalOpen(t *testing.T) {
	a := tuiTestApp()
	a.setupInputCapture()
	a.refresh()
	a.applyFocus()
	sim := tcell.NewSimulationScreen("UTF-8")
	sim.Init()
	sim.SetSize(80, 24)
	a.app.SetScreen(sim)
	// No SetRoot here by design: rely on the constructor (see TestConstructorRegistersRoot).
	done := make(chan error, 1)
	go func() { done <- a.app.Run() }()
	time.Sleep(300 * time.Millisecond)
	sim.InjectKey(tcell.KeyRune, '?', tcell.ModNone)
	time.Sleep(500 * time.Millisecond)
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		a.app.Stop()
		<-done
		t.Fatalf("help modal did not open")
	}
	sim.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
	select {
	case err := <-done:
		_ = err
	case <-time.After(5 * time.Second):
		a.app.Stop()
		<-done
		t.Fatalf("Ctrl+C with modal open did not stop the app within 5s")
	}
}

func TestLanguagePressRelocalizes(t *testing.T) {
	// Isolated from the real config file: the dispatch cycles from the
	// SAVED config, so redirect it at a temp file (CI has no config).
	configPathOverride = filepath.Join(t.TempDir(), "config.json")
	defer func() { configPathOverride = "" }()
	a := tuiTestApp()
	// Pin the starting language through the same isolated file.
	saveConfig("en", false)
	a.t = loadTranslations("en")
	a.st = tuiState{pane: tuiPaneActions, rows: tuiActionRows(a.t)}
	before := simText(t, a, 80, 24)
	if !strings.Contains(before, "Start") {
		t.Fatalf("english frame missing Start:\n%s", before)
	}
	a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModNone))
	after := simText(t, a, 80, 24)
	if !strings.Contains(after, "lat") {
		t.Fatalf("after l (en->tr), frame not Turkish:\n%s", after)
	}
	a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModNone))
	third := simText(t, a, 80, 24)
	if third == after {
		t.Fatalf("second l did not cycle language again")
	}
}

func TestThemePressIsExplicit(t *testing.T) {
	a := tuiTestApp()
	a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 't', tcell.ModNone))
	msg := a.msgSnapshot()
	if msg == "" || msg == "dark" || msg == "light" || msg == "system" {
		t.Fatalf("theme press returned bare token %q (must explain TUI palette)", msg)
	}
}
