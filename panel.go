package main

import (
	"bufio"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"html/template"
	"time"

	"github.com/getlantern/systray"
)

//go:embed web
var webFS embed.FS

// --- EVRENSEL VE PLATFORMA DUYARLI YOL YAPILANDIRMASI ---
func getOmniroutePath() string {
	return getOmniroutePathEnhanced()
}

var (
	OmniroutePath = getOmniroutePath()
	StartCommand  = "node"
	StartArgs     = filepath.Join(OmniroutePath, "bin", "omniroute.mjs")
)

const AppName = "OmniroutePanel"
const AppVersion = "1.0.2"
const LogFileName = "panel.log"
const ConfigFileName = "config.json"
const LocalesDir = "locales"
const OmniPort = 20128
const logMaxBytes = 5 * 1024 * 1024

var (
	cmd               *exec.Cmd
	cmdMutex          sync.Mutex
	logBuffer         []string
	logMutex          sync.Mutex
	maxLogSize        = 1000
	mAutoStart        *systray.MenuItem
	mToggle           *systray.MenuItem
	mOpenOmni         *systray.MenuItem
	mOpen             *systray.MenuItem
	mQuit             *systray.MenuItem
	fileLogWriter     *os.File
	isIntentionalStop bool
	currentLang       = "tr"
	configMutex       sync.Mutex
	watchdogFailCount int
	watchdogLastFail  time.Time
	crashBackoffUntil time.Time
)

type StatusResponse struct {
	IsRunning bool `json:"isRunning"`
}
type AutoStartResponse struct {
	IsEnabled bool `json:"isEnabled"`
}
type LangResponse struct {
	Language string `json:"language"`
}
type ThemeResponse struct {
	Theme string `json:"theme"`
}


// --- Port helpers (EADDRINUSE fix) ---
func isPortInUse(port int) bool {
	// try both v4 and v6
	for _, host := range []string{"127.0.0.1", "::1"} {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)), 400*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}
	// also try 0.0.0.0 via net.Listen probe (if dial fails but listen also fails, port is in use)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return true
	}
	ln.Close()
	return false
}

func killPortHolders(port int) int {
	killed := 0
	if runtime.GOOS == "windows" {
		// Prefer PowerShell Get-NetTCPConnection
		psCmd := fmt.Sprintf("Get-NetTCPConnection -LocalPort %d -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique", port)
		cmd := exec.Command("powershell", "-NoProfile", "-Command", psCmd)
		hideWindow(cmd)
		out, err := cmd.Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				pidStr := strings.TrimSpace(line)
				if pidStr == "" {
					continue
				}
				pid, err := strconv.Atoi(pidStr)
				if err != nil || pid == os.Getpid() {
					continue
				}
				// avoid killing system idle (0) or very low pids
				if pid < 100 {
					continue
				}
				c := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
				hideWindow(c)
				c.Run()
				killed++
			}
			if killed > 0 {
				return killed
			}
		}
		// fallback: netstat
		cmd2 := exec.Command("cmd", "/c", fmt.Sprintf("netstat -ano | findstr :%d", port))
		hideWindow(cmd2)
		out2, err := cmd2.Output()
		if err == nil {
			for _, line := range strings.Split(string(out2), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 0 {
					continue
				}
				pidStr := fields[len(fields)-1]
				pid, err := strconv.Atoi(pidStr)
				if err != nil || pid == os.Getpid() || pid < 100 {
					continue
				}
				c2 := exec.Command("taskkill", "/F", "/PID", pidStr)
				hideWindow(c2)
				c2.Run()
				killed++
			}
		}
	} else {
		// macOS / Linux
		out, err := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti:%d 2>/dev/null", port)).Output()
		if err == nil {
			for _, pidStr := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				pidStr = strings.TrimSpace(pidStr)
				if pidStr == "" {
					continue
				}
				exec.Command("kill", "-9", pidStr).Run()
				killed++
			}
		}
		if killed == 0 {
			// fallback: fuser
			exec.Command("sh", "-c", fmt.Sprintf("fuser -k %d/tcp 2>/dev/null", port)).Run()
		}
	}
	return killed
}

func ensurePortFree(port int) bool {
	if !isPortInUse(port) {
		return true
	}
	writeLog("WARN: Port %d dolu, temizleniyor...", port)
	killPortHolders(port)
	time.Sleep(1200 * time.Millisecond)
	if isPortInUse(port) {
		// second attempt - broader: kill node.exe that may be omniroute
		if runtime.GOOS == "windows" {
			c := exec.Command("taskkill", "/F", "/IM", "node.exe")
			hideWindow(c)
			c.Run()
			time.Sleep(800 * time.Millisecond)
		}
	}
	return !isPortInUse(port)
}

func rotateLogIfNeeded() {
	if fileLogWriter == nil {
		return
	}
	lp := getLogPath()
	info, err := os.Stat(lp)
	if err != nil {
		return
	}
	if info.Size() < logMaxBytes {
		return
	}
	fileLogWriter.Close()
	// keep last 50KB
	data, err := os.ReadFile(lp)
	if err == nil && len(data) > 50*1024 {
		data = data[len(data)-50*1024:]
		// cut to next newline
		if idx := strings.Index(string(data), "\n"); idx != -1 {
			data = data[idx+1:]
		}
		os.WriteFile(lp, data, 0644)
	}
	f, err := os.OpenFile(lp, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		fileLogWriter = f
	}
}

func initFileLog() {
	var err error
	lp := getLogPath()
	// rotate if oversized before opening
	if info, err2 := os.Stat(lp); err2 == nil && info.Size() > logMaxBytes {
		data, err3 := os.ReadFile(lp)
		if err3 == nil && len(data) > 50*1024 {
			cut := data[len(data)-50*1024:]
			if idx := strings.Index(string(cut), "\n"); idx != -1 {
				cut = cut[idx+1:]
			}
			_ = os.WriteFile(lp, cut, 0644)
		} else if err3 == nil {
			_ = os.WriteFile(lp, data, 0644)
		}
	}
	fileLogWriter, err = os.OpenFile(lp, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Log dosyası açılamadı: %v\n", err)
	}
}

func writeLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamped := fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), msg)

	// Detect EADDRINUSE crash to trigger backoff (debounced)
	if strings.Contains(msg, "EADDRINUSE") {
		logMutex.Lock()
		now := time.Now()
		// debounce: same burst within 5s counts as one failure
		if now.Sub(watchdogLastFail) < 5*time.Second && watchdogFailCount > 0 {
			// just extend backoff slightly, do not increment failCount
			if now.After(crashBackoffUntil) {
				crashBackoffUntil = now.Add(30 * time.Second)
			}
			logMutex.Unlock()
		} else {
			watchdogFailCount++
			watchdogLastFail = now
			backoff := time.Duration(30*(1<<min(watchdogFailCount-1, 3))) * time.Second
			if backoff > 300*time.Second {
				backoff = 300 * time.Second
			}
			crashBackoffUntil = now.Add(backoff)
			fc := watchdogFailCount
			bb := crashBackoffUntil
			logMutex.Unlock()
			// avoid recursion: direct write
			ts := fmt.Sprintf("[%s] BACKOFF: Port %d çakışması algılandı, %v beklemeye alındı (fail #%d, until %s)", time.Now().Format("2006-01-02 15:04:05"), OmniPort, backoff, fc, bb.Format("15:04:05"))
			logMutex.Lock()
			logBuffer = append(logBuffer, ts)
			if len(logBuffer) > maxLogSize {
				logBuffer = logBuffer[1:]
			}
			logMutex.Unlock()
			if fileLogWriter != nil {
				fileLogWriter.WriteString(ts + "\n")
			}
		}
	} else if strings.Contains(msg, "OmniRoute is running") {
		logMutex.Lock()
		watchdogFailCount = 0
		crashBackoffUntil = time.Time{}
		logMutex.Unlock()
	}

	logMutex.Lock()
	logBuffer = append(logBuffer, timestamped)
	if len(logBuffer) > maxLogSize {
		logBuffer = logBuffer[1:]
	}
	logMutex.Unlock()

	if fileLogWriter != nil {
		fileLogWriter.WriteString(timestamped + "\n")
		if info, err := fileLogWriter.Stat(); err == nil && info.Size() > logMaxBytes {
			// async rotate to avoid blocking
			go rotateLogIfNeeded()
		}
	}
}

func startOmniroute() {
	cmdMutex.Lock()
	defer cmdMutex.Unlock()

	if cmd != nil && cmd.Process != nil {
		return
	}

	// Backoff check - EADDRINUSE loop prevention, but if port now free, clear EADDRINUSE backoff
	logMutex.Lock()
	bb := crashBackoffUntil
	logMutex.Unlock()
	if !bb.IsZero() && time.Now().Before(bb) {
		if !isPortInUse(OmniPort) {
			// port freed externally, clear EADDRINUSE backoff to allow quick recovery
			logMutex.Lock()
			watchdogFailCount = 0
			crashBackoffUntil = time.Time{}
			logMutex.Unlock()
			writeLog("INFO: Port %d serbest kaldı, backoff temizlendi", OmniPort)
		} else {
			writeLog("INFO: Backoff aktif (%v kaldı), başlatma atlandı", time.Until(bb).Round(time.Second))
			return
		}
	}

	// Port collision check before spawn
	if !ensurePortFree(OmniPort) {
		writeLog("ERROR: Port %d hâlâ dolu, OmniRoute başlatılamadı. 15s sonra watchdog tekrar deneyecek.", OmniPort)
		logMutex.Lock()
		watchdogFailCount++
		crashBackoffUntil = time.Now().Add(15 * time.Second)
		logMutex.Unlock()
		return
	}

	// OmniRoute kurulu mu kontrolü (uninstall sonrası Dir geçersiz hatasını önle)
	if !isOmnirouteDir(OmniroutePath) {
		// path'i yenile
		OmniroutePath = getOmniroutePathEnhanced()
		StartArgs = filepath.Join(OmniroutePath, "bin", "omniroute.mjs")
		if !isOmnirouteDir(OmniroutePath) {
			writeLog("INFO: OmniRoute kurulu değil (%s), otomatik başlatma atlandı. Web'den 'Kurulumu Başlat' ile kurun.", OmniroutePath)
			return
		}
	}

	isIntentionalStop = false
	t := loadTranslations(currentLang)
	writeLog("%s", t["LogStarting"])

	cmd = exec.Command(StartCommand, StartArgs, "--no-open", "--no-tray")
	cmd.Dir = OmniroutePath
	
	cmd.Env = append(os.Environ(),
		"CI=true",
		"BROWSER=none",
		"NONINTERACTIVE=true",
	)

	configureStartCmd(cmd)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	err := cmd.Start()
	if err != nil {
		writeLog("ERROR: Servis başlatılamadı - %v", err)
		cmd = nil
		return
	}

	writeLog(t["LogStartedSuccess"], cmd.Process.Pid)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			writeLog("%s", scanner.Text())
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			writeLog("[HATA] %s", scanner.Text())
		}
	}()

	go func() {
		cmd.Wait()
		cmdMutex.Lock()
		defer cmdMutex.Unlock()
		
		tSub := loadTranslations(currentLang)
		cmd = nil
		if !isIntentionalStop {
			writeLog("%s", tSub["LogUnexpectedStop"])
		} else {
			writeLog("%s", tSub["LogStoppedInfo"])
		}
	}()
}

func stopOmniroute() {
	cmdMutex.Lock()
	defer cmdMutex.Unlock()

	isIntentionalStop = true
	logMutex.Lock()
	watchdogFailCount = 0
	crashBackoffUntil = time.Time{}
	logMutex.Unlock()
	t := loadTranslations(currentLang)
	writeLog("%s", t["LogStopSignal"])

	if cmd != nil && cmd.Process != nil {
		pid := cmd.Process.Pid
		if runtime.GOOS == "windows" {
			c := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
			hideWindow(c)
			c.Run()
		} else {
			cmd.Process.Kill()
		}
		cmd = nil
		writeLog("%s", t["LogProcessesKilled"])
	}
	// Extra: ensure port 20128 fully freed (orphan nodes)
	time.Sleep(400 * time.Millisecond)
	if isPortInUse(OmniPort) {
		killed := killPortHolders(OmniPort)
		if killed > 0 {
			writeLog("INFO: Port %d için %d yetim süreç temizlendi", OmniPort, killed)
			time.Sleep(600 * time.Millisecond)
		}
		if isPortInUse(OmniPort) {
			writeLog("WARN: Port %d hâlâ dolu, node.exe zorla kapatılıyor", OmniPort)
			if runtime.GOOS == "windows" {
				c2 := exec.Command("taskkill", "/F", "/IM", "node.exe")
				hideWindow(c2)
				c2.Run()
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}

func startWatchdog() {
	go func() {
		for {
			time.Sleep(5 * time.Second)
			cmdMutex.Lock()
			shouldRestart := cmd == nil && !isIntentionalStop
			cmdMutex.Unlock()
			if !shouldRestart {
				continue
			}
			// If OmniRoute not installed, don't auto-restart (user must click install)
			if !isOmnirouteDir(getOmniroutePathEnhanced()) {
				continue
			}
			// Respect crash backoff, but if port now free, clear EADDRINUSE backoff
			logMutex.Lock()
			bb := crashBackoffUntil
			failCnt := watchdogFailCount
			logMutex.Unlock()
			if !bb.IsZero() && time.Now().Before(bb) {
				if !isPortInUse(OmniPort) {
					logMutex.Lock()
					watchdogFailCount = 0
					crashBackoffUntil = time.Time{}
					logMutex.Unlock()
					writeLog("INFO: Port %d serbest kaldı, watchdog backoff temizlendi", OmniPort)
				} else {
					if failCnt > 0 && time.Now().Sub(watchdogLastFail) > 10*time.Second {
						writeLog("INFO: Backoff aktif, watchdog beklemede (remaining %v)", time.Until(bb).Round(time.Second))
						logMutex.Lock()
						watchdogLastFail = time.Now()
						logMutex.Unlock()
					}
					continue
				}
			}
			// Port still occupied after cleanup attempts -> log once and backoff
			if isPortInUse(OmniPort) {
				if !ensurePortFree(OmniPort) {
					writeLog("WARN: Port %d dolu, watchdog beklemede (fail #%d)", OmniPort, failCnt+1)
					logMutex.Lock()
					watchdogFailCount++
					watchdogLastFail = time.Now()
					crashBackoffUntil = time.Now().Add(30 * time.Second)
					logMutex.Unlock()
					continue
				}
			}
			t := loadTranslations(currentLang)
			writeLog("%s", t["LogWatchdog"])
			startOmniroute()
		}
	}()
}

func openBrowser(url string) {
	if runtime.GOOS == "windows" {
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		return
	}
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", url).Start()
		return
	}
	// linux: try xdg-open, then sensible-browser, www-browser, gio
	for _, opener := range []string{"xdg-open", "sensible-browser", "www-browser", "gio"} {
		if _, err := exec.LookPath(opener); err == nil {
			var cmd *exec.Cmd
			if opener == "gio" {
				cmd = exec.Command("gio", "open", url)
			} else {
				cmd = exec.Command(opener, url)
			}
			if err := cmd.Start(); err == nil {
				return
			}
		}
	}
	// last resort: try xdg-open anyway
	_ = exec.Command("xdg-open", url).Start()
}

func startWebServer() {
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, getIconPath())
	})

	http.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(exeDir(), "favicon.svg"))
	})

	http.Handle("/themes/", http.StripPrefix("/themes/", http.FileServer(http.Dir(getThemesDir()))))

	// web static (js/css) - use ReadFile fallback approach
	staticFS, _ := fs.Sub(webFS, "web/static")
	http.Handle("/web/static/", http.StripPrefix("/web/static/", http.FileServer(http.FS(staticFS))))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		cfg := loadConfig()
		// First launch: auto-detect language from browser
		if cfg.Language == "" || cfg.Language == "tr" {
			if _, err := os.Stat(getConfigPath()); os.IsNotExist(err) {
				detected := detectLanguage(r.Header.Get("Accept-Language"))
				cfg.Language = detected
				if cfg.Theme == "" || cfg.Theme == ThemeSystem {
					detectedTheme := "dark"
					if strings.Contains(strings.ToLower(r.Header.Get("Sec-CH-Prefers-Color-Scheme")), "light") {
						detectedTheme = "light"
					}
					cfg.Theme = detectedTheme
				}
				saveConfig(cfg.Language, cfg.AutoStart)
				saveTheme(cfg.Theme)
			}
		}
		t := loadTranslations(cfg.Language)
		tJson, _ := json.Marshal(t)
		data := map[string]interface{}{
			"Lang":       cfg.Language,
			"Theme":      cfg.Theme,
			"T":          t,
			"TJson":      template.JS(tJson),
			"AppVersion": AppVersion,
		}
		tmplData, _ := fs.ReadFile(webFS, "web/templates/index.html")
		tmpl := template.Must(template.New("index").Parse(string(tmplData)))
		tmpl.Execute(w, data)
	})

	http.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) { startOmniroute() })
	http.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) { stopOmniroute() })
	http.HandleFunc("/api/restart", func(w http.ResponseWriter, r *http.Request) {
		stopOmniroute()
		time.Sleep(1 * time.Second)
		startOmniroute()
	})

	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		cmdMutex.Lock()
		isRunning := (cmd != nil && cmd.Process != nil)
		cmdMutex.Unlock()
		json.NewEncoder(w).Encode(StatusResponse{IsRunning: isRunning})
	})

	http.HandleFunc("/api/autostart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req AutoStartResponse
			json.NewDecoder(r.Body).Decode(&req)
			setAutoStart(req.IsEnabled)
			
			if req.IsEnabled {
				mAutoStart.Check()
			} else {
				mAutoStart.Uncheck()
			}
			return
		}
		json.NewEncoder(w).Encode(AutoStartResponse{IsEnabled: isAutoStartEnabled()})
	})

	http.HandleFunc("/api/language", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req LangResponse
			json.NewDecoder(r.Body).Decode(&req)
			if req.Language != "" {
				cfg := loadConfig()
				saveConfig(req.Language, cfg.AutoStart)
				updateTrayTexts()
			}
			return
		}
		cfg := loadConfig()
		json.NewEncoder(w).Encode(LangResponse{Language: cfg.Language})
	})

	http.HandleFunc("/api/theme", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req ThemeResponse
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if !isValidTheme(req.Theme) {
				http.Error(w, "invalid theme", http.StatusBadRequest)
				return
			}
			saveTheme(req.Theme)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(ThemeResponse{Theme: req.Theme})
			return
		}
		cfg := loadConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ThemeResponse{Theme: cfg.Theme})
	})

	// OmniRoute health & ops
	http.HandleFunc("/api/omni/health", handleOmniHealth)
	http.HandleFunc("/api/omni/install", handleOmniInstall)
	http.HandleFunc("/api/omni/update", handleOmniUpdate)
	http.HandleFunc("/api/omni/repair", handleOmniRepair)
	http.HandleFunc("/api/omni/reinstall", handleOmniReinstall)

	http.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		lastStr := r.URL.Query().Get("last")
		lastIdx, _ := strconv.Atoi(lastStr)

		logMutex.Lock()
		defer logMutex.Unlock()

		total := len(logBuffer)
		if lastIdx > total {
			lastIdx = 0 
		}

		newLogs := logBuffer[lastIdx:]
		json.NewEncoder(w).Encode(map[string]interface{}{
			"logs":     newLogs,
			"newIndex": total,
		})
	})

	http.HandleFunc("/api/file-logs", func(w http.ResponseWriter, r *http.Request) {
		logMutex.Lock()
		defer logMutex.Unlock()
		data, err := os.ReadFile(getLogPath())
		if err != nil {
			w.Write([]byte("Henüz log dosyası oluşturulmadı."))
			return
		}
		w.Write(data)
	})

	log.Fatal(http.ListenAndServe(":20127", nil))
}

func updateTrayTexts() {
	cfg := loadConfig()
	t := loadTranslations(cfg.Language)
	if mOpen != nil { mOpen.SetTitle(t["TrayOpenPanel"]) }
	if mOpenOmni != nil { mOpenOmni.SetTitle(t["TrayGoOmni"]) }
	if mAutoStart != nil { mAutoStart.SetTitle(t["TrayAutoStart"]) }
	if mQuit != nil { mQuit.SetTitle(t["TrayQuit"]) }
}

func main() {
	loadConfig()
	initFileLog()
	tInit := loadTranslations(currentLang)
	writeLog("%s", tInit["LogStarted"])
	
	startWatchdog()

	go startWebServer()
	systray.Run(onReady, onExit)
}

func onReady() {
	iconBytes, err := os.ReadFile(getIconPath())
	if err != nil {
		defaultIconBase64 := "AAABAAEAEBAAAAEAIABoBAAAFgAAACgAAAAQAAAAIAAAAAEAIAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAgICA/wAAAP8AAAD/AAAA/wAAAP8AAAD/AAAA/wAAAP8AAAD/AAAA/wAAAP8AAAD/AAAA/wAAAP8CAgL/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
		iconBytes, _ = base64.StdEncoding.DecodeString(defaultIconBase64)
	}
	systray.SetIcon(iconBytes)
	systray.SetTitle("Omniroute")
	
	cfg := loadConfig()
	t := loadTranslations(cfg.Language)
	systray.SetTooltip(t["TrayTooltip"])

	mOpen = systray.AddMenuItem(t["TrayOpenPanel"], "")
	mOpenOmni = systray.AddMenuItem(t["TrayGoOmni"], "")
	mOpenOmni.Disable()
	
	systray.AddSeparator()
	mToggle = systray.AddMenuItem(t["TrayStart"], "")
	systray.AddSeparator()
	mAutoStart = systray.AddMenuItemCheckbox(t["TrayAutoStart"], "", isAutoStartEnabled())
	systray.AddSeparator()
	mQuit = systray.AddMenuItem(t["TrayQuit"], "")

	go func() {
		var wasRunning bool = false
		var wasDisabled bool = false
		for {
			time.Sleep(500 * time.Millisecond)
			
			cmdMutex.Lock()
			isRunning := (cmd != nil && cmd.Process != nil)
			cmdMutex.Unlock()
			installed := isOmnirouteDir(getOmniroutePathEnhanced())
			opRunning := isOmniOpRunning()
			shouldDisable := !installed || opRunning

			if shouldDisable != wasDisabled {
				if shouldDisable {
					mToggle.Disable()
					mOpenOmni.Disable()
				} else {
					mToggle.Enable()
					// mOpenOmni state depends on isRunning
					if isRunning {
						mOpenOmni.Enable()
					} else {
						mOpenOmni.Disable()
					}
				}
				wasDisabled = shouldDisable
			}
			if shouldDisable {
				// keep title as "Kurulum bekleniyor" when disabled? use current lang
				cNow := loadConfig()
				tNow := loadTranslations(cNow.Language)
				if !installed {
					mToggle.SetTitle(tNow["TrayInstallOmni"])
				} else if opRunning {
					mToggle.SetTitle(tNow["TrayInstalling"])
				}
				continue
			}

			cNow := loadConfig()
			tNow := loadTranslations(cNow.Language)
			if isRunning != wasRunning {
				if isRunning {
					mToggle.SetTitle(tNow["TrayStop"])
					mOpenOmni.Enable()
				} else {
					mToggle.SetTitle(tNow["TrayStart"])
					mOpenOmni.Disable()
				}
				wasRunning = isRunning
			}
		}
	}()

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser("http://localhost:20127")
			case <-mOpenOmni.ClickedCh:
				openBrowser("http://localhost:20128")
			case <-mToggle.ClickedCh:
				if !isOmnirouteDir(getOmniroutePathEnhanced()) || isOmniOpRunning() {
					continue
				}
				cmdMutex.Lock()
				isRunning := (cmd != nil && cmd.Process != nil)
				cmdMutex.Unlock()
				
				cNow := loadConfig()
				tNow := loadTranslations(cNow.Language)

				if isRunning {
					if showConfirmDialog(tNow["DialogStopTitle"], tNow["DialogStopMsg"]) {
						stopOmniroute()
					}
				} else {
					startOmniroute()
				}
			case <-mAutoStart.ClickedCh:
				newState := !isAutoStartEnabled()
				setAutoStart(newState)
				if newState {
					mAutoStart.Check()
				} else {
					mAutoStart.Uncheck()
				}
			case <-mQuit.ClickedCh:
				cNow := loadConfig()
				tNow := loadTranslations(cNow.Language)
				if showConfirmDialog(tNow["DialogQuitTitle"], tNow["DialogQuitMsg"]) {
					systray.Quit()
				}
			}
		}
	}()
}

func onExit() {
	t := loadTranslations(currentLang)
	writeLog("=== %s ===", t["TrayQuit"])
	stopOmniroute()
}
