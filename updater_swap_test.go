package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestSwapUniqueNameNeverPreexists pins the destination contract: every
// generated name is unique, so the move can never land on a leftover file.
func TestSwapUniqueNameNeverPreexists(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		n := swapOldName(`C:\x\orPanel.exe`)
		if seen[n] {
			t.Fatalf("duplicate sidecar name %q", n)
		}
		seen[n] = true
		if !strings.HasPrefix(filepath.Base(n), "orPanel.exe.old.") {
			t.Fatalf("unexpected sidecar shape %q", n)
		}
	}
}

// TestSwapBatchUsesPingBackoff pins the retry mechanics: no `timeout /t`
// (exits immediately with redirected stdin), `ping -n 3` waits instead,
// and the move target is the unique sidecar, never "<exe>.old".
func TestSwapBatchUsesPingBackoff(t *testing.T) {
	bat := buildWindowsSwapBatch(`C:\u\apply_update.log`, `C:\p\orPanel.exe`, `C:\p\orPanel.exe.old.20240101-000000.1`, `C:\u\orPanel.exe`, `rem noop`)
	if strings.Contains(bat, "timeout /t") {
		t.Fatalf("batch still uses timeout /t (fake backoff on hidden launch)")
	}
	if strings.Count(bat, "ping -n 3 127.0.0.1 >nul") < 4 {
		t.Fatalf("expected ping backoff in all retry loops")
	}
	if strings.Contains(bat, `"C:\p\orPanel.exe.old"`) {
		t.Fatalf("batch still moves onto the fixed .old name")
	}
	if !strings.Contains(bat, `orPanel.exe.old.20240101-000000.1`) {
		t.Fatalf("batch does not use the unique sidecar name")
	}
}

// TestSwapLockHintHelpers covers the CLI failure-reporting helpers.
func TestSwapLockHintHelpers(t *testing.T) {
	if !looksLocked("Erişim engellendi.\n0 file(s) moved.") {
		t.Fatalf("Turkish lock text not detected")
	}
	if !looksLocked("FAILED move after 10 attempts") {
		t.Fatalf("move failure not detected")
	}
	if looksLocked("success: swapped, cleaning up") {
		t.Fatalf("success misclassified as locked")
	}
	tail := lastLines("a\n\nb\nc\nd\ne\n", 3)
	if tail != "c\nd\ne" {
		t.Fatalf("tail = %q", tail)
	}
}

// TestWindowsSwapOwnerTrap reproduces the owner's exact state in a temp
// sandbox (never the real install dir, never a real panel process):
//
//	sandbox\app.exe running  +  sandbox\app.exe.old holding its image
//	(a previous swap renamed the live image aside)  +  fresh app.exe staged.
//
// The OLD fixed-name scheme must fail (pins the defect); the NEW unique-name
// scheme must swap successfully with the tray-equivalent process still
// running. Verified failing pre-fix (fixed move -> "Access is denied"),
// passing post-fix. Windows only; skipped elsewhere.
func TestWindowsSwapOwnerTrap(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows mapped-image semantics only")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	app := filepath.Join(dir, "app.exe")
	oldFixed := app + ".old"
	staged := filepath.Join(dir, "staged.exe")
	logPath := filepath.Join(dir, "apply_update.log")
	if err := copyFile(self, app); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(self, staged); err != nil {
		t.Fatal(err)
	}
	// Sleeper: re-exec this test binary gated to a helper that only sleeps,
	// so the child sits mapped on app.exe with no suite running inside it.
	sleeper := exec.Command(app, "-test.run=TestHelperSwapSleeper")
	sleeper.Env = append(os.Environ(), "ORPANEL_SWAP_SLEEPER=1")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	// stopSleeper kills the child AND waits for the OS to release its
	// mapped image. Kill alone is async: without the wait, TempDir
	// removal races the teardown and fails with "Erişim engellendi."
	stopSleeper := func() {
		if sleeper.Process == nil {
			return
		}
		_ = sleeper.Process.Kill()
		done := make(chan struct{})
		go func() {
			_, _ = sleeper.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
	}
	// Runs BEFORE t.TempDir's own RemoveAll (LIFO): child is reaped and
	// the dir is removed tolerantly, so the swap assertions stay the
	// thing under test even on failure paths.
	t.Cleanup(func() {
		stopSleeper()
		var err error
		for i := 0; i < 10; i++ {
			if err = os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Errorf("sandbox %s still locked after sleeper exit (holder pid %d): %v", dir, sleeper.Process.Pid, err)
	})
	// Deterministic gate (no fixed sleep): the sleeper proves its own image
	// is mapped (open-for-write must fail) and only then writes dir-ready.
	// Poll for that file; the OLD-scheme move below is only valid once the
	// child is truly mapped, otherwise Defender/cold-loader delay turns the
	// trap into a false red ("trap invalid").
	ready := filepath.Join(dir, "sleeper.ready")
	readyDeadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(readyDeadline) {
			alive := sleeper.ProcessState == nil || !sleeper.ProcessState.Exited()
			t.Fatalf("sleeper never signalled mapped image (ready file %s missing; child pid %d alive=%v)", ready, sleeper.Process.Pid, alive)
		}
		time.Sleep(100 * time.Millisecond)
	}
	// app.exe.old (allowed), then a fresh app.exe arrived (copy).
	if out, err := exec.Command("cmd", "/c", "move", "/Y", app, oldFixed).CombinedOutput(); err != nil {
		t.Fatalf("setup move aside failed (sandbox broken): %v %s", err, out)
	}
	if err := copyFile(staged, app); err != nil {
		t.Fatal(err)
	}
	// OLD scheme: move running app.exe onto the mapped app.exe.old.
	out, err := exec.Command("cmd", "/c", "move", "/Y", app, oldFixed).CombinedOutput()
	if err == nil {
		t.Fatalf("OLD fixed-name move unexpectedly succeeded; trap invalid: %s", out)
	}
	// NEW scheme: the generated batch moves to a unique name, then copies.
	oldUnique := swapOldName(app)
	if _, err := os.Stat(oldUnique); !os.IsNotExist(err) {
		t.Fatalf("unique sidecar pre-exists: %s", oldUnique)
	}
	bat := buildWindowsSwapBatch(logPath, app, oldUnique, staged, "rem noop-swap-test")
	batPath := filepath.Join(dir, "apply_update.bat")
	if err := os.WriteFile(batPath, []byte(bat), 0644); err != nil {
		t.Fatal(err)
	}
	run := exec.Command("cmd", "/c", batPath)
	if out, err := run.CombinedOutput(); err != nil {
		data, _ := os.ReadFile(logPath)
		t.Fatalf("NEW scheme batch failed: %v %s\nlog:\n%s", err, out, data)
	}
	if _, err := os.Stat(app); err != nil {
		t.Fatalf("app.exe missing after swap: %v", err)
	}
	if _, err := os.Stat(oldUnique); err != nil {
		t.Fatalf("unique sidecar missing after swap: %v", err)
	}
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(strings.ToLower(string(data)), "success") {
		t.Fatalf("batch log lacks success marker:\n%s", data)
	}
	// Backoff really waits: ping -n 3 costs ~2s per wait, and a clean swap
	// performs exactly one (the initial wait). The owner's broken run
	// compressed 10 attempts into 0.1s; anything >= ~1.5s proves the wait
	// is real, not a timeout-style spin.
	stamps := logTimestamps(t, string(data))
	if len(stamps) >= 2 && stamps[len(stamps)-1].Sub(stamps[0]) < 1500*time.Millisecond {
		t.Fatalf("backoff did not wait (span %v); timeout-style spin?", stamps[len(stamps)-1].Sub(stamps[0]))
	}
	// No explicit kill here: t.Cleanup's stopSleeper (kill + Wait +
	// tolerant RemoveAll) owns teardown on every path, including failures.
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0644)
}

func lockProbePath(p string) string { return p }

func logTimestamps(t *testing.T, log string) []time.Time {
	t.Helper()
	var out []time.Time
	for _, line := range strings.Split(log, "\n") {
		// Batch %date% %time% format is locale-dependent; accept HH:MM:SS.cc
		// fragments anywhere in the line.
		for _, tok := range strings.Fields(line) {
			if len(tok) >= 8 && tok[2] == ':' && tok[5] == ':' {
				if ts, err := time.Parse("15:04:05", tok[:8]); err == nil {
					out = append(out, ts)
					break
				}
			}
		}
	}
	return out
}

// TestHelperSwapSleeper is not a real test: the trap re-execs the test
// binary gated to this name so the child sits mapped on app.exe running
// NO suite code. It proves its own image is mapped — opening os.Executable
// for write MUST fail while the loader holds it — and only then writes
// sleeper.ready next to itself. The parent polls for that file instead of
// sleeping a fixed 500ms (Defender/cold-loader delay made the OLD-scheme
// move succeed spuriously => false "trap invalid" red).
func TestHelperSwapSleeper(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	for i := 0; i < 100; i++ {
		f, err := os.OpenFile(exe, os.O_WRONLY, 0)
		if err != nil {
			_ = os.WriteFile(filepath.Join(filepath.Dir(exe), "sleeper.ready"), []byte("mapped"), 0644)
			break
		}
		_ = f.Close()
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(60 * time.Second)
}
