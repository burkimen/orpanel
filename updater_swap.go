package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// updater_swap.go: Windows self-update swap mechanics shared by the web-UI
// path (performUpdate) and the CLI path (launchWindowsSwap).
//
// Forensics (v1.4.0 live failure): the batch moved the running exe onto a
// FIXED "<exe>.old" name. A previous swap had left that file behind and it
// was still mapped as a live process image, so every move failed with
// "Erişim engellendi." Moving a running image ASIDE to a fresh name always
// succeeds — only the fixed destination was the defect.

// swapOldName returns a unique sidecar path for the running exe. The
// destination never pre-exists, so the move cannot collide with a leftover
// file that is still mapped by a live process. Uniqueness comes from the
// timestamp plus pid plus an atomic counter (same-second swaps in tests).
var swapOldCounter uint64

func swapOldName(exe string) string {
	n := atomic.AddUint64(&swapOldCounter, 1)
	return fmt.Sprintf("%s.old.%s.%d.%d", exe, time.Now().Format("20060102-150405"), os.Getpid(), n)
}

// buildWindowsSwapBatch renders apply_update.bat. oldName must come from
// swapOldName. Backoff uses `ping -n 3` (~2s) because `timeout /t` returns
// immediately when stdin is redirected (hidden launch), making retries spin
// with zero wait. relaunch is the post-swap line (production:
// `start "" "<exe>" --tray`); tests pass a no-op so no process is started.
func buildWindowsSwapBatch(applyLog, exe, oldName, newExe, relaunch string) string {
	return fmt.Sprintf(`@echo off
set LOG="%s"
echo [%%date%% %%time%%] waiting for old process to exit >> %%LOG%%
ping -n 3 127.0.0.1 >nul
set M=0
:moveretry
set /a M+=1
echo [%%date%% %%time%%] move attempt %%M%%: move old aside to "%s" >> %%LOG%%
move /Y "%s" "%s" >> %%LOG%% 2>&1
if errorlevel 1 (
  echo [%%date%% %%time%%] move attempt %%M%% failed >> %%LOG%%
  if %%M%% GEQ 10 goto :failmove
  ping -n 3 127.0.0.1 >nul
  goto :moveretry
)
set C=0
:copyretry
set /a C+=1
echo [%%date%% %%time%%] copy attempt %%C%%: copy new into place >> %%LOG%%
copy /Y "%s" "%s" >> %%LOG%% 2>&1
if errorlevel 1 (
  echo [%%date%% %%time%%] copy attempt %%C%% failed >> %%LOG%%
  if %%C%% GEQ 10 goto :rollback
  ping -n 3 127.0.0.1 >nul
  goto :copyretry
)
echo [%%date%% %%time%%] success: swapped, cleaning up >> %%LOG%%
del "%s" >> %%LOG%% 2>&1
%s
exit /b 0
:rollback
echo [%%date%% %%time%%] copy failed after move: rolling back old binary >> %%LOG%%
set R=0
:rbretry
set /a R+=1
move /Y "%s" "%s" >> %%LOG%% 2>&1
if errorlevel 1 (
  echo [%%date%% %%time%%] rollback attempt %%R%% failed >> %%LOG%%
  if %%R%% GEQ 5 goto :failcopy
  ping -n 3 127.0.0.1 >nul
  goto :rbretry
)
echo [%%date%% %%time%%] rolled back: old binary restored, update NOT applied >> %%LOG%%
%s
exit /b 1
:failmove
echo [%%date%% %%time%%] FAILED move after 10 attempts, exe untouched >> %%LOG%%
exit /b 1
:failcopy
echo [%%date%% %%time%%] FAILED copy and rollback after retries, manual repair needed >> %%LOG%%
exit /b 1
`, applyLog, oldName, exe, oldName, newExe, exe, newExe, relaunch, oldName, exe, relaunch)
}

// bestEffortStopSiblings tries to stop other instances of our own binary
// that run from the same install path, then waits briefly for them to exit.
// Best effort only: every error is swallowed and the swap must succeed even
// when siblings cannot be stopped (e.g. elevated process). Windows only;
// no-op elsewhere.
func bestEffortStopSiblings(exe string) {
	if runtime.GOOS != "windows" {
		return
	}
	me := os.Getpid()
	base := filepath.Base(exe)
	low := strings.ToLower(exe)
	stop := fmt.Sprintf(`Get-CimInstance Win32_Process -Filter "Name='%s'" | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.ToLower() -eq '%s' -and $_.ProcessId -ne %d } | ForEach-Object { try { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue } catch {} }`,
		strings.ReplaceAll(base, "'", "''"), strings.ReplaceAll(low, "'", "''"), me)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", stop)
	hideWindow(c)
	_ = c.Run()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !siblingInstallInstanceAlive(exe, me) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// siblingInstallInstanceAlive reports whether any process besides me runs
// the given exe path. Fail-open (false) so the swap never blocks on it.
func siblingInstallInstanceAlive(exe string, me int) bool {
	base := filepath.Base(exe)
	low := strings.ToLower(exe)
	ps := fmt.Sprintf(`@(Get-CimInstance Win32_Process -Filter "Name='%s'" | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.ToLower() -eq '%s' -and $_.ProcessId -ne %d }).Count`,
		strings.ReplaceAll(base, "'", "''"), strings.ReplaceAll(low, "'", "''"), me)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	hideWindow(c)
	out, err := c.Output()
	if err != nil {
		return false
	}
	n := strings.TrimSpace(string(out))
	return n != "" && n != "0"
}

// lastLines returns the last n non-blank lines of s (batch-log tail).
func lastLines(s string, n int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return strings.Join(out, "\n")
}

// looksLocked reports whether batch-log text points at a locked/mapped file
// (the tray instance still running while the swap ran).
func looksLocked(s string) bool {
	l := strings.ToLower(s)
	for _, m := range []string{
		"engellendi",
		"access is denied",
		"being used by another process",
		"the process cannot access the file",
		"0 file(s) moved",
		"failed move",
	} {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}
