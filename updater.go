package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	DownloadURL    string `json:"downloadUrl"`
	ReleaseNotes   string `json:"releaseNotes"`
}

func getCurrentVersion() string {
	return AppVersion
}

const (
	githubLatestReleaseAPI = "https://api.github.com/repos/burkimen/orpanel/releases/latest"
	updaterUserAgent       = "orpanel-updater"
	// maxDownloadBytes caps a single self-update download (binary or sums file).
	maxDownloadBytes = 200 << 20 // 200 MiB
)

// releaseAsset maps a release asset file name to its browser download URL.
// Size comes from the GitHub API and feeds the preflight + progress display.
type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

func githubGet(url string, timeout time.Duration) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", updaterUserAgent)
	req.Header.Set("Accept", "application/octet-stream")
	return (&http.Client{Timeout: timeout}).Do(req)
}

func getLatestReleaseInfo() (version, notes string, assets map[string]string, err error) {
	full := getLatestReleaseFull()
	if full.err != nil {
		return "", "", nil, full.err
	}
	return full.version, full.notes, full.assets, nil
}

type releaseFull struct {
	version string
	notes   string
	assets  map[string]string
	sizes   map[string]int64
	err     error
}

func getLatestReleaseFull() releaseFull {
	version, notes, assets, sizes, err := fetchLatestRelease()
	return releaseFull{version: version, notes: notes, assets: assets, sizes: sizes, err: err}
}

func fetchLatestRelease() (version, notes string, assets map[string]string, sizes map[string]int64, err error) {
	req, err := http.NewRequest(http.MethodGet, githubLatestReleaseAPI, nil)
	if err != nil {
		return "", "", nil, nil, err
	}
	req.Header.Set("User-Agent", updaterUserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", nil, nil, fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}
	var release struct {
		TagName string         `json:"tag_name"`
		Body    string         `json:"body"`
		Assets  []releaseAsset `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", nil, nil, err
	}
	assets = make(map[string]string, len(release.Assets))
	sizes = make(map[string]int64, len(release.Assets))
	for _, a := range release.Assets {
		if a.Name != "" && a.URL != "" {
			assets[a.Name] = a.URL
			sizes[a.Name] = a.Size
		}
	}
	version = strings.TrimPrefix(release.TagName, "v")
	return version, release.Body, assets, sizes, nil
}

// getAssetName returns the release asset file name for this OS/arch.
// Single source of truth shared by getDownloadURL and performUpdate.
func getAssetName() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Normalize arch: amd64 -> x64 (Go uses amd64, binaries use x64)
	if arch == "amd64" {
		arch = "x64"
	}

	switch osName {
	case "windows":
		return fmt.Sprintf("orpanel-win32-%s.exe", arch)
	case "darwin":
		return fmt.Sprintf("orpanel-darwin-%s", arch)
	default:
		return fmt.Sprintf("orpanel-%s-%s", osName, arch)
	}
}

func getDownloadURL(version string) string {
	return fmt.Sprintf("https://github.com/burkimen/orpanel/releases/download/v%s/%s", version, getAssetName())
}

// verifyChecksum checks filePath against sha256sum-format sumsText for assetName.
// Accepts text ("<hex>  <name>") and binary ("<hex> *<name>") markers, ignores
// blank lines and lines with malformed hex; duplicate entries: last match wins.
func verifyChecksum(sumsText, assetName, filePath string) error {
	var want string
	found := false
	validLines := 0
	for _, line := range strings.Split(sumsText, "\n") {
		line = strings.TrimLeft(strings.TrimRight(line, "\r"), " \t")
		if line == "" {
			continue
		}
		sep := strings.IndexAny(line, " \t")
		if sep < 0 {
			continue
		}
		hash := line[:sep]
		rest := strings.TrimLeft(line[sep:], " \t")
		rest = strings.TrimPrefix(rest, "*")
		name := strings.TrimSpace(rest)
		raw, derr := hex.DecodeString(strings.ToLower(strings.TrimSpace(hash)))
		if derr != nil || len(raw) != sha256.Size || name == "" {
			continue
		}
		validLines++
		if name == assetName {
			want = strings.ToLower(strings.TrimSpace(hash))
			found = true
		}
	}
	if validLines == 0 {
		return fmt.Errorf("malformed checksum file: no valid entries")
	}
	if !found {
		return fmt.Errorf("checksum entry not found for %s", assetName)
	}
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("checksum target unreadable: %v", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("checksum read failed: %v", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, strings.ToLower(want), got)
	}
	return nil
}

// downloadToFile fetches url with a bounded client timeout and size cap.
func downloadToFile(url, destPath string) (int64, error) {
	resp, err := githubGet(url, 120*time.Second)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	outFile, err := os.Create(destPath)
	if err != nil {
		return 0, err
	}
	written, err := io.Copy(outFile, io.LimitReader(resp.Body, maxDownloadBytes+1))
	outFile.Close()
	if err != nil {
		os.Remove(destPath)
		return 0, err
	}
	if written > maxDownloadBytes {
		os.Remove(destPath)
		return 0, fmt.Errorf("download exceeds %d MiB limit", maxDownloadBytes>>20)
	}
	return written, nil
}

func getUpdateDir() string {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("LOCALAPPDATA")
		if appData == "" {
			appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(appData, "Orpanel", "update")
	}
	return filepath.Join(os.TempDir(), "orpanel-update")
}

func checkForUpdate() UpdateInfo {
	current := getCurrentVersion()
	latest, notes, _, err := getLatestReleaseInfo()
	if err != nil {
		return UpdateInfo{
			CurrentVersion: current,
			LatestVersion:  current,
		}
	}

	updateAvailable := isVersionLess(current, latest)
	downloadURL := ""
	if updateAvailable {
		downloadURL = getDownloadURL(latest)
	}

	return UpdateInfo{
		CurrentVersion: current,
		LatestVersion:  latest,
		UpdateAvailable: updateAvailable,
		DownloadURL:    downloadURL,
		ReleaseNotes:   notes,
	}
}

func performUpdate() error {
	current := getCurrentVersion()
	latest, _, assets, err := getLatestReleaseInfo()
	if err != nil {
		return fmt.Errorf("GitHub releases unreachable: %v", err)
	}

	if !isVersionLess(current, latest) {
		return fmt.Errorf("already up to date: v%s", current)
	}

	assetName := getAssetName()
	sumsURL, ok := assets["sha256sums.txt"]
	if !ok {
		return fmt.Errorf("release v%s has no sha256sums.txt; re-run the install script to update", latest)
	}
	downloadURL, ok := assets[assetName]
	if !ok {
		return fmt.Errorf("release v%s has no asset %s; re-run the install script to update", latest, assetName)
	}
	writeLog("INFO: Downloading update v%s -> v%s", current, latest)
	setUpdatePhase(updateDownloading, current, latest, "")
	exe, _ := os.Executable()
	updateDir := getUpdateDir()
	os.MkdirAll(updateDir, 0755)

	newExe := filepath.Join(updateDir, filepath.Base(exe))

	written, err := downloadToFile(downloadURL, newExe)
	if err != nil {
		os.Remove(newExe)
		return fmt.Errorf("download failed: %v", err)
	}
	writeLog("INFO: Downloaded: %.1f MB", float64(written)/(1024*1024))
	setUpdatePhase(updateVerifying, current, latest, "")

	// Fetch checksums from the same release and verify before applying.
	sumsPath := filepath.Join(updateDir, "sha256sums.txt")
	if _, err := downloadToFile(sumsURL, sumsPath); err != nil {
		os.Remove(newExe)
		os.Remove(sumsPath)
		return fmt.Errorf("release v%s has no sha256sums.txt; re-run the install script to update: %v", latest, err)
	}
	sumsBytes, err := os.ReadFile(sumsPath)
	os.Remove(sumsPath)
	if err != nil {
		os.Remove(newExe)
		return fmt.Errorf("checksum file unreadable: %v", err)
	}
	if err := verifyChecksum(string(sumsBytes), assetName, newExe); err != nil {
		os.Remove(newExe)
		return fmt.Errorf("checksum verification failed: %v", err)
	}

	// Make executable on unix
	if runtime.GOOS != "windows" {
		os.Chmod(newExe, 0755)
	}
	writeLog("SUCCESS: v%s → v%s güncelleniyor, yeniden başlatılıyor...", current, latest)
	setUpdatePhase(updateApplying, current, latest, "")
	if runtime.GOOS == "windows" {
		// Windows: rename-then-copy. A running exe cannot be overwritten,
		// but it CAN be moved aside first. Invariant: <exe> exists when
		// this script ends, either as the new build or rolled back.
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
		os.WriteFile(applyScript, []byte(batContent), 0644)

		cmd := exec.Command("cmd", "/c", applyScript)
		hideWindow(cmd)
		cmd.Start()
	} else {
		// Unix: rename çalışır
		if err := os.Rename(newExe, exe); err != nil {
			os.Remove(newExe)
			return fmt.Errorf("rename failed: %v", err)
		}
		cmd := exec.Command(exe, "--tray")
		cmd.SysProcAttr = relaunchAttrs()
		cmd.Start()
	}
	// Exit current process. Bound: this delay must stay below the batch's
	// `timeout /t 2` so the old process is gone before the swap runs.
	setUpdatePhase(updateRestarting, current, latest, "")
	time.Sleep(1200 * time.Millisecond)
	os.Exit(0)
	return nil
}

func handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	info := checkForUpdate()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

type updatePhase string

const (
	updateIdle       updatePhase = "idle"
	updateDownloading updatePhase = "downloading"
	updateVerifying  updatePhase = "verifying"
	updateApplying   updatePhase = "applying"
	updateRestarting updatePhase = "restarting"
	updateFailed     updatePhase = "failed"
)

type updateStatus struct {
	Phase          updatePhase `json:"phase"`
	Error          string      `json:"error"`
	CurrentVersion string      `json:"currentVersion"`
	LatestVersion  string      `json:"latestVersion"`
	StartedAt      string      `json:"startedAt"`
	UpdatedAt      string      `json:"updatedAt"`
}

var (
	updateMu     sync.Mutex
	updatePhaseNow updatePhase = updateIdle
	updateErr    string
	updateCur    string
	updateLat    string
	updateStart  time.Time
	updateAt     time.Time
)

func setUpdatePhase(p updatePhase, cur, lat, errStr string) {
	updateMu.Lock()
	defer updateMu.Unlock()
	updatePhaseNow = p
	if cur != "" {
		updateCur = cur
	}
	if lat != "" {
		updateLat = lat
	}
	updateErr = errStr
	now := time.Now()
	if p == updateDownloading && updateStart.IsZero() {
		updateStart = now
	}
	updateAt = now
}

func getUpdateStatus() updateStatus {
	updateMu.Lock()
	defer updateMu.Unlock()
	var started, updated string
	if !updateStart.IsZero() {
		started = updateStart.UTC().Format(time.RFC3339)
	}
	if !updateAt.IsZero() {
		updated = updateAt.UTC().Format(time.RFC3339)
	}
	cur := updateCur
	if cur == "" {
		cur = getCurrentVersion()
	}
	return updateStatus{
		Phase:          updatePhaseNow,
		Error:          updateErr,
		CurrentVersion: cur,
		LatestVersion:  updateLat,
		StartedAt:      started,
		UpdatedAt:      updated,
	}
}

func handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(getUpdateStatus())
}

func handlePerformUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cur, lat := getCurrentVersion(), ""
	if info := checkForUpdate(); info.LatestVersion != "" {
		lat = info.LatestVersion
		cur = info.CurrentVersion
	}
	setUpdatePhase(updateDownloading, cur, lat, "")
	go func() {
		if err := performUpdate(); err != nil {
			setUpdatePhase(updateFailed, "", "", err.Error())
			writeLog("ERROR: Güncelleme başarısız: %v", err)
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(getUpdateStatus())
}
