package main

import (
	"testing"
	"time"
)

func stubDeps(probe bool, holders []int, spawnErr error, wait bool, spawns *int) trayDeps {
	return trayDeps{
		probe:   func() bool { return probe },
		holders: func(port int) []int { return holders },
		spawnTray: func() error {
			*spawns++
			return spawnErr
		},
		waitPort: func(timeout time.Duration) bool { return wait },
	}
}

// (a) port answers -> zero spawns, client mode.
func TestTrayBootstrapExistingPanel(t *testing.T) {
	var spawns int
	d, n := decideTrayBootstrap(stubDeps(true, nil, nil, false, &spawns), "/x/orpanel.exe")
	if d != trayClientExisting || n != 0 || spawns != 0 {
		t.Fatalf("decision=%v spawns=%d calls=%d, want client-existing/0/0", d, spawns, n)
	}
}

// (b) port silent -> exactly ONE spawn with --tray, then client mode.
func TestTrayBootstrapSpawnsOnce(t *testing.T) {
	var spawns int
	d, n := decideTrayBootstrap(stubDeps(false, nil, nil, true, &spawns), "/x/orpanel.exe")
	if d != trayClientSpawned || n != 1 || spawns != 1 {
		t.Fatalf("decision=%v spawns=%d calls=%d, want client-spawned/1/1", d, spawns, n)
	}
	if argv := traySpawnArgv("/x/orpanel.exe"); len(argv) != 2 || argv[1] != "--tray" {
		t.Fatalf("spawn argv=%q, want [exe --tray]", argv)
	}
}

// (c) spawn ok but port stays silent -> direct mode, honest status.
func TestTrayBootstrapSilentAfterSpawn(t *testing.T) {
	var spawns int
	d, n := decideTrayBootstrap(stubDeps(false, nil, nil, false, &spawns), "/x/orpanel.exe")
	if d != trayDirectNoPanel || n != 1 || spawns != 1 {
		t.Fatalf("decision=%v spawns=%d calls=%d, want direct-no-panel/1/1", d, spawns, n)
	}
}

// (d) foreign holder -> zero spawns, no kill, direct mode.
func TestTrayBootstrapForeignHolder(t *testing.T) {
	old := processImagePathFn
	processImagePathFn = func(pid int) (string, bool) { return `C:\Other\app.exe`, true }
	defer func() { processImagePathFn = old }()
	var spawns int
	d, n := decideTrayBootstrap(stubDeps(false, []int{4242}, nil, false, &spawns), `C:\P\orpanel.exe`)
	if d != trayDirectForeign || n != 0 || spawns != 0 {
		t.Fatalf("decision=%v spawns=%d calls=%d, want direct-foreign/0/0", d, spawns, n)
	}
}

// Routing regression: script seam wins, headless gets the plain summary,
// interactive terminal gets the TUI. Pure decision, no terminal needed.
func TestNoArgRoutingPredicates(t *testing.T) {
	if got := decideNoArgRoute([]string{"q"}, true, true); got != routeScript {
		t.Fatalf("script must route first, got %v", got)
	}
	for _, tc := range []struct {
		in, out bool
		want    noArgRoute
	}{
		{true, true, routeTUI},
		{false, true, routePlain},
		{true, false, routePlain},
		{false, false, routePlain},
	} {
		if got := decideNoArgRoute(nil, tc.in, tc.out); got != tc.want {
			t.Fatalf("in=%v out=%v: got %v want %v", tc.in, tc.out, got, tc.want)
		}
	}
	t.Setenv("ORPANEL_TUI_SCRIPT", "")
	if got := tuiScriptKeys(); got != nil {
		t.Fatalf("empty script env must yield nil, got %q", got)
	}
	t.Setenv("ORPANEL_TUI_SCRIPT", "q")
	if got := tuiScriptKeys(); len(got) != 1 || got[0] != "q" {
		t.Fatalf("script seam must route first, got %q", got)
	}
	t.Setenv("ORPANEL_TUI_SCRIPT", "")
}
