package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)
//  1. probe 127.0.0.1:PanelPort with a short timeout — answers: client mode.
//  2. silent: check ownership — foreign holder: direct mode, never spawn/kill.
//  3. our port free (or only our own stale holder): spawnDetached-equivalent
//     (--tray, hidden window), wait bounded for the port, then client mode.
//  4. port still silent: direct mode with the honest status line.
//
// Quitting the TUI never stops the tray/panel: there is no cleanup path
// that kills it. Idempotent: never a second spawn while one answers.

// trayDeps is the injectable seam: probe answers?, holder pids, spawn, wait.
type trayDeps struct {
	probe     func() bool
	holders   func(port int) []int
	spawnTray func() error
	waitPort  func(timeout time.Duration) bool
}

type trayDecision int

const (
	trayClientExisting trayDecision = iota // panel answered: attach, no spawn
	trayClientSpawned                      // spawned, port answered: attach
	trayDirectForeign                      // foreign holder: direct, no spawn/kill
	trayDirectNoPanel                      // spawn done (or nothing to spawn), still silent: direct
)

// decideTrayBootstrap is pure given deps: no network/process calls inside
// except through the seam, so unit tests launch nothing.
func decideTrayBootstrap(d trayDeps, exe string) (trayDecision, int) {
	if d.probe() {
		return trayClientExisting, 0
	}
	var spawns int
	for _, pid := range d.holders(PanelPort) {
		if !isOwnPanelProcess(pid, exe) {
			return trayDirectForeign, spawns
		}
	}
	if err := d.spawnTray(); err != nil {
		return trayDirectNoPanel, spawns
	}
	spawns = 1
	if d.waitPort(8 * time.Second) {
		return trayClientSpawned, spawns
	}
	return trayDirectNoPanel, spawns
}

// Production seam wiring: short-timeout probe of our own status endpoint,
// ownership via the reclaim path's pid collection, spawn via the same
// detached --tray helper --tray uses, bounded wait on the port.
func prodTrayDeps() trayDeps {
	return trayDeps{
		probe: func() bool {
			client := &http.Client{Timeout: 800 * time.Millisecond}
			resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(PanelPort) + "/api/status")
			if err != nil || resp == nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode < 500
		},
		holders: func(port int) []int { return collectPortPidsFn(port) },
		spawnTray: func() error {
			exe, _ := os.Executable()
			if exe == "" {
				exe = filepath.Base(os.Args[0])
			}
			return spawnTrayDetached(exe)
		},
		waitPort: func(timeout time.Duration) bool {
			deadline := time.Now().Add(timeout)
			for time.Now().Before(deadline) {
				if isPortInUse(PanelPort) {
					// Answer, not just bound: a foreign binder is not ours.
					client := &http.Client{Timeout: 500 * time.Millisecond}
					resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(PanelPort) + "/api/status")
					if err == nil && resp != nil {
						resp.Body.Close()
						if resp.StatusCode < 500 {
							return true
						}
					}
				}
				time.Sleep(250 * time.Millisecond)
			}
			return false
		},
	}
}

// ensureTrayForTUI runs the bootstrap and reports whether the panel answered
// (client mode) or the TUI runs direct. Never kills anything.
func ensureTrayForTUI() bool {
	exe, _ := os.Executable()
	decision, _ := decideTrayBootstrap(prodTrayDeps(), exe)
	switch decision {
	case trayClientExisting, trayClientSpawned:
		return true
	default:
		return false
	}
}

// noArgRoute is the pure routing decision for the no-argument path, so the
// TUI-vs-summary branch is unit-testable without a terminal.
type noArgRoute int

const (
	routeScript noArgRoute = iota
	routePlain
	routeTUI
)

func decideNoArgRoute(script []string, termIn, termOut bool) noArgRoute {
	if script != nil {
		return routeScript
	}
	if !termIn || !termOut {
		return routePlain
	}
	return routeTUI
}

// noArgLaunch routes the no-argument path: scripted frames (test seam),
// plain summary when headless, otherwise ensure the tray/panel then the TUI.
// Quitting the TUI leaves the tray running.
func noArgLaunch() {
	switch decideNoArgRoute(tuiScriptKeys(), isTerminal(), isTerminalOut()) {
	case routeScript:
		runTuiScript(tuiScriptKeys())
		return
	case routePlain:
		printPlainSummary()
		return
	default:
		_ = ensureTrayForTUI()
		runTUI()
	}
}

// spawnTrayDetached launches exe --tray detached with the window hidden.
// Same flags --tray uses today; factored so tests can assert the argv.
func spawnTrayDetached(exe string) error {
	cmd := exec.Command(exe, "--tray")
	cmd.SysProcAttr = relaunchAttrs()
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

// traySpawnArgv is the exact argv the bootstrap spawns; tests assert it.
func traySpawnArgv(exe string) []string { return []string{exe, "--tray"} }

