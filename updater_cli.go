package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)
const (
	updateExitOK       = 0
	updateExitFail     = 1
	updateExitRollback = 2
)

// isTerminalOut reports whether stdout is a terminal (in-place progress)
// or redirected (plain periodic lines, no ANSI/CR tricks).
func isTerminalOut() bool {
	st, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}

// renderProgressLine renders one download progress line. downloaded and
// total are bytes, elapsed the time since download start. Total <= 0 means
// unknown: spinner + bytes + speed, never a percentage. Zero elapsed yields
// zero speed (no divide by zero).
func renderProgressLine(downloaded, total int64, elapsed time.Duration) string {
	mb := func(b int64) string { return fmt.Sprintf("%.1f", float64(b)/(1024*1024)) }
	secs := elapsed.Seconds()
	var speed float64
	if secs > 0 && downloaded > 0 {
		speed = float64(downloaded) / (1024 * 1024) / secs
	}
	speedStr := fmt.Sprintf("%.1f MB/s", speed)
	if total <= 0 {
		frames := []string{"|", "/", "-", "\\"}
		idx := 0
		if secs > 0 {
			idx = int(elapsed/time.Second) % len(frames)
		}
		return fmt.Sprintf("%s %s MB (%s)", frames[idx], mb(downloaded), speedStr)
	}
	pct := 0.0
	if total > 0 && downloaded >= 0 {
		pct = 100 * float64(downloaded) / float64(total)
	}
	if pct < 0 {
		pct = 0
	}
	return fmt.Sprintf("%5.1f%% %s/%s MB (%s)", pct, mb(downloaded), mb(total), speedStr)
}

// progressWriter prints download progress either in place (terminal) or as
// periodic plain lines (redirected). It never writes to panel.log.
type progressWriter struct {
	total      int64
	start      time.Time
	lastPrint  time.Time
	lastPct    int
	terminal   bool
	label      string
}

func newProgressWriter(total int64, terminal bool, label string) *progressWriter {
	return &progressWriter{total: total, start: time.Now(), terminal: terminal, label: label, lastPct: -1}
}

func (p *progressWriter) update(downloaded int64) {
	now := time.Now()
	line := renderProgressLine(downloaded, p.total, now.Sub(p.start))
	if p.terminal {
		fmt.Printf("\r%s %s", p.label, line)
		return
	}
	pct := -1
	if p.total > 0 {
		pct = int(100 * float64(downloaded) / float64(p.total))
	}
	if pct >= 0 && (pct-p.lastPct >= 10 || now.Sub(p.lastPrint) >= 2*time.Second) {
		p.lastPct = pct
		p.lastPrint = now
		fmt.Printf("%s %s\n", p.label, line)
	} else if pct < 0 && now.Sub(p.lastPrint) >= 2*time.Second {
		p.lastPrint = now
		fmt.Printf("%s %s\n", p.label, line)
	}
}

func (p *progressWriter) done() {
	if p.terminal {
		fmt.Print("\n")
	}
}

// downloadToFileProgress mirrors downloadToFile but reports progress.
// totalHint <= 0 falls back to Content-Length, else unknown-total mode.
func downloadToFileProgress(url, destPath string, totalHint int64, pw *progressWriter) (int64, error) {
	resp, err := githubGet(url, 120*time.Second)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	total := totalHint
	if total <= 0 && resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	if pw != nil && total > 0 {
		pw.total = total
	}
	outFile, err := os.Create(destPath)
	if err != nil {
		return 0, err
	}
	var written int64
	buf := make([]byte, 128*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := outFile.Write(buf[:n]); werr != nil {
				outFile.Close()
				os.Remove(destPath)
				return written, werr
			}
			written += int64(n)
			if written > maxDownloadBytes+1 {
				outFile.Close()
				os.Remove(destPath)
				return written, fmt.Errorf("download exceeds %d MiB limit", maxDownloadBytes>>20)
			}
			if pw != nil {
				pw.update(written)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			outFile.Close()
			os.Remove(destPath)
			return written, rerr
		}
	}
	outFile.Close()
	if written > maxDownloadBytes {
		os.Remove(destPath)
		return written, fmt.Errorf("download exceeds %d MiB limit", maxDownloadBytes>>20)
	}
	if pw != nil {
		pw.update(written)
		pw.done()
	}
	return written, nil
}

// updateOutcome maps batch-log content to a CLI outcome and exit code.
type updateOutcome struct {
	code    int
	message string
}

// parseBatchOutcome classifies apply_update.log text. success wins over
// rollback (a stale line from a prior run must not mask a fresh success).
func parseBatchOutcome(logText string) updateOutcome {
	lower := strings.ToLower(logText)
	if strings.Contains(lower, "rolled back") {
		return updateOutcome{code: updateExitRollback, message: "rolled back"}
	}
	if strings.Contains(lower, "success") {
		return updateOutcome{code: updateExitOK, message: "success"}
	}
	if strings.Contains(lower, "failed") {
		return updateOutcome{code: updateExitFail, message: "failed"}
	}
	return updateOutcome{code: updateExitFail, message: "unknown"}
}

// runCLIUpdate implements `orpanel --update`: preflight, staged progress,
// checksum, swap, bounded wait for the truth, locale-aware outcome lines.
// Web-UI path (performUpdate) is untouched.
func runCLIUpdate() int {
	t := loadTranslations(getCurrentLang())
	full := getLatestReleaseFull()
	if full.err != nil {
		fmt.Printf("%s: %v\n", t["SettingUpdateFailed"], full.err)
		return updateExitFail
	}
	cur := getCurrentVersion()
	lat := full.version
	if !isVersionLess(cur, lat) {
		fmt.Printf("%s: v%s\n", t["SettingUpToDate"], cur)
		return updateExitOK
	}
	assetName := getAssetName()
	size := full.sizes[assetName]
	sizeStr := "?"
	if size > 0 {
		sizeStr = fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
	fmt.Printf("%s: v%s -> v%s (%s, %s)\n", t["SettingUpdate"], cur, lat, assetName, sizeStr)
	sumsURL, ok := full.assets["sha256sums.txt"]
	if !ok {
		fmt.Printf("%s\n", t["SettingUpdateFailed"])
		return updateExitFail
	}
	downloadURL, ok := full.assets[assetName]
	if !ok {
		fmt.Printf("%s\n", t["SettingUpdateFailed"])
		return updateExitFail
	}
	exe, _ := os.Executable()
	updateDir := getUpdateDir()
	os.MkdirAll(updateDir, 0755)
	newExe := filepath.Join(updateDir, filepath.Base(exe))
	term := isTerminalOut()
	pw := newProgressWriter(size, term, t["UpdatePhaseDownloading"])
	written, err := downloadToFileProgress(downloadURL, newExe, size, pw)
	if err != nil {
		os.Remove(newExe)
		fmt.Printf("%s: %v\n", t["SettingUpdateFailed"], err)
		return updateExitFail
	}
	fmt.Printf("%s (%.1f MB)\n", t["UpdatePhaseVerifying"], float64(written)/(1024*1024))
	sumsPath := filepath.Join(updateDir, "sha256sums.txt")
	if _, err := downloadToFile(sumsURL, sumsPath); err != nil {
		os.Remove(newExe)
		os.Remove(sumsPath)
		fmt.Printf("%s: %v\n", t["SettingUpdateFailed"], err)
		return updateExitFail
	}
	sumsBytes, err := os.ReadFile(sumsPath)
	os.Remove(sumsPath)
	if err != nil {
		os.Remove(newExe)
		fmt.Printf("%s: %v\n", t["SettingUpdateFailed"], err)
		return updateExitFail
	}
	if err := verifyChecksum(string(sumsBytes), assetName, newExe); err != nil {
		os.Remove(newExe)
		fmt.Printf("%s: %v\n", t["SettingUpdateFailed"], err)
		return updateExitFail
	}
	fmt.Printf("%s\n", t["UpdatePhaseVerifying"]+" OK")
	fmt.Printf("%s\n", t["UpdatePhaseApplying"])
	if runtime.GOOS != "windows" {
		if err := os.Rename(newExe, exe); err != nil {
			os.Remove(newExe)
			fmt.Printf("%s: %v\n", t["SettingUpdateFailed"], err)
			return updateExitFail
		}
		cmd := exec.Command(exe, "--tray")
		cmd.SysProcAttr = relaunchAttrs()
		cmd.Start()
		fmt.Printf("%s: v%s -> v%s\n", t["SettingUpdateDone"], cur, lat)
		return updateExitOK
	}
	applyLog := filepath.Join(updateDir, "apply_update.log")
	os.Remove(applyLog)
	before, _ := fileHash(newExe)
	if err := launchWindowsSwap(exe, newExe, updateDir); err != nil {
		fmt.Printf("%s: %v\n", t["SettingUpdateFailed"], err)
		return updateExitFail
	}
	code := waitForSwapOutcome(exe, newExe, before, applyLog, lat)
	if code == updateExitOK {
		fmt.Printf("%s: v%s -> v%s\n", t["SettingUpdateDone"], cur, lat)
		return updateExitOK
	}
	data, _ := os.ReadFile(applyLog)
	out := parseBatchOutcome(string(data))
	if out.code == updateExitRollback {
		fmt.Printf("%s (%s)\n%s: %s\n", t["SettingUpdateFailed"], t["UpdateSameVersion"], t["SettingUpdateFailed"], applyLog)
		return updateExitRollback
	}
	fmt.Printf("%s\n%s: %s\n", t["SettingUpdateFailed"], t["SettingUpdateFailed"], applyLog)
	return updateExitFail
}

// fileHash returns the sha256 hex of a file, empty on error.
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func launchWindowsSwap(exe, newExe, updateDir string) error {
	applyScript := filepath.Join(updateDir, "apply_update.bat")
	applyLog := filepath.Join(updateDir, "apply_update.log")
	batContent := fmt.Sprintf(`@echo off
set LOG="%s"
echo [%%date%% %%time%%] waiting for old process to exit >> %%LOG%%
timeout /t 2 /nobreak >nul
set M=0
:moveretry
set /a M+=1
echo [%%date%% %%time%%] move attempt %%M%%: move old aside >> %%LOG%%
move /Y "%s" "%s.old" >> %%LOG%% 2>&1
if errorlevel 1 (
  echo [%%date%% %%time%%] move attempt %%M%% failed >> %%LOG%%
  if %%M%% GEQ 10 goto :failmove
  timeout /t 2 /nobreak >nul
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
  timeout /t 2 /nobreak >nul
  goto :copyretry
)
echo [%%date%% %%time%%] success: swapped, cleaning up >> %%LOG%%
del "%s" >> %%LOG%% 2>&1
start "" "%s" --tray
exit /b 0
:rollback
echo [%%date%% %%time%%] copy failed after move: rolling back old binary >> %%LOG%%
set R=0
:rbretry
set /a R+=1
move /Y "%s.old" "%s" >> %%LOG%% 2>&1
if errorlevel 1 (
  echo [%%date%% %%time%%] rollback attempt %%R%% failed >> %%LOG%%
  if %%R%% GEQ 5 goto :failcopy
  timeout /t 2 /nobreak >nul
  goto :rbretry
)
echo [%%date%% %%time%%] rolled back: old binary restored, update NOT applied >> %%LOG%%
start "" "%s" --tray
exit /b 1
:failmove
echo [%%date%% %%time%%] FAILED move after 10 attempts, exe untouched >> %%LOG%%
exit /b 1
:failcopy
echo [%%date%% %%time%%] FAILED copy and rollback after retries, manual repair needed >> %%LOG%%
exit /b 1
`, applyLog, exe, exe, newExe, exe, newExe, exe, exe, exe, exe)
	if err := os.WriteFile(applyScript, []byte(batContent), 0644); err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/c", applyScript)
	hideWindow(cmd)
	return cmd.Start()
}

// waitForSwapOutcome polls up to 60s for the swapped exe to verify against
// the downloaded hash, then maps the batch log to an exit code.
func waitForSwapOutcome(exe, newExe, wantHash, applyLog, latest string) int {
	_ = latest
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		got, err := fileHash(exe)
		if err == nil && wantHash != "" && got == wantHash {
			return updateExitOK
		}
		if data, err := os.ReadFile(applyLog); err == nil {
			out := parseBatchOutcome(string(data))
			if out.code == updateExitRollback {
				return updateExitRollback
			}
			if out.code == updateExitFail && strings.Contains(strings.ToLower(string(data)), "failed") {
				return updateExitFail
			}
		}
	}
	if data, err := os.ReadFile(applyLog); err == nil {
		return parseBatchOutcome(string(data)).code
	}
	return updateExitFail
}
