package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
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

func getLatestReleaseInfo() (string, string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/burkimen/orpanel/releases/latest")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var release struct {
		TagName    string `json:"tag_name"`
		Body       string `json:"body"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", err
	}

	version := strings.TrimPrefix(release.TagName, "v")
	return version, release.Body, nil
}

func getDownloadURL(version string) string {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	var assetName string
	switch osName {
	case "windows":
		assetName = fmt.Sprintf("orpanel-%s-%s.exe", osName, arch)
	case "darwin":
		assetName = fmt.Sprintf("orpanel-%s-%s", osName, arch)
	default:
		assetName = fmt.Sprintf("orpanel-%s-%s", osName, arch)
	}

	return fmt.Sprintf("https://github.com/burkimen/orpanel/releases/download/v%s/%s", version, assetName)
}

func checkForUpdate() UpdateInfo {
	current := getCurrentVersion()
	latest, notes, err := getLatestReleaseInfo()
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
	latest, _, err := getLatestReleaseInfo()
	if err != nil {
		return fmt.Errorf("GitHub releases erişilemedi: %v", err)
	}

	if !isVersionLess(current, latest) {
		return fmt.Errorf("zaten güncel: v%s", current)
	}

	downloadURL := getDownloadURL(latest)
	writeLog("INFO: Güncelleme v%s → v%s indiriliyor", current, latest)

	exe, _ := os.Executable()
	newPath := exe + ".new"
	oldPath := exe + ".old"

	// Cleanup previous attempt
	os.Remove(newPath)
	os.Remove(oldPath)

	// Download new binary
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("indirme başarısız: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("indirme başarısız: HTTP %d", resp.StatusCode)
	}

	outFile, err := os.Create(newPath)
	if err != nil {
		return fmt.Errorf("dosya oluşturulamadı: %v", err)
	}
	written, err := io.Copy(outFile, resp.Body)
	outFile.Close()
	if err != nil {
		os.Remove(newPath)
		return fmt.Errorf("indirme başarısız: %v", err)
	}
	writeLog("INFO: İndirildi: %.1f MB", float64(written)/(1024*1024))

	// Make executable on unix
	if runtime.GOOS != "windows" {
		os.Chmod(newPath, 0755)
	}

	// Replace binary: rename current → .old, new → current
	// This works even while current process is running (OS keeps old handle)
	if err := os.Rename(exe, oldPath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("mevcut binary yeniden adlandırılamadı: %v", err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		os.Rename(oldPath, exe) // rollback
		return fmt.Errorf("yeni binary yerleştirilemedi: %v", err)
	}

	writeLog("SUCCESS: v%s → v%s güncellendi. Yeniden başlatılıyor...", current, latest)

	// Cleanup old binary after a short delay (will be in use briefly)
	go func() {
		time.Sleep(2 * time.Second)
		os.Remove(oldPath)
	}()

	// Relaunch
	if runtime.GOOS == "windows" {
		cmd := exec.Command(exe, "--tray")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: 0x00000008 | 0x00000200,
			HideWindow:    true,
		}
		cmd.Start()
	} else {
		cmd := exec.Command(exe, "--tray")
		cmd.Start()
	}

	return nil
}

func handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	info := checkForUpdate()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func handlePerformUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go func() {
		if err := performUpdate(); err != nil {
			writeLog("ERROR: Güncelleme başarısız: %v", err)
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}
