package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func tuiTestMap() map[string]string {
	return map[string]string{
		"TuiStatus": "Status", "TuiLogs": "Logs", "TuiActions": "Actions",
		"TuiHelpTitle": "Keys", "TuiPressQ": "quit",
	}
}

func tuiTestSnap() tuiSnapshot {
	return tuiSnapshot{
		appVer: "1.0", omniVer: "3.0", lang: "en", theme: "dark",
		status: "running", probe: "healthy", port: "20128", nodeVer: "v24",
		logs: []string{"[2026-01-01 00:00:00] INFO: hello world, a fairly long log line for truncation"},
	}
}

func tuiLineWidths(t *testing.T, frame string, width int) {
	t.Helper()
	for i, ln := range strings.Split(frame, "\n") {
		if visibleLen(ln) != width {
			t.Fatalf("line %d visible=%d want %d: %q", i, visibleLen(ln), width, ln)
		}
		if len(ln) == 0 {
			t.Fatalf("line %d empty", i)
		}
	}
}

func TestComposeFrameSizes(t *testing.T) {
	tm := tuiTestMap()
	for _, w := range []int{40, 80, 120} {
		for _, h := range []int{10, 24, 40} {
			st := tuiState{pane: tuiPaneActions, rows: tuiActionRows(tm)}
			f := composeFrame(st, tuiTestSnap(), tm, w, h, time.Now())
			lines := strings.Split(f, "\n")
			if len(lines) != h {
				t.Fatalf("w=%d h=%d lines=%d", w, h, len(lines))
			}
			tuiLineWidths(t, f, w)
			if !strings.Contains(f, "Status") || !strings.Contains(f, "Logs") || !strings.Contains(f, "Actions") {
				t.Fatalf("w=%d h=%d missing sections", w, h)
			}
		}
	}
}

func TestComposeFrameTruncates(t *testing.T) {
	tm := tuiTestMap()
	st := tuiState{pane: tuiPaneActions, rows: tuiActionRows(tm)}
	f := composeFrame(st, tuiTestSnap(), tm, 40, 24, time.Now())
	if !strings.Contains(f, "…") {
		t.Fatalf("expected ellipsis truncation at width 40:\n%s", f)
	}
	for _, ln := range strings.Split(f, "\n") {
		if visibleLen(ln) > 40 {
			t.Fatalf("overflow: %q", ln)
		}
	}
}

func TestHandleKeyTransitions(t *testing.T) {
	tm := tuiTestMap()
	rows := tuiActionRows(tm)
	mk := func() tuiState { return tuiState{pane: tuiPaneActions, rows: rows} }
	st := handleKey(mk(), tuiKeyRune('j'))
	if st.sel != 1 || st.lastAct != tuiActNone {
		t.Fatalf("j: %+v", st)
	}
	st = handleKey(mk(), tuiKeyRune('k'))
	if st.sel != 0 {
		t.Fatalf("k clamp: %+v", st)
	}
	st = mk()
	st.sel = len(rows) - 1
	st = handleKey(st, tuiKeyRune('j'))
	if st.sel != len(rows)-1 {
		t.Fatalf("j clamp end: %+v", st)
	}
	st = mk()
	st.sel = 2
	st = handleKey(st, tuiKey{r: '\r'})
	if st.lastAct != rows[2].id {
		t.Fatalf("enter: %+v", st.lastAct)
	}
	if got := handleKey(mk(), tuiKeyRune('q')); !got.quit || got.lastAct != tuiActQuit {
		t.Fatalf("q: %+v", got)
	}
	if got := handleKey(mk(), tuiKey{raw: "\x03"}); !got.quit {
		t.Fatalf("ctrl-c: %+v", got)
	}
	if got := handleKey(mk(), tuiKeyRune('z')); got.lastAct != tuiActNone || got.quit {
		t.Fatalf("unknown: %+v", got)
	}
	a := handleKey(mk(), tuiKeyRune('?'))
	if !a.showHelp || a.lastAct != tuiActHelp {
		t.Fatalf("help on: %+v", a)
	}
	b := handleKey(a, tuiKeyRune('?'))
	if b.showHelp {
		t.Fatalf("help off: %+v", b)
	}
	c := handleKey(mk(), tuiKey{r: '\t'})
	if c.pane != tuiPaneLogs || c.lastAct != tuiActPaneNext {
		t.Fatalf("tab: %+v", c)
	}
	d := handleKey(tuiState{pane: tuiPaneLogs}, tuiKeyRune('j'))
	if d.logOff != 1 || d.lastAct != tuiActScrollDown {
		t.Fatalf("log scroll: %+v", d)
	}
	if got := handleKey(mk(), tuiKeyRune('s')); got.lastAct != tuiActStart {
		t.Fatalf("s: %+v", got)
	}
}

func TestPlainSummaryNoESC(t *testing.T) {
	out := plainSummaryText()
	if strings.Contains(out, "\x1b") {
		t.Fatalf("ESC in plain summary: %q", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines=%d: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "orpanel v") || !strings.Contains(lines[2], "omniroute status=") {
		t.Fatalf("summary: %q", out)
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
