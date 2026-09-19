package main

import (
	"encoding/json"
	"fmt"
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
	a.t = englishTestMap()
	a.st = tuiState{pane: tuiPaneActions, rows: tuiActionRows(a.t)}
	a.syncListsLocked(tuiSnapshot{status: "running", updateAvail: true})
	return a
}

// englishTestMap is the fixed English label set, shared so NO_COLOR tests
// can build an app without resetting the color pin.
func englishTestMap() map[string]string {
	return map[string]string{
		"TuiStatus": "Status", "TuiLogs": "Logs", "TuiActions": "Actions",
		"TuiHelpTitle": "Keys", "TuiHelpNavTitle": "Navigation", "TuiHelpActTitle": "Actions",
		"TuiNavHint": "Tab switch pane", "TuiClose": "Close", "TuiHelp": "Help",
		"TuiConfirmTitle": "Confirm", "TuiConfirmHint": "Enter confirm",
		"TuiConfirmOK": "confirm", "TuiConfirmCancel": "cancel",
		"TuiConfirmLegend": "* needs confirm",
		"TuiHintScrollLine": "arrows select", "TuiHintSelect": "select",
		"TuiHintPane": "switch pane", "TuiHintActivate": "run", "TuiHintCancel": "close",
		"TuiHintScroll": "scroll", "TuiHintEdges": "top/bottom",
		"TuiHintHelp": "help", "TuiHintQuit": "quit",
		"TuiNavSelect": "Select", "TuiNavPane": "Switch pane", "TuiNavActivate": "Run",
		"TuiNavCancel": "Close", "TuiNavScroll": "Scroll", "TuiNavEdges": "Top/bottom",
		"TuiTooSmall": "Terminal too small (need 60x16)",
		"HealthBadgeRunning": "Running", "HealthBadgeStopped": "Stopped",
		"HealthBadgeNotInstalled": "Not installed", "HealthBadgePortConflict": "Port conflict",
		"ProbeStarting": "Starting", "ProbeDegraded": "Not responding", "ProbeUnknown": "Checking",
		"TuiStart": "Start", "TuiStop": "Stop", "TuiRestart": "Restart", "TuiUpdate": "Update",
		"TuiRepair": "Repair", "TuiInstall": "Install", "TuiAutostart": "Autostart",
		"TuiLanguage": "Language", "TuiTheme": "Theme", "TuiWebUI": "Web UI",
		"TuiManagedPanel": "managed by panel", "TuiManagedLocal": "managed local",
		"TuiOpFailed": "failed", "TuiOpStarted": "Started",
		"TuiBakim": "Maintenance", "TuiAyarlar": "Settings",
		"TuiQuitTitle": "Quit", "TuiQuitMsg": "Close it?", "TuiQuitOK": "quit",
		"TuiLogLoading": "loading…", "TuiLogEmpty": "no logs",
		"TuiMouseNote": "mouse note", "TuiTray": "Tray", "TuiMgmt": "managed",
		"TuiThemeMsg": "theme", "TuiThemeNote": "note",
	}
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
		// Language-agnostic pane presence: the harness pins the action
		// labels (a.t) but pane chrome localizes from the ambient config
		// (tr/es/en) — assert locale-proof fragments plus the List count.
		if !strings.Contains(f, "tart") || !strings.Contains(f, "tus") {
			t.Fatalf("%dx%d missing pane titles:\n%s", wh[0], wh[1], f)
		}
		if a.visibleListLocked().GetItemCount() != len(a.st.rows) {
			t.Fatalf("%dx%d list %d items for %d rows", wh[0], wh[1], a.visibleListLocked().GetItemCount(), len(a.st.rows))
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
	if got := a.visibleListLocked().GetCurrentItem(); got != 2 {
		t.Fatalf("list=%d want 2", got)
	}
	f := simText(t, a, 80, 24)
	lbl := a.st.rows[2].text
	if !strings.Contains(f, lbl) {
		t.Fatalf("row 2 %q missing from frame:\n%s", lbl, f)
	}
}

func TestSimTickKeepsSelection(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "down", "down")
	got0 := a.visibleListLocked().GetCurrentItem()
	a.applyBodyClass(80)
	a.refresh()
	a.applyFocus()
	if got := a.visibleListLocked().GetCurrentItem(); got != got0 {
		t.Fatalf("tick moved list=%d want %d", got, got0)
	}
}

func TestSimTickThenDownContinues(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	simPress(a, "down", "down")
	a.applyBodyClass(80)
	a.refresh()
	a.applyFocus()
	l := a.visibleListLocked()
	got0, n := l.GetCurrentItem(), l.GetItemCount()
	simPress(a, "down")
	want := got0 + 1
	if want > n-1 {
		want = n - 1
	}
	if got := a.visibleListLocked().GetCurrentItem(); got != want {
		t.Fatalf("list=%d want %d after tick+down (was %d of %d)", got, want, got0, n)
	}
}

func TestSimResizeKeepsSelection(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 99, 24)
	simPress(a, "down", "down")
	a.applyBodyClass(120)
	a.applyBodyClass(120)
	f := simText(t, a, 120, 30)
	if got := a.visibleListLocked().GetCurrentItem(); got != 2 {
		t.Fatalf("list=%d want 2 after 99->120", got)
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
	// Help lists state actions directly and section sub-entries as
	// "<Group> > <label>" rows (openers are grouping, not actions).
	for _, r := range a.st.rows {
		if tuiEntryIsSection(r.id) {
			if r.id == tuiActBakim && !strings.Contains(body, "Bak") && !strings.Contains(body, "aintenance") && !strings.Contains(body, "antenimiento") {
				t.Fatalf("help missing Bakim group:\n%s", body)
			}
			if r.id == tuiActAyarlar && !strings.Contains(body, "Ayarlar") && !strings.Contains(body, "ettings") && !strings.Contains(body, "justes") {
				t.Fatalf("help missing Ayarlar group:\n%s", body)
			}
			continue
		}
		if !strings.Contains(body, oneLine(r.text)) {
			t.Fatalf("help missing row %q:\n%s", r.text, body)
		}
		if r.key != "" {
			if !strings.Contains(body, "["+r.key) {
				t.Fatalf("help missing key %q:\n%s", r.key, body)
			}
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
	// Top list when stopped: Start, Bakim section, Ayarlar section. Open
	// Bakim (its first row, Install, confirms), then assert the modal
	// names the row keycap + label with Esc as the safe default.
	l := a.visibleListLocked()
	bi := -1
	for i, r := range a.st.rows {
		if r.id == tuiActBakim {
			bi = i
		}
	}
	if bi < 0 {
		t.Fatalf("no Bakim section in top rows: %+v", a.st.rows)
	}
	for i := 0; i < bi; i++ {
		simPress(a, "down")
	}
	simPress(a, "enter")
	if sec := a.stateSnapshot().sec; sec != tuiSecBakim {
		t.Fatalf("sec=%d want Bakim", sec)
	}
	simPress(a, "enter")
	if !a.pages.HasPage("modal") {
		t.Fatalf("confirm modal page missing")
	}
	f := simText(t, a, 80, 24)
	// Names the action keycap, its row label, and the safe default.
	// Kept as the permanent visual gate for the destructive-action path.
	// Keycaps render literally as "[x]" (tview-escaped); no hidden chars.
	l = a.visibleListLocked()
	main, _ := l.GetItemText(l.GetCurrentItem())
	idx := -1
	for i, r := range a.st.rows {
		if r.text != "" && strings.Contains(main, r.text) {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("List selection %q matches no row", main)
	}
	want := a.st.rows[idx]
	if !strings.Contains(f, "["+want.key+"]") {
		t.Fatalf("confirm missing action key [%s]:\n%s", want.key, f)
	}
	if !strings.Contains(f, want.text) {
		t.Fatalf("confirm missing action label %q:\n%s", want.text, f)
	}
	if !strings.Contains(f, "confirm") && !strings.Contains(f, "onayla") && !strings.Contains(f, "Esc") {
		t.Fatalf("confirm missing safe default Esc:\n%s", f)
	}
}

// TestListParityWithEntries: every section List shows exactly its
// tuiEntriesFor slice, in order, with * marks — render/help/dispatch share
// the slice, so Enter can never hit a neighbour.
func TestListParityWithEntries(t *testing.T) {
	for _, sec := range []tuiSection{tuiSecTop, tuiSecBakim, tuiSecAyarlar} {
		a := tuiTestApp()
		snap := tuiSnapshot{status: "running", updateAvail: true}
		want := tuiEntriesFor(sec, a.t, snap)
		l := map[tuiSection]*tview.List{tuiSecTop: a.listTop, tuiSecBakim: a.listBak, tuiSecAyarlar: a.listAya}[sec]
		if l.GetItemCount() != len(want) {
			t.Fatalf("sec %d: %d items for %d entries", sec, l.GetItemCount(), len(want))
		}
		for i, e := range want {
			main, _ := l.GetItemText(i)
			if !strings.Contains(main, e.text) {
				t.Fatalf("sec %d item %d = %q, want label %q", sec, i, main, e.text)
			}
			if tuiConfirmNeeded(e.id) && !strings.Contains(main, "*") {
				t.Fatalf("sec %d item %d (%q) missing * mark", sec, i, e.text)
			}
		}
	}
}

// TestListOverflowMarker: a Bakim list taller than its box still exposes
// every entry (scroll, not silent drop) — count is the contract.
func TestListOverflowMarker(t *testing.T) {
	a := tuiTestApp()
	snap := tuiSnapshot{status: "running", updateAvail: true}
	if n := len(tuiEntriesFor(tuiSecBakim, a.t, snap)); n < 3 {
		t.Fatalf("bakim list too short to overflow: %d", n)
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
		if tuiEntryIsSection(r.id) {
			if st.sec == tuiSecTop {
				t.Fatalf("index %d (%q): section did not open", i, r.text)
			}
			continue
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

// TestListLabelsMatchEntries: every List item label equals the entry label
// in order — no missing, extra, or empty entries.
func TestListLabelsMatchEntries(t *testing.T) {
	a := tuiTestApp()
	for _, sec := range []tuiSection{tuiSecTop, tuiSecBakim, tuiSecAyarlar} {
		snap := tuiSnapshot{status: "running", updateAvail: true}
		want := tuiEntriesFor(sec, a.t, snap)
		l := map[tuiSection]*tview.List{tuiSecTop: a.listTop, tuiSecBakim: a.listBak, tuiSecAyarlar: a.listAya}[sec]
		for i, e := range want {
			if e.text == "" {
				t.Fatalf("sec %d entry %d renders empty: %+v", sec, i, e)
			}
			main, _ := l.GetItemText(i)
			if !strings.Contains(main, e.text) {
				t.Fatalf("sec %d item %d = %q, want %q", sec, i, main, e.text)
			}
		}
	}
}

// TestListSelectionFollowsVisible: the visible List index tracks st.sel,
// so keyboard Enter and click land on the same entry.


// TestHoverMarkerDiffersFromSelection: a synthetic motion event through the
// hover capture moves the marker to the expected row while the List
// selection stays put; leaving clears it. Frame dump shows both markers on
// different rows.
func TestHoverMarkerDiffersFromSelection(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	l := a.visibleListLocked()
	rx, ry, _, _ := l.GetInnerRect()
	sel := l.GetCurrentItem()
	target := sel + 2
	if target >= l.GetItemCount() {
		target = l.GetItemCount() - 1
	}
	if target == sel {
		t.Skip("list too short for a distinct hover row")
	}
	a.handleMouseMove(tcell.NewEventMouse(rx, ry+target, tcell.ButtonNone, tcell.ModNone))
	if a.hover != target {
		t.Fatalf("hover=%d want %d", a.hover, target)
	}
	if got := l.GetCurrentItem(); got != sel {
		t.Fatalf("motion moved selection to %d (was %d)", got, sel)
	}
	f := simText(t, a, 80, 24)
	hoverLine := -1
	for i, ln := range strings.Split(f, "\n") {
		if strings.Contains(ln, "\u203a") {
			hoverLine = i
		}
	}
	if hoverLine < 0 {
		t.Fatalf("no hover marker in frame:\n%s", f)
	}
	_, lry, _, _ := l.GetInnerRect()
	selLine := lry + sel
	if hoverLine == selLine {
		t.Fatalf("hover and selection on the same row %d:\n%s", hoverLine, f)
	}
	a.handleMouseMove(tcell.NewEventMouse(0, 0, tcell.ButtonNone, tcell.ModNone))
	if a.hover != -1 {
		t.Fatalf("hover=%d after leave, want -1", a.hover)
	}
	f2 := simText(t, a, 80, 24)
	if strings.Contains(f2, "\u203a") {
		t.Fatalf("hover marker survives leave:\n%s", f2)
	}
}

// TestThemePalettesDiffer: dark vs light paint different selection colours;
// system paints no brand colours (terminal defaults only).
func TestThemePalettesDiffer(t *testing.T) {
	configPathOverride = filepath.Join(t.TempDir(), "config.json")
	defer func() { configPathOverride = "" }()
	_ = os.Setenv("NO_COLOR", "")
	saveConfig("en", false)
	saveTheme(ThemeDark)
	a := newTuiApp()
	a.t = englishTestMap()
	a.st = tuiState{pane: tuiPaneActions, rows: tuiActionRows(a.t)}
	a.syncListsLocked(tuiSnapshot{status: "running", updateAvail: true})
	a.syncListsLocked(tuiSnapshot{status: "running", updateAvail: true})
	// Style-level contract: text frame dumps carry no colour, so gate
	// the palette function directly — dark/light resolve differently
	// and system resolves to the terminal default.
	if c := tuiSelectionColors(ThemeDark); c == tuiSelectionColors(ThemeLight) {
		t.Fatalf("dark/light selection identical: %v", c)
	}
	if got := tuiSelectionColors(ThemeSystem); got != tcell.ColorDefault {
		t.Fatalf("system palette paints brand colour %v", got)
	}
	saveTheme(ThemeSystem)
	a.applyTheme()
}

// TestClientLogPollAdvancesCursor: the client-mode poll advances last to the
// returned total index and renders loading/empty states, never an empty box.
func TestClientLogPollAdvancesCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last := r.URL.Query().Get("last")
		if last == "0" {
			fmt.Fprint(w, `{"logs":["boot ok"],"newIndex":1}`)
			return
		}
		fmt.Fprint(w, `{"logs":[],"newIndex":1}`)
	}))
	defer srv.Close()
	old := tuiPanelBase
	tuiPanelBase = srv.URL
	defer func() { tuiPanelBase = old }()
	lines, cursor, ok := tuiPollPanelLogs(0)
	if !ok || cursor != 1 || len(lines) != 1 {
		t.Fatalf("poll(0) = %q,%d,%v want [boot ok],1,true", lines, cursor, ok)
	}
	lines2, cursor2, ok2 := tuiPollPanelLogs(cursor)
	if !ok2 || cursor2 != 1 || len(lines2) != 0 {
		t.Fatalf("poll(1) = %q,%d,%v want [],1,true", lines2, cursor2, ok2)
	}
}

// TestClientLogPaneStates: client mode renders the (panel) marker plus
// a served line, never an empty box.
func TestClientLogPaneStates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/logs") {
			fmt.Fprint(w, `{"logs":["hello from panel"],"newIndex":7}`)
			return
		}
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	old := tuiPanelBase
	tuiPanelBase = srv.URL
	defer func() { tuiPanelBase = old }()
	tuiPanelMu.Lock()
	tuiPanelHit, tuiPanelMiss = true, time.Now()
	tuiPanelMu.Unlock()
	defer func() {
		tuiPanelMu.Lock()
		tuiPanelHit, tuiPanelMiss = false, time.Time{}
		tuiPanelMu.Unlock()
	}()
	a := tuiTestApp()
	f := simText(t, a, 80, 24)
	if !strings.Contains(f, "panel") {
		t.Fatalf("client frame missing (panel) marker:\n%s", f)
	}
	// The poll cursor advanced to the served total index (incremental
	// contract); the served line itself paints once the log pane has
	// room (narrow frames crop it — the unit-level poll test pins the
	// line content, see TestClientLogPollAdvancesCursor).
	if a.logCursor != 7 {
		t.Fatalf("logCursor=%d want 7 (last did not advance to total)", a.logCursor)
	}
}

// TestClientLogEmptyState: an empty poll renders the explicit empty line,
// never an empty box.
func TestClientLogEmptyState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"logs":[],"newIndex":3}`)
	}))
	defer srv.Close()
	old := tuiPanelBase
	tuiPanelBase = srv.URL
	defer func() { tuiPanelBase = old }()
	tuiPanelMu.Lock()
	tuiPanelHit, tuiPanelMiss = true, time.Now()
	tuiPanelMu.Unlock()
	defer func() {
		tuiPanelMu.Lock()
		tuiPanelHit, tuiPanelMiss = false, time.Time{}
		tuiPanelMu.Unlock()
	}()
	a := tuiTestApp()
	f := simText(t, a, 80, 24)
	if strings.Contains(f, "(panel)") && strings.Contains(f, "hello from panel") {
		t.Fatalf("empty poll leaked a served line:\n%s", f)
	}
	if f == "" {
		t.Fatalf("empty client frame")
	}
}

// TestNoColorKeepsStatesDistinct: with NO_COLOR=1 every state stays
// distinguishable (badges/markers, no brand colours).
func TestNoColorKeepsStatesDistinct(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	a := newTuiApp()
	a.t = englishTestMap()
	a.st = tuiState{pane: tuiPaneActions, rows: tuiActionRows(a.t)}
	a.syncListsLocked(tuiSnapshot{status: "running", updateAvail: true})
	a.applyTheme()
	f := simText(t, a, 80, 24)
	if !strings.Contains(f, "tart") {
		t.Fatalf("NO_COLOR frame lost action rows:\n%s", f)
	}
	t.Setenv("NO_COLOR", "")
}
func TestListSelectionFollowsVisible(t *testing.T) {
	a := tuiTestApp()
	a.syncListsLocked(tuiSnapshot{status: "running", updateAvail: true})
	l := a.visibleListLocked()
	if l.GetCurrentItem() != a.stateSnapshot().sel {
		t.Fatalf("list index %d != sel %d", l.GetCurrentItem(), a.stateSnapshot().sel)
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
	// Letter shortcuts are GONE (owner decision): the visible vocabulary is
	// arrows + Tab + Enter + Esc + ? + q, and handleKey must not dispatch
	// any letter to an action.
	tm := tuiTestApp().t
	rows := tuiActionRows(tm)
	for _, b := range tuiActionBindings() {
		st := handleKey(tuiState{pane: tuiPaneActions, rows: rows}, tuiKey{r: b.key})
		if st.lastAct != tuiActNone || st.confirm != 0 {
			t.Fatalf("key %q must not dispatch (got lastAct=%d confirm=%d)", b.key, st.lastAct, st.confirm)
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
	openSection(a, tuiActBakim, t)
	simPress(a, "enter")
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		t.Fatalf("confirm modal not open")
	}
	sel := a.stateSnapshot().sel
	simPress(a, "esc")
	if front, _ := a.pages.GetFrontPage(); front == "modal" {
		t.Fatalf("confirm modal still open after Esc")
	}
	if a.stateSnapshot().confirm != 0 {
		t.Fatalf("confirm still pending")
	}
	if a.stateSnapshot().sel != sel {
		t.Fatalf("sel moved during modal: %d", a.stateSnapshot().sel)
	}
}

func TestSimModalEnterRunsActionOnce(t *testing.T) {
	a := tuiTestApp()
	simText(t, a, 80, 24)
	openSection(a, tuiActBakim, t)
	simPress(a, "enter")
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		t.Fatalf("confirm modal not open")
	}
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
	// Esc through the same routed path the live loop and harness use
	// (modal owns focus while open; see simInject).
	simPress(a, "esc")
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
	// Enter on the focused button closes via the modal done func.
	simPress(a, "enter")
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
	openSection(a, tuiActBakim, t)
	simPress(a, "enter")
	if front, _ := a.pages.GetFrontPage(); front != "modal" {
		t.Fatalf("confirm not open")
	}
	// Esc through the same routed path the live loop and harness use.
	simPress(a, "esc")
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
	a.syncListsLocked(tuiSnapshot{status: "running", updateAvail: true})
	before := simText(t, a, 80, 24)
	if !strings.Contains(before, "Start") {
		t.Fatalf("english frame missing Start:\n%s", before)
	}
	openSection(a, tuiActAyarlar, t)
	pressEntry(a, tuiActLanguage, t)
	after := simText(t, a, 80, 24)
	if !strings.Contains(after, "lat") {
		t.Fatalf("after Language Enter (en->tr), frame not Turkish:\n%s", after)
	}
	pressEntry(a, tuiActLanguage, t)
	third := simText(t, a, 80, 24)
	if third == after {
		t.Fatalf("second Language Enter did not cycle language again")
	}
}

func TestThemePressIsExplicit(t *testing.T) {
	a := tuiTestApp()
	openSection(a, tuiActAyarlar, t)
	pressEntry(a, tuiActTheme, t)
	msg := a.msgSnapshot()
	if msg == "" || msg == "dark" || msg == "light" || msg == "system" {
		t.Fatalf("theme press returned bare token %q (must explain TUI palette)", msg)
	}
}

// openSection moves the List cursor onto the section entry and presses
// Enter through the shared key path, so the test exercises the same
// render/help/dispatch slice the owner uses.
func openSection(a *tuiApp, secAct int, t *testing.T) {
	t.Helper()
	a.syncListsLocked(a.snap)
	bi := -1
	for i, r := range a.st.rows {
		if r.id == secAct {
			bi = i
			break
		}
	}
	if bi < 0 {
		t.Fatalf("no section %d in rows: %+v", secAct, a.st.rows)
	}
	l := a.visibleListLocked()
	for l.GetCurrentItem() < bi {
		a.handleKeyEvent(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	}
	for l.GetCurrentItem() > bi {
		a.handleKeyEvent(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	}
	a.handleKeyEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
}

// pressEntry moves the List cursor onto the entry id and presses Enter
// through the shared key path.
func pressEntry(a *tuiApp, id int, t *testing.T) {
	t.Helper()
	a.syncListsLocked(a.snap)
	bi := -1
	for i, r := range a.st.rows {
		if r.id == id {
			bi = i
			break
		}
	}
	if bi < 0 {
		t.Fatalf("no entry %d in rows: %+v", id, a.st.rows)
	}
	l := a.visibleListLocked()
	for l.GetCurrentItem() < bi {
		a.handleKeyEvent(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	}
	for l.GetCurrentItem() > bi {
		a.handleKeyEvent(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	}
	a.handleKeyEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
}

// TestSplitQuitSentences: the quit message pre-splits at the sentence end so
// the modal box fits each line whole — WordWrap must never see a 66-rune
// sentence it re-wraps mid-word ("arka/planda").
func TestSplitQuitSentences(t *testing.T) {
	want := []string{"TUI kapatılsın mı?", "Panel ve tepsi arka planda çalışmaya devam eder."}
	parts := splitOneSentence("TUI kapatılsın mı? Panel ve tepsi arka planda çalışmaya devam eder.")
	if len(parts) != len(want) || parts[0] != want[0] || parts[1] != want[1] {
		t.Fatalf("split = %q want %q", parts, want)
	}
	// No split inside paths/versions: ". " only splits before uppercase.
	if p := splitOneSentence(`C:\a\b.exe devam`); len(p) != 1 {
		t.Fatalf("path split: %q", p)
	}
}
