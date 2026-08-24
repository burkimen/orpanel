package main

import (
	"encoding/json"
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
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
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
		assetName = fmt.Sprintf("orpanel-win32-%s.exe", arch)
	case "darwin":
		assetName = fmt.Sprintf("orpanel-darwin-%s", arch)
	default:
		assetName = fmt.Sprintf("orpanel-%s-%s", osName, arch)
	}

	return fmt.Sprintf("https://github.com/burkimen/orpanel/releases/download/v%s/%s", version, assetName)
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
	updateDir := getUpdateDir()
	os.MkdirAll(updateDir, 0755)

	newExe := filepath.Join(updateDir, filepath.Base(exe))

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

	outFile, err := os.Create(newExe)
	if err != nil {
		return fmt.Errorf("dosya oluşturulamadı: %v", err)
	}
	written, err := io.Copy(outFile, resp.Body)
	outFile.Close()
	if err != nil {
		os.Remove(newExe)
		return fmt.Errorf("indirme başarısız: %v", err)
	}
	writeLog("INFO: İndirildi: %.1f MB", float64(written)/(1024*1024))

	// Make executable on unix
	if runtime.GOOS != "windows" {
		os.Chmod(newExe, 0755)
	}

	writeLog("SUCCESS: v%s → v%s güncelleniyor, yeniden başlatılıyor...", current, latest)

	if runtime.GOOS == "windows" {
		// Windows: batch script ile güncelleme
		applyScript := filepath.Join(updateDir, "apply_update.bat")
		batContent := fmt.Sprintf(`@echo off
timeout /t 2 /nobreak >nul
copy /Y "%s" "%s"
del "%s"
start "" "%s" --tray
`, newExe, exe, newExe, exe)
		os.WriteFile(applyScript, []byte(batContent), 0644)

		cmd := exec.Command("cmd", "/c", applyScript)
		cmd.SysProcAttr = relaunchAttrs()
		cmd.Start()
	} else {
		// Unix: rename çalışır
		os.Rename(newExe, exe)
		cmd := exec.Command(exe, "--tray")
		cmd.SysProcAttr = relaunchAttrs()
		cmd.Start()
	}

	// Exit current process
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
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
