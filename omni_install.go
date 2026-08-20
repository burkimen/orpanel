package main

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"path/filepath"
)

var (
	omniOpMu        sync.Mutex
	omniOpRunning   bool
)

func isOmniOpRunning() bool {
	omniOpMu.Lock()
	defer omniOpMu.Unlock()
	return omniOpRunning
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
	hideWindow(cmd)
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
		// invalidate caches after install/update
		latestMu.Lock()
		latestCache = ""
		latestMu.Unlock()
		cachedOmniPath = ""
		cachedOmniPathTime = time.Time{}
		OmniroutePath = getOmniroutePathEnhanced()
		StartArgs = filepath.Join(OmniroutePath, "bin", "omniroute.mjs")
		writeLog("INFO: OmniRoute yolu yenilendi: %s", OmniroutePath)
		// auto-start after successful install/update
		time.Sleep(800 * time.Millisecond)
		go startOmniroute()
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
		hideWindow(cmd1)
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
		hideWindow(cmd2)
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
			cachedOmniPath = ""
			cachedOmniPathTime = time.Time{}
			OmniroutePath = getOmniroutePathEnhanced()
			StartArgs = filepath.Join(OmniroutePath, "bin", "omniroute.mjs")
			writeLog("INFO: OmniRoute yolu yenilendi: %s", OmniroutePath)
			time.Sleep(800 * time.Millisecond)
			go startOmniroute()
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "op": "reinstall"})
}
