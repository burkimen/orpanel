package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
		return fmt.Errorf("zaten güncel sürümdesiniz: %s", current)
	}

	downloadURL := getDownloadURL(latest)
	writeLog("INFO: Güncelleme başlatılıyor v%s → v%s", current, latest)
	writeLog("INFO: İndiriliyor: %s", downloadURL)

	exe, _ := os.Executable()
	tmpPath := exe + ".update.tmp"

	// Download
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("indirme başarısız: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("indirme başarısız: HTTP %d", resp.StatusCode)
	}

	outFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("dosya oluşturulamadı: %v", err)
	}
	written, err := io.Copy(outFile, resp.Body)
	outFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("indirme başarısız: %v", err)
	}
	writeLog("INFO: İndirildi: %.1f MB", float64(written)/(1024*1024))

	// Make executable on unix
	if runtime.GOOS != "windows" {
		os.Chmod(tmpPath, 0755)
	}

	// Replace current binary
	if err := os.Rename(tmpPath, exe); err != nil {
		// Windows: rename fails if file is locked, try copy
		if runtime.GOOS == "windows" {
			data, readErr := os.ReadFile(tmpPath)
			if readErr != nil {
				return fmt.Errorf("yedekleme başarısız: %v", readErr)
			}
			os.WriteFile(exe, data, 0755)
			os.Remove(tmpPath)
		} else {
			return fmt.Errorf("binary değiştirilemedi: %v", err)
		}
	}

	writeLog("SUCCESS: Güncelleme tamamlandı: v%s → v%s", current, latest)
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
