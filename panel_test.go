package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestOwnedByOmnirouteTable(t *testing.T) {
	oldPath, oldFn := OmniroutePath, processCommandLineFn
	defer func() { OmniroutePath, processCommandLineFn = oldPath, oldFn }()

	OmniroutePath = `C:\Users\x\AppData\Roaming\npm\node_modules\omniroute`

	tests := []struct {
		name    string
		cmdline string
		ok      bool
		want    bool
	}{
		{
			name:    "entry script",
			cmdline: `node C:\Users\x\AppData\Roaming\npm\node_modules\omniroute\bin\omniroute.mjs --no-open --no-tray`,
			ok:      true,
			want:    true,
		},
		{
			name:    "grep probe",
			cmdline: `grep -r omniroute D:\Projects`,
			ok:      true,
			want:    false,
		},
		{
			name:    "editor notes",
			cmdline: `C:\Program Files\Notepad++\notepad++.exe omniroute-notes.txt`,
			ok:      true,
			want:    false,
		},
		{
			name:    "lookup failure",
			cmdline: "",
			ok:      false,
			want:    false,
		},
		{
			name:    "empty cmdline",
			cmdline: "",
			ok:      true,
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			processCommandLineFn = func(pid int) (string, bool) {
				return tc.cmdline, tc.ok
			}
			if got := ownedByOmniroute(1234); got != tc.want {
				t.Fatalf("ownedByOmniroute(%q)=%v want %v", tc.cmdline, got, tc.want)
			}
		})
	}
}

func TestOwnedByOmnirouteEmptyPath(t *testing.T) {
	oldPath, oldFn := OmniroutePath, processCommandLineFn
	defer func() { OmniroutePath, processCommandLineFn = oldPath, oldFn }()

	OmniroutePath = ""
	processCommandLineFn = func(pid int) (string, bool) {
		return `node C:\x\omniroute\bin\omniroute.mjs`, true
	}
	if ownedByOmniroute(1234) {
		t.Fatal("empty OmniroutePath must refuse attribution")
	}
}

func TestDecideWatchdogAction(t *testing.T) {
	tests := []struct {
		name   string
		state  watchdogState
		want   watchdogAction
	}{
		{name: "healthy no child adopt", state: watchdogState{probe: probeHealthy, installed: true}, want: watchdogAdoptExternal},
		{name: "healthy with child none", state: watchdogState{probe: probeHealthy, childAlive: true, installed: true}, want: watchdogNone},
		{name: "healthy adopted none", state: watchdogState{probe: probeHealthy, installed: true, externalAdopted: true}, want: watchdogNone},
		{name: "down first wait", state: watchdogState{probe: probeDown, failures: 0, installed: true}, want: watchdogWait},
		{name: "down second restart", state: watchdogState{probe: probeDown, failures: 1, installed: true}, want: watchdogRestart},
		{name: "down second own child", state: watchdogState{probe: probeDown, failures: 1, childAlive: true, installed: true}, want: watchdogRestartOwnChild},
		{name: "degraded none", state: watchdogState{probe: probeDegraded, installed: true}, want: watchdogNone},
		{name: "startup down free restart", state: watchdogState{probe: probeDown, installed: true, startupPhase: true}, want: watchdogRestart},
		{name: "startup down busy wait", state: watchdogState{probe: probeDown, installed: true, startupPhase: true, portBusy: true}, want: watchdogWait},
		{name: "startup down child wait", state: watchdogState{probe: probeDown, failures: 0, childAlive: true, installed: true, startupPhase: true}, want: watchdogWait},
		{name: "intentional stop skip", state: watchdogState{probe: probeDown, failures: 5, intentionalStop: true, installed: true}, want: watchdogSkip},
		{name: "op running skip", state: watchdogState{probe: probeDown, failures: 5, opRunning: true, installed: true}, want: watchdogSkip},
		{name: "backoff skip", state: watchdogState{probe: probeDown, failures: 5, backoffActive: true, installed: true}, want: watchdogSkip},
		{name: "not installed skip", state: watchdogState{probe: probeDown, failures: 5}, want: watchdogSkip},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := decideWatchdogAction(tc.state); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestOmniHealthJSONFields(t *testing.T) {
	probeMu.Lock()
	probeStatus = "degraded"
	probeFailures = 1
	externalAdopted = true
	probeRecovering = true
	probeMu.Unlock()
	defer func() {
		probeMu.Lock()
		probeStatus = "unknown"
		probeFailures = 0
		externalAdopted = false
		probeRecovering = false
		probeAt = time.Time{}
		probeMu.Unlock()
	}()
	mux := newPanelMux()
	req := httptest.NewRequest(http.MethodGet, "/api/omni/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var h map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &h); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"probeStatus", "consecutiveFailures", "lastProbeAt", "externallyManaged", "recovering"} {
		if _, ok := h[k]; !ok {
			t.Fatalf("missing field %s", k)
		}
	}
}

func TestApplyProbeTick(t *testing.T) {
	tests := []struct {
		name  string
		prev  probeSnapshot
		state watchdogState
		check func(t *testing.T, r probeTickResult)
	}{
		{
			name:  "healthy own child",
			prev:  probeSnapshot{status: "unknown"},
			state: watchdogState{installed: true, probe: probeHealthy, childAlive: true},
			check: func(t *testing.T, r probeTickResult) {
				if r.snap.status != "healthy" || r.snap.failures != 0 {
					t.Fatalf("snap=%+v", r.snap)
				}
			},
		},
		{
			name:  "healthy clears degraded",
			prev:  probeSnapshot{status: "degraded", degradedShown: true, failures: 1},
			state: watchdogState{installed: true, probe: probeHealthy, childAlive: true},
			check: func(t *testing.T, r probeTickResult) {
				if r.snap.degradedShown || r.snap.status != "healthy" {
					t.Fatalf("snap=%+v", r.snap)
				}
			},
		},
		{
			name:  "degraded no restart",
			prev:  probeSnapshot{status: "healthy"},
			state: watchdogState{installed: true, probe: probeDegraded},
			check: func(t *testing.T, r probeTickResult) {
				if r.action != watchdogNone || r.snap.status != "degraded" || r.snap.failures != 0 || !r.logDegraded {
					t.Fatalf("action=%v snap=%+v log=%v", r.action, r.snap, r.logDegraded)
				}
			},
		},
		{
			name:  "down once",
			prev:  probeSnapshot{status: "healthy"},
			state: watchdogState{installed: true, probe: probeDown, failures: 0},
			check: func(t *testing.T, r probeTickResult) {
				if r.action != watchdogWait || r.snap.failures != 1 || r.snap.status != "unreachable" {
					t.Fatalf("action=%v snap=%+v", r.action, r.snap)
				}
			},
		},
		{
			name:  "down twice recovers",
			prev:  probeSnapshot{status: "unreachable", failures: 1},
			state: watchdogState{installed: true, probe: probeDown, failures: 1},
			check: func(t *testing.T, r probeTickResult) {
				if r.action != watchdogRestart || !r.snap.recovering {
					t.Fatalf("action=%v snap=%+v", r.action, r.snap)
				}
			},
		},
		{
			name:  "healthy after recovery",
			prev:  probeSnapshot{status: "unreachable", recovering: true, failures: 0},
			state: watchdogState{installed: true, probe: probeHealthy, childAlive: true},
			check: func(t *testing.T, r probeTickResult) {
				if r.snap.recovering || r.snap.status != "healthy" {
					t.Fatalf("snap=%+v", r.snap)
				}
			},
		},
		{
			name:  "skip latch resets",
			prev:  probeSnapshot{skipLogged: true, skipReason: "op running"},
			state: watchdogState{installed: true, probe: probeHealthy, childAlive: true},
			check: func(t *testing.T, r probeTickResult) {
				if r.snap.skipLogged || r.snap.skipReason != "" {
					t.Fatalf("snap=%+v", r.snap)
				}
			},
		},
		{
			name:  "healthy after restarts clears counter",
			prev:  probeSnapshot{status: "unreachable", recoveries: 3, recovering: true},
			state: watchdogState{installed: true, probe: probeHealthy, childAlive: true},
			check: func(t *testing.T, r probeTickResult) {
				if r.snap.recoveries != 0 || r.requestBackoff {
					t.Fatalf("snap=%+v backoff=%v", r.snap, r.requestBackoff)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, applyProbeTick(tc.prev, tc.state, 500))
		})
	}
}

func TestApplyProbeTickBackoffEscalation(t *testing.T) {
	st := watchdogState{installed: true, probe: probeDown, failures: 1}
	prev := probeSnapshot{status: "unreachable", failures: 1}
	var r probeTickResult
	for i := 0; i < 10; i++ {
		st.failures = prev.failures
		r = applyProbeTick(prev, st, 0)
		prev = r.snap
		if r.requestBackoff {
			break
		}
	}
	if !r.requestBackoff {
		t.Fatalf("4th restart must request backoff: %+v", r.snap)
	}
	if r.action == watchdogRestart || r.action == watchdogRestartOwnChild {
		t.Fatalf("must not ask for another restart: action=%v", r.action)
	}
	if r.snap.recoveries != 4 {
		t.Fatalf("recoveries=%d want 4", r.snap.recoveries)
	}
}

func TestApplyProbeTickGrace(t *testing.T) {
	prev := probeSnapshot{status: "unknown"}
	st := watchdogState{installed: true, probe: probeDown, failures: 1, childAlive: true, inGrace: true}
	r := applyProbeTick(prev, st, 0)
	if r.action != watchdogSkip || r.snap.status != "starting" || r.snap.failures != 0 {
		t.Fatalf("grace down: action=%v snap=%+v", r.action, r.snap)
	}
	if r.snap.recoveries != 0 || r.requestBackoff {
		t.Fatalf("grace must not count recovery: %+v", r.snap)
	}
	after := watchdogState{installed: true, probe: probeDown, failures: 1, childAlive: true}
	r2 := applyProbeTick(r.snap, after, 0)
	if r2.action != watchdogRestartOwnChild {
		t.Fatalf("post-grace: action=%v", r2.action)
	}
	healthy := watchdogState{installed: true, probe: probeHealthy, childAlive: true, inGrace: true}
	r3 := applyProbeTick(r.snap, healthy, 200)
	if r3.snap.status != "healthy" || r3.snap.inGrace {
		t.Fatalf("healthy clears grace: %+v", r3.snap)
	}
}

func TestHandleChildExit(t *testing.T) {
	old := cmd
	defer func() { cmd = old }()
	cmd = nil
	setIntentionalStop(true)
	done := exec.Command("go", "version")
	if err := done.Start(); err != nil {
		t.Skipf("no process runtime: %v", err)
	}
	if err := done.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	handleChildExit(done)
	if cmd != nil {
		t.Fatal("nil global must stay nil")
	}
	other := exec.Command("go", "version")
	if err := other.Start(); err != nil {
		t.Skipf("no process runtime: %v", err)
	}
	cmd = other
	handleChildExit(done)
	if cmd != other {
		t.Fatal("non-matching global clobbered")
	}
	_ = other.Wait()
	cmd = nil
}

func TestGraceDeadlineRace(t *testing.T) {
	cmdMutex.Lock()
	omniStartGraceUntil = time.Time{}
	cmdMutex.Unlock()
	defer func() {
		cmdMutex.Lock()
		omniStartGraceUntil = time.Time{}
		cmdMutex.Unlock()
	}()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 50 {
				cmdMutex.Lock()
				omniStartGraceUntil = time.Now().Add(time.Duration(n*10+j) * time.Millisecond)
				graceUntil := omniStartGraceUntil
				childAlive := cmd != nil && cmd.Process != nil
				cmdMutex.Unlock()
				inGrace := !graceUntil.IsZero() && time.Now().Before(graceUntil)
				_ = inGrace
				_ = childAlive
			}
		}(i)
	}
	wg.Wait()
}
