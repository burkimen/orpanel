package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type OmniHealth struct {
	Installed       bool   `json:"installed"`
	Path            string `json:"path"`
	Version         string `json:"version"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	NodeVersion     string `json:"nodeVersion"`
	NodeOk          bool   `json:"nodeOk"`
	Status          string `json:"status"` // running|stopped|not_installed|corrupt|port_conflict
	PortFree        bool   `json:"portFree"`
	Health          string `json:"health"` // ok|port_conflict|missing_deps|not_installed
	Message         string `json:"message"`
}

var (
	latestCache     string
	latestCacheTime time.Time
	latestMu        sync.Mutex
	omniOpMu        sync.Mutex
	omniOpRunning   bool
)

func getNodeVersion() (string, bool) {
	verOut, err := exec.Command("node", "--version").Output()
	if err != nil {
		// try nodejs binary name on some linux
		verOut, err = exec.Command("nodejs", "--version").Output()
		if err != nil {
			return "", false
		}
	}
	ver := strings.TrimSpace(string(verOut))
	ver = strings.TrimPrefix(ver, "v")
	// check >=22.22.2 <23 || >=24 <27
	// simple check: major >=24 or (major==22 && minor>=22) etc. For now major >=22 and major !=23
	parts := strings.Split(ver, ".")
	if len(parts) < 1 {
		return ver, false
	}
	var major, minor int
	fmt.Sscanf(parts[0], "%d", &major)
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &minor)
	}
	ok := false
	if major >= 24 && major < 27 {
		ok = true
	} else if major == 22 && minor >= 22 {
		ok = true
	} else if major == 22 && len(parts) == 1 {
		ok = true
	}
	return ver, ok
}

func getOmniLocalVersion(p string) string {
	if p == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(p, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return pkg.Version
}

func getOmniLatestVersion() string {
	latestMu.Lock()
	defer latestMu.Unlock()
	if latestCache != "" && time.Since(latestCacheTime) < time.Hour {
		return latestCache
	}
	// npm view
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "npm", "view", "omniroute", "version")
	} else {
		cmd = exec.Command("npm", "view", "omniroute", "version")
	}
	out, err := cmd.Output()
	if err != nil {
		// fallback to cached if any
		return latestCache
	}
	ver := strings.TrimSpace(string(out))
	// npm view may return multiple lines if multiple versions, take last
	lines := strings.Split(ver, "\n")
	ver = strings.TrimSpace(lines[len(lines)-1])
	// strip v prefix if any and quotes
	ver = strings.Trim(ver, `"' `)
	if ver != "" {
		latestCache = ver
		latestCacheTime = time.Now()
	}
	return ver
}

func isVersionLess(a, b string) bool {
	// simple semver compare a < b
	if a == "" || b == "" {
		return false
	}
	pa := strings.Split(strings.TrimPrefix(a, "v"), ".")
	pb := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < 3; i++ {
		var ai, bi int
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &ai)
		}
		if i < len(pb) {
			fmt.Sscanf(pb[i], "%d", &bi)
		}
		if ai < bi {
			return true
		}
		if ai > bi {
			return false
		}
	}
	return false
}

func checkOmniHealth() OmniHealth {
	path := getOmniroutePathEnhanced()
	installed := isOmnirouteDir(path)
	var ver string
	if installed {
		ver = getOmniLocalVersion(path)
		if ver == "" {
			installed = false
		}
	}
	latest := getOmniLatestVersion()
	updateAvail := false
	if ver != "" && latest != "" && isVersionLess(ver, latest) {
		updateAvail = true
	}
	nodeVer, nodeOk := getNodeVersion()
	portFree := !isPortInUse(OmniPort)
	// also check if omni is running via cmd mutex
	cmdMutex.Lock()
	running := cmd != nil && cmd.Process != nil
	cmdMutex.Unlock()
	status := "not_installed"
	health := "not_installed"
	msg := "OmniRoute sisteminizde kurulu değil"
	if installed {
		if running && ver != "" {
			status = "running"
			health = "ok"
			msg = "OmniRoute çalışıyor"
		} else if !portFree {
			status = "port_conflict"
			health = "port_conflict"
			msg = "Port 20128 dolu, OmniRoute başlatılamıyor"
		} else if ver == "" {
			status = "corrupt"
			health = "not_installed"
			msg = "OmniRoute dosyaları bozuk"
		} else {
			status = "stopped"
			health = "ok"
			msg = "OmniRoute kurulu fakat duruyor"
		}
		if !nodeOk {
			health = "missing_deps"
			msg = "Node.js 22+ gerekli, mevcut: " + nodeVer
		}
	} else {
		if !nodeOk && nodeVer != "" {
			msg = "Node.js 22+ gerekli (" + nodeVer + "), OmniRoute kurulu değil"
		} else if !nodeOk {
			msg = "Node.js bulunamadı, önce Node.js kurun"
		}
	}
	return OmniHealth{
		Installed:       installed,
		Path:            path,
		Version:         ver,
		Latest:          latest,
		UpdateAvailable: updateAvail,
		NodeVersion:     nodeVer,
		NodeOk:          nodeOk,
		Status:          status,
		PortFree:        portFree,
		Health:          health,
		Message:         msg,
	}
}

func handleOmniHealth(w http.ResponseWriter, r *http.Request) {
	h := checkOmniHealth()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h)
}

func runNpmOmni(op string, args ...string) {
	omniOpMu.Lock()
	if omniOpRunning {
		omniOpMu.Unlock()
		writeLog("WARN: OmniRoute işlemi zaten çalışıyor, %s atlandı", op)
		return
	}
	omniOpRunning = true
	omniOpMu.Unlock()
	defer func() {
		omniOpMu.Lock()
		omniOpRunning = false
		omniOpMu.Unlock()
	}()

	writeLog("INFO: OmniRoute %s başlatılıyor: npm %s", op, strings.Join(args, " "))
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// use cmd /c npm
		allArgs := append([]string{"/c", "npm"}, args...)
		cmd = exec.Command("cmd", allArgs...)
	} else {
		cmd = exec.Command("npm", args...)
	}
	// stream via writeLog
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				writeLog("[NPM] %s", line)
			}
		}
	}
	if err != nil {
		writeLog("ERROR: npm %s başarısız: %v", strings.Join(args, " "), err)
	} else {
		writeLog("SUCCESS: npm %s tamamlandı", strings.Join(args, " "))
		// invalidate latest cache after install/update
		latestMu.Lock()
		latestCache = ""
		latestMu.Unlock()
	}
}

func handleOmniInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h := checkOmniHealth()
	if !h.NodeOk {
		http.Error(w, "Node.js 22+ gerekli: "+h.NodeVersion, http.StatusPreconditionFailed)
		return
	}
	go runNpmOmni("kurulum", "install", "-g", "omniroute@latest")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "op": "install"})
}

func handleOmniUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h := checkOmniHealth()
	if !h.Installed {
		http.Error(w, "OmniRoute kurulu değil", http.StatusNotFound)
		return
	}
	if !h.NodeOk {
		http.Error(w, "Node.js 22+ gerekli", http.StatusPreconditionFailed)
		return
	}
	go runNpmOmni("güncelleme", "update", "-g", "omniroute@latest")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "op": "update"})
}

func handleOmniRepair(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go runNpmOmni("onarım", "install", "-g", "omniroute@latest", "--force")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "op": "repair"})
}

func handleOmniReinstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go func() {
		omniOpMu.Lock()
		if omniOpRunning {
			omniOpMu.Unlock()
			writeLog("WARN: Reinstall zaten çalışıyor")
			return
		}
		omniOpRunning = true
		omniOpMu.Unlock()
		defer func() {
			omniOpMu.Lock()
			omniOpRunning = false
			omniOpMu.Unlock()
		}()
		writeLog("INFO: OmniRoute yeniden kurulum: uninstall")
		var cmd1 *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd1 = exec.Command("cmd", "/c", "npm", "uninstall", "-g", "omniroute")
		} else {
			cmd1 = exec.Command("npm", "uninstall", "-g", "omniroute")
		}
		out, _ := cmd1.CombinedOutput()
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) != "" {
				writeLog("[NPM] %s", strings.TrimSpace(line))
			}
		}
		writeLog("INFO: OmniRoute yeniden kurulum: install")
		var cmd2 *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd2 = exec.Command("cmd", "/c", "npm", "install", "-g", "omniroute@latest")
		} else {
			cmd2 = exec.Command("npm", "install", "-g", "omniroute@latest")
		}
		out2, err := cmd2.CombinedOutput()
		for _, line := range strings.Split(string(out2), "\n") {
			if strings.TrimSpace(line) != "" {
				writeLog("[NPM] %s", strings.TrimSpace(line))
			}
		}
		if err != nil {
			writeLog("ERROR: reinstall başarısız: %v", err)
		} else {
			writeLog("SUCCESS: reinstall tamamlandı")
			latestMu.Lock()
			latestCache = ""
			latestMu.Unlock()
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "op": "reinstall"})
}
