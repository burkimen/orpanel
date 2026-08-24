package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	omniOpMu      sync.Mutex
	omniOpRunning bool
)

func isOmniOpRunning() bool {
	omniOpMu.Lock()
	defer omniOpMu.Unlock()
	return omniOpRunning
}

func streamCmd(name string, args []string) error {
	cmd := exec.Command(name, args...)
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			writeLog("[NPM] %s", scanner.Text())
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			writeLog("[NPM] %s", scanner.Text())
		}
	}()
	return cmd.Wait()
}

func npmCmd(args []string) []string {
	return args
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
	err := streamCmd("npm", args)
	if err != nil {
		writeLog("ERROR: npm %s başarısız: %v", strings.Join(args, " "), err)
	} else {
		writeLog("SUCCESS: npm %s tamamlandı", strings.Join(args, " "))
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
		streamCmd("npm", []string{"uninstall", "-g", "omniroute"})

		writeLog("INFO: OmniRoute yeniden kurulum: install")
		err := streamCmd("npm", []string{"install", "-g", "omniroute@latest"})
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
