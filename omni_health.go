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
	Status          string `json:"status"` // running|stopped|not_installed|corrupt|port_conflict|installing
	PortFree        bool   `json:"portFree"`
	Health          string `json:"health"` // ok|port_conflict|missing_deps|not_installed
	Message         string `json:"message"`
	OpRunning       bool   `json:"opRunning"`
}

var (
	latestCache     string
	latestCacheTime time.Time
	latestMu        sync.Mutex
	nodeCache       string
	nodeCacheOk     bool
	nodeCacheTime   time.Time
	nodeCacheMu     sync.Mutex
)

func getNodeVersion() (string, bool) {
	nodeCacheMu.Lock()
	if nodeCacheTime.After(time.Now().Add(-5 * time.Minute)) && nodeCache != "" {
		v, ok := nodeCache, nodeCacheOk
		nodeCacheMu.Unlock()
		return v, ok
	}
	nodeCacheMu.Unlock()

	cmd := exec.Command("node", "--version")
	hideWindow(cmd)
	verOut, err := cmd.Output()
	if err != nil {
		// try nodejs binary name on some linux
		cmd2 := exec.Command("nodejs", "--version")
		hideWindow(cmd2)
		verOut, err = cmd2.Output()
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
	nodeCacheMu.Lock()
	nodeCache = ver
	nodeCacheOk = ok
	nodeCacheTime = time.Now()
	nodeCacheMu.Unlock()
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
	hideWindow(cmd)
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
	h := OmniHealth{
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
	omniOpMu.Lock()
	h.OpRunning = omniOpRunning
	omniOpMu.Unlock()
	if h.OpRunning {
		h.Status = "installing"
		h.Health = "installing"
		h.Message = "OmniRoute işlemi devam ediyor, lütfen bekleyin..."
	}
	return h
}

func handleOmniHealth(w http.ResponseWriter, r *http.Request) {
	h := checkOmniHealth()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h)
}