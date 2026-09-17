package main

import (
	"bufio"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
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

//go:embed web themes locales app.ico icon.png favicon.svg
var webFS embed.FS

// --- EVRENSEL VE PLATFORMA DUYARLI YOL YAPILANDIRMASI ---
func getOmniroutePath() string {
	return getOmniroutePathEnhanced()
}

// nodeSearchRoots lists directories probed for node.exe when PATH resolution
// fails (logon/Run-key launches inherit a narrow environment).
func nodeSearchRoots() []string {
	var roots []string
	if v := os.Getenv("ProgramFiles"); v != "" {
		roots = append(roots, filepath.Join(v, "nodejs"))
	}
	if v := os.Getenv("ProgramFiles(x86)"); v != "" {
		roots = append(roots, filepath.Join(v, "nodejs"))
	}
	if v := os.Getenv("ProgramData"); v != "" {
		roots = append(roots, filepath.Join(v, "chocolatey", "bin"))
	}
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		roots = append(roots,
			filepath.Join(v, "Programs", "nodejs"),
			filepath.Join(v, "Volta", "bin"),
		)
	}
	if v := os.Getenv("USERPROFILE"); v != "" {
		roots = append(roots, filepath.Join(v, "scoop", "shims"))
	}
	if v := os.Getenv("APPDATA"); v != "" {
		roots = append(roots, filepath.Join(v, "fnm", "node-versions"))
	}
	if v := os.Getenv("NVM_HOME"); v != "" {
		roots = append(roots, v)
	}
	if v := os.Getenv("NVM_SYMLINK"); v != "" {
		roots = append(roots, v)
	}
	if v := os.Getenv("APPDATA"); v != "" {
		roots = append(roots, filepath.Join(v, "nvm"))
	}
	return roots
}

// resolveNodePath returns an absolute node executable path. It tries PATH
// first (node, nodejs), then the standard install roots. Search roots are
// injectable so tests do not depend on this machine.
func resolveNodePath(roots []string) (string, error) {
	if roots == nil {
		for _, name := range []string{"node", "nodejs"} {
			if p, err := exec.LookPath(name); err == nil {
				if abs, err := filepath.Abs(p); err == nil {
					p = abs
				}
				return p, nil
			}
		}
		roots = nodeSearchRoots()
	}
	var searched []string
	for _, dir := range roots {
		cand := filepath.Join(dir, "node.exe")
		searched = append(searched, cand)
		if runtime.GOOS != "windows" {
			cand = filepath.Join(dir, "node")
			searched = append(searched, cand)
		}
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(cand), ".exe") {
				continue
			}
			return cand, nil
		}
	}
	return "", fmt.Errorf("node not found (PATH plus %d locations)", len(searched))
}

var nodeResolvedLogged bool
var (
	OmniroutePath = getOmniroutePath()
	StartCommand  = "node"
	StartArgs     = filepath.Join(OmniroutePath, "bin", "omniroute.mjs")
)

const AppName = "OmniroutePanel"
var AppVersion = "dev"

func init() {
	if AppVersion == "dev" {
		AppVersion = "0.0.0-dev"
	}
}
const LogFileName = "panel.log"
const ConfigFileName = "config.json"
const LocalesDir = "locales"
const OmniPort = 20128
const PanelPort = 20127
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
	fileLogMu         sync.Mutex
	isIntentionalStop bool
	stopMu            sync.RWMutex
	currentLang       = "tr"
	langMu            sync.RWMutex
	configMutex       sync.Mutex
	watchdogFailCount int
	watchdogLastFail  time.Time
	crashBackoffUntil time.Time
)

// Lock ordering (never invert): cmdMutex -> configMutex -> langMu/stopMu -> logMutex -> fileLogMu.
// logMutex and fileLogMu are leaf locks: writeLog takes them briefly, and no path acquires
// cmdMutex/configMutex/langMu/stopMu while holding logMutex/fileLogMu. Calling writeLog while
// holding cmdMutex is allowed; the cmd.Wait goroutine still avoids it (captures state, unlocks, then logs).
// omniStartGraceUntil and cmd belong to the cmdMutex group: always read/write them under cmdMutex.

func getCurrentLang() string {
	langMu.RLock()
	defer langMu.RUnlock()
	return currentLang
}

func setCurrentLang(lang string) {
	langMu.Lock()
	defer langMu.Unlock()
	currentLang = lang
}

func getIntentionalStop() bool {
	stopMu.RLock()
	defer stopMu.RUnlock()
	return isIntentionalStop
}

func setIntentionalStop(v bool) {
	stopMu.Lock()
	defer stopMu.Unlock()
	isIntentionalStop = v
}

func setMenuChecked(m *systray.MenuItem, on bool) {
	if m == nil {
		return
	}
	if on {
		m.Check()
	} else {
		m.Uncheck()
	}
}

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

func processCommandLine(pid int) (string, bool) {
	if runtime.GOOS == "windows" {
		psCmd := fmt.Sprintf("Get-CimInstance Win32_Process -Filter 'ProcessId=%d' | Select-Object -ExpandProperty CommandLine", pid)
		c := exec.Command("powershell", "-NoProfile", "-Command", psCmd)
		hideWindow(c)
		out, err := c.Output()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(out)), true
	}
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		cmdline := strings.ReplaceAll(string(data), "\x00", " ")
		return strings.TrimSpace(cmdline), true
	}
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

var processCommandLineFn = processCommandLine

func ownedByOmniroute(pid int) bool {
	cmdline, ok := processCommandLineFn(pid)
	if !ok || strings.TrimSpace(cmdline) == "" {
		return false
	}
	// Same values startOmniroute spawns (StartCommand + StartArgs = OmniroutePath/bin/omniroute.mjs).
	base := strings.TrimSpace(OmniroutePath)
	if base == "" {
		return false
	}
	lower := strings.ToLower(cmdline)
	if strings.Contains(lower, "omniroute.mjs") {
		return true
	}
	return strings.Contains(lower, strings.ToLower(base))
}

func killOwnedPid(pid int) bool {
	if pid == os.Getpid() || pid < 100 {
		return false
	}
	if !ownedByOmniroute(pid) {
		return false
	}
	if runtime.GOOS == "windows" {
		c := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
		hideWindow(c)
		return c.Run() == nil
	}
	return exec.Command("kill", "-9", strconv.Itoa(pid)).Run() == nil
}

func collectPortPids(port int) []int {
	var pids []int
	seen := map[int]bool{}
	add := func(pid int) {
		if pid == os.Getpid() || pid < 100 || seen[pid] {
			return
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	if runtime.GOOS == "windows" {
		// Prefer PowerShell Get-NetTCPConnection
		psCmd := fmt.Sprintf("Get-NetTCPConnection -LocalPort %d -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique", port)
		cmd := exec.Command("powershell", "-NoProfile", "-Command", psCmd)
		hideWindow(cmd)
		if out, err := cmd.Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				pidStr := strings.TrimSpace(line)
				if pidStr == "" {
					continue
				}
				if pid, err := strconv.Atoi(pidStr); err == nil {
					add(pid)
				}
			}
		}
		if len(pids) > 0 {
			return pids
		}
		// fallback: netstat
		cmd2 := exec.Command("cmd", "/c", fmt.Sprintf("netstat -ano | findstr :%d", port))
		hideWindow(cmd2)
		if out2, err := cmd2.Output(); err == nil {
			for _, line := range strings.Split(string(out2), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 0 {
					continue
				}
				if pid, err := strconv.Atoi(fields[len(fields)-1]); err == nil {
					add(pid)
				}
			}
		}
		return pids
	}
	// macOS / Linux
	if out, err := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti:%d 2>/dev/null", port)).Output(); err == nil {
		for _, pidStr := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			pidStr = strings.TrimSpace(pidStr)
			if pidStr == "" {
				continue
			}
			if pid, err := strconv.Atoi(pidStr); err == nil {
				add(pid)
			}
		}
	}
	return pids
}

func killPortHolders(port int) int {
	killed := 0
	for _, pid := range collectPortPids(port) {
		if !ownedByOmniroute(pid) {
			writeLog("WARN: pid %d on port %d owner unverified, leaving process running", pid, port)
			continue
		}
		if killOwnedPid(pid) {
			killed++
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
		writeLog("WARN: port %d still in use, owner unverified, leaving process running", port)
	}
	return !isPortInUse(port)
}

func appendLogBufferLocked(line string) {
	logBuffer = append(logBuffer, line)
	if len(logBuffer) > maxLogSize {
		logBuffer = logBuffer[1:]
	}
}

func writeFileLogLine(line string) {
	fileLogMu.Lock()
	defer fileLogMu.Unlock()
	if fileLogWriter == nil {
		return
	}
	if _, err := fileLogWriter.WriteString(line + "\n"); err != nil {
		return
	}
	if info, err := fileLogWriter.Stat(); err == nil && info.Size() > logMaxBytes {
		// async rotate to avoid blocking
		go rotateLogIfNeeded()
	}
}

func rotateLogIfNeeded() {
	fileLogMu.Lock()
	defer fileLogMu.Unlock()
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
	fileLogWriter = nil
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
	fileLogMu.Lock()
	defer fileLogMu.Unlock()
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
	if fileLogWriter != nil {
		fileLogWriter.Close()
		fileLogWriter = nil
	}
	fileLogWriter, err = os.OpenFile(lp, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Log dosyası açılamadı: %v\n", err)
	}
}

func cleanOldLogs() {
	cfg := loadConfig()
	hours := cfg.LogRetentionHours
	if hours <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	logMutex.Lock()
	defer logMutex.Unlock()
	cleaned := 0
	for len(logBuffer) > 0 {
		line := logBuffer[0]
		ts := parseLogTimestamp(line)
		if !ts.IsZero() && ts.Before(cutoff) {
			logBuffer = logBuffer[1:]
			cleaned++
		} else {
			break
		}
	}
	if cleaned > 0 {
		fmt.Printf("  Log cleaned: %d entries older than %dh removed\n", cleaned, hours)
	}
}

func parseLogTimestamp(line string) time.Time {
	if len(line) < 21 {
		return time.Time{}
	}
	if line[0] != '[' {
		return time.Time{}
	}
	ts, err := time.Parse("2006-01-02 15:04:05", line[1:20])
	if err != nil {
		return time.Time{}
	}
	return ts
}

func startLogCleanup() {
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			cleanOldLogs()
		}
	}()
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
			appendLogBufferLocked(ts)
			logMutex.Unlock()
			writeFileLogLine(ts)
		}
	} else if strings.Contains(msg, "OmniRoute is running") {
		logMutex.Lock()
		watchdogFailCount = 0
		crashBackoffUntil = time.Time{}
		logMutex.Unlock()
	}

	logMutex.Lock()
	appendLogBufferLocked(timestamped)
	logMutex.Unlock()

	writeFileLogLine(timestamped)
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
	setIntentionalStop(false)
	t := loadTranslations(getCurrentLang())
	nodePath, nodeErr := resolveNodePath(nil)
	if nodeErr != nil {
		writeLog("ERROR: node not found in PATH or standard locations, OmniRoute start skipped")
		probeMu.Lock()
		probeStatus = "unreachable"
		probeMu.Unlock()
		return
	}
	if !nodeResolvedLogged {
		nodeResolvedLogged = true
		writeLog("INFO: node resolved: %s", nodePath)
	}
	writeLog("%s", t["LogStarting"])

	cmd = exec.Command(nodePath, StartArgs, "--no-open", "--no-tray")
	cmd.Dir = OmniroutePath

	cmd.Env = append(os.Environ(),
		"CI=true",
		"BROWSER=none",
		"NONINTERACTIVE=true",
		"PATH="+filepath.Dir(nodePath)+string(os.PathListSeparator)+os.Getenv("PATH"),
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
	started := cmd
	omniStartGraceUntil = time.Now().Add(30 * time.Second)

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

	go handleChildExit(started)
}

func handleChildExit(c *exec.Cmd) {
	if c == nil {
		return
	}
	c.Wait()
	var key string
	cmdMutex.Lock()
	unexpected := !getIntentionalStop()
	if unexpected {
		key = "LogUnexpectedStop"
	} else {
		key = "LogStoppedInfo"
	}
	lang := getCurrentLang()
	if cmd == c {
		cmd = nil
	}
	cmdMutex.Unlock()
	tSub := loadTranslations(lang)
	writeLog("%s", tSub[key])
}

func stopOmniroute() {
	cmdMutex.Lock()
	defer cmdMutex.Unlock()

	setIntentionalStop(true)
	logMutex.Lock()
	watchdogFailCount = 0
	crashBackoffUntil = time.Time{}
	logMutex.Unlock()
	t := loadTranslations(getCurrentLang())
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
			writeLog("WARN: port %d still in use, owner unverified, leaving process running", OmniPort)
		}
	}
}

var (
	probeMu             sync.Mutex
	probeFailures       int
	probeDegradedShown  bool
	externalAdopted     bool
	probeStatus         = "unknown"
	probeAt             time.Time
	probeRecovering     bool
	probeRecoveries     int
	probePortBusyLogged bool
	skipLogged          bool
	skipReason          string
	omniStartGraceUntil time.Time
	panelStartAt        time.Time
)

type watchdogAction int

const (
	watchdogNone watchdogAction = iota
	watchdogAdoptExternal
	watchdogWait
	watchdogRestart
	watchdogRestartOwnChild
	watchdogSkip
)
type watchdogState struct {
	probe              probeOutcome
	childAlive         bool
	failures           int
	intentionalStop    bool
	opRunning          bool
	backoffActive      bool
	installed          bool
	externalAdopted    bool
	wasDegraded        bool
	inGrace            bool
	startupPhase       bool
	portBusy           bool
}

func decideWatchdogAction(s watchdogState) (watchdogAction, string) {
	if !s.installed {
		return watchdogSkip, "not installed"
	}
	if s.intentionalStop {
		return watchdogSkip, "intentional stop"
	}
	if s.opRunning {
		return watchdogSkip, "op running"
	}
	if s.backoffActive {
		return watchdogSkip, "backoff active"
	}
	if s.inGrace && s.probe == probeDown {
		return watchdogSkip, "starting"
	}
	switch s.probe {
	case probeHealthy:
		if !s.childAlive && !s.externalAdopted {
			return watchdogAdoptExternal, ""
		}
		return watchdogNone, ""
	case probeDegraded:
		return watchdogNone, ""
	default:
		if s.startupPhase && !s.childAlive && !s.externalAdopted && !s.portBusy {
			return watchdogRestart, ""
		}
		if s.startupPhase && !s.childAlive && !s.externalAdopted && s.portBusy {
			return watchdogWait, "port busy, waiting"
		}
		if s.failures+1 >= 2 {
			if s.childAlive {
				return watchdogRestartOwnChild, ""
			}
			return watchdogRestart, ""
		}
		return watchdogWait, ""
	}
}
type probeSnapshot struct {
	failures        int
	degradedShown   bool
	externalAdopted bool
	status          string
	recovering      bool
	recoveries      int
	inGrace         bool
	portBusyLogged  bool
	skipLogged      bool
	skipReason      string
}

type probeTickResult struct {
	snap           probeSnapshot
	action         watchdogAction
	reason         string
	statusCode     int
	logDegraded    bool
	logSkip        bool
	logAdopt       bool
	logPortBusy    bool
	requestBackoff bool
}

func applyProbeTick(prev probeSnapshot, st watchdogState, statusCode int) probeTickResult {
	act, reason := decideWatchdogAction(st)
	r := probeTickResult{action: act, reason: reason, statusCode: statusCode}
	snap := prev
	if act != watchdogSkip {
		snap.skipLogged = false
		snap.skipReason = ""
	}
	switch act {
	case watchdogNone:
		if st.probe == probeHealthy {
			snap.failures = 0
			snap.degradedShown = false
			snap.status = "healthy"
			snap.recovering = false
			snap.recoveries = 0
			snap.inGrace = false
			snap.portBusyLogged = false
		} else if st.probe == probeDegraded {
			snap.status = "degraded"
			if !prev.degradedShown {
				snap.degradedShown = true
				r.logDegraded = true
			}
		}
	case watchdogAdoptExternal:
		snap.failures = 0
		snap.degradedShown = false
		snap.externalAdopted = true
		snap.status = "healthy"
		snap.recoveries = 0
		snap.inGrace = false
		snap.portBusyLogged = false
		r.logAdopt = true
	case watchdogWait:
		snap.failures = prev.failures + 1
		snap.status = "unreachable"
		if st.portBusy && !prev.portBusyLogged {
			snap.portBusyLogged = true
			r.logPortBusy = true
		}
		snap.recovering = false
		if st.inGrace && st.probe == probeDown {
			snap.status = "starting"
			snap.failures = 0
			if reason != "" && !prev.skipLogged {
				snap.skipLogged = true
				snap.skipReason = reason
				r.logSkip = true
			}
			break
		}
	case watchdogRestart, watchdogRestartOwnChild:
		snap.failures = 0
		snap.recovering = true
		snap.status = "unreachable"
		if act == watchdogRestart && st.startupPhase {
			snap.status = "starting"
		}
		snap.recoveries = prev.recoveries + 1
		if snap.recoveries >= 4 {
			r.action = watchdogSkip
			r.reason = "repeated recovery attempts"
			r.requestBackoff = true
		}
	case watchdogSkip:
		snap.recovering = false
		if st.inGrace && st.probe == probeDown {
			snap.status = "starting"
			snap.failures = 0
			if reason != "" && !prev.skipLogged {
				snap.skipLogged = true
				snap.skipReason = reason
				r.logSkip = true
			}
			break
		}
		if reason != "" && !prev.skipLogged {
			snap.skipLogged = true
			snap.skipReason = reason
			r.logSkip = true
		}
	}
	r.snap = snap
	return r
}

func startWatchdog() {
	go func() {
		panelStartAt = time.Now()
		for {
			time.Sleep(3 * time.Second)
			res := probeOmniHealth(3 * time.Second)
			cmdMutex.Lock()
			childAlive := cmd != nil && cmd.Process != nil
			graceUntil := omniStartGraceUntil
			cmdMutex.Unlock()
			inGrace := !graceUntil.IsZero() && time.Now().Before(graceUntil)
			installed := isOmnirouteDir(getOmniroutePathEnhanced())
			if !installed || !childAlive {
				inGrace = false
			}
			portBusy := isPortInUse(OmniPort)
			startupPhase := time.Since(panelStartAt) < 60*time.Second
			logMutex.Lock()
			bb := crashBackoffUntil
			probeMu.Lock()
			failures := probeFailures
			adopted := externalAdopted
			wasDegraded := probeDegradedShown
			probeMu.Unlock()
			backoffActive := !bb.IsZero() && time.Now().Before(bb)
			failCnt := watchdogFailCount
			logMutex.Unlock()
			st := watchdogState{
				probe:           res.outcome,
				childAlive:      childAlive,
				failures:        failures,
				intentionalStop: getIntentionalStop(),
				opRunning:       isOmniOpRunning(),
				backoffActive:   backoffActive,
				installed:       installed,
				externalAdopted: adopted,
				wasDegraded:     wasDegraded,
				inGrace:         inGrace,
				startupPhase:    startupPhase,
				portBusy:        portBusy,
			}
			probeMu.Lock()
			prev := probeSnapshot{
				failures:        probeFailures,
				degradedShown:   probeDegradedShown,
				externalAdopted: externalAdopted,
				status:          probeStatus,
				recovering:      probeRecovering,
				recoveries:      probeRecoveries,
				portBusyLogged:  probePortBusyLogged,
				skipLogged:      skipLogged,
				skipReason:      skipReason,
			}
			probeMu.Unlock()
			tick := applyProbeTick(prev, st, res.statusCode)
			probeMu.Lock()
			probeFailures = tick.snap.failures
			probeDegradedShown = tick.snap.degradedShown
			externalAdopted = tick.snap.externalAdopted
			probeStatus = tick.snap.status
			probeAt = time.Now()
			probeRecovering = tick.snap.recovering
			probeRecoveries = tick.snap.recoveries
			probePortBusyLogged = tick.snap.portBusyLogged
			skipLogged = tick.snap.skipLogged
			skipReason = tick.snap.skipReason
			probeMu.Unlock()
			if tick.requestBackoff {
				logMutex.Lock()
				now := time.Now()
				watchdogFailCount++
				watchdogLastFail = now
				backoff := time.Duration(30*(1<<min(watchdogFailCount-1, 3))) * time.Second
				if backoff > 300*time.Second {
					backoff = 300 * time.Second
				}
				crashBackoffUntil = now.Add(backoff)
				probeMu.Lock()
				probeRecovering = false
				probeMu.Unlock()
				logMutex.Unlock()
				writeLog("WARN: repeated recovery attempts (%d), backing off %v", tick.snap.recoveries, backoff.Round(time.Second))
			}
			if tick.logDegraded {
				writeLog("WARN: OmniRoute reachable but unhealthy (status %d), waiting", res.statusCode)
				continue
			}
			if tick.logPortBusy {
				writeLog("WARN: OmniRoute port %d busy but health not answering yet, waiting", OmniPort)
				continue
			}
			if tick.logSkip {
				writeLog("INFO: watchdog skip: %s", tick.reason)
				continue
			}
			if tick.logAdopt {
				writeLog("INFO: external OmniRoute instance detected, adopting (no duplicate spawn)")
			}
			switch tick.action {
			case watchdogRestartOwnChild:
				stopOmniroute()
				startOmniroute()
				probeMu.Lock()
				externalAdopted = false
				probeMu.Unlock()
			case watchdogRestart:
				if !backoffActive && installed {
					if isPortInUse(OmniPort) {
						if !ensurePortFree(OmniPort) {
							writeLog("WARN: Port %d dolu, watchdog beklemede (fail #%d)", OmniPort, failCnt+1)
							logMutex.Lock()
							watchdogFailCount++
							watchdogLastFail = time.Now()
							logMutex.Unlock()
							continue
						}
					}
					t := loadTranslations(getCurrentLang())
					writeLog("%s", t["LogWatchdog"])
					startOmniroute()
				}
				probeMu.Lock()
				externalAdopted = false
				probeMu.Unlock()
			}
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

var autostartApply = setAutoStart
var autostartEnabled = isAutoStartEnabled

func newPanelMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		if data, err := fs.ReadFile(webFS, "app.ico"); err == nil {
			w.Header().Set("Content-Type", "image/x-icon")
			w.Write(data)
			return
		}
		http.ServeFile(w, r, getIconPath())
	})

	mux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		if data, err := fs.ReadFile(webFS, "favicon.svg"); err == nil {
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Write(data)
			return
		}
		http.ServeFile(w, r, filepath.Join(exeDir(), "favicon.svg"))
	})

	themesFS, _ := fs.Sub(webFS, "themes")
	mux.Handle("/themes/", http.StripPrefix("/themes/", http.FileServer(http.FS(themesFS))))

	// web static (js/css) - use ReadFile fallback approach
	staticFS, _ := fs.Sub(webFS, "web/static")
	mux.Handle("/web/static/", http.StripPrefix("/web/static/", http.FileServer(http.FS(staticFS))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) { startOmniroute() })
	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) { stopOmniroute() })
	mux.HandleFunc("/api/restart", func(w http.ResponseWriter, r *http.Request) {
		stopOmniroute()
		time.Sleep(1 * time.Second)
		startOmniroute()
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		cmdMutex.Lock()
		isRunning := (cmd != nil && cmd.Process != nil)
		cmdMutex.Unlock()
		json.NewEncoder(w).Encode(StatusResponse{IsRunning: isRunning})
	})

	mux.HandleFunc("/api/autostart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req AutoStartResponse
			json.NewDecoder(r.Body).Decode(&req)
			autostartApply(req.IsEnabled)
			setMenuChecked(mAutoStart, req.IsEnabled)
			return
		}
		json.NewEncoder(w).Encode(AutoStartResponse{IsEnabled: autostartEnabled()})
	})
	mux.HandleFunc("/api/language", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/api/theme", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/api/omni/health", handleOmniHealth)
	mux.HandleFunc("/api/omni/install", handleOmniInstall)
	mux.HandleFunc("/api/omni/update", handleOmniUpdate)
	mux.HandleFunc("/api/omni/repair", handleOmniRepair)
	mux.HandleFunc("/api/omni/reinstall", handleOmniReinstall)

	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/api/file-logs", func(w http.ResponseWriter, r *http.Request) {
		logMutex.Lock()
		defer logMutex.Unlock()
		data, err := os.ReadFile(getLogPath())
		if err != nil {
			w.Write([]byte(""))
			return
		}
		w.Write(data)
	})

	// Settings endpoints
	mux.HandleFunc("/api/settings/log-retention", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req struct {
				Hours int `json:"hours"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			if req.Hours < 0 {
				req.Hours = 0
			}
			configMutex.Lock()
			cfg := loadConfigUnlocked()
			cfg.LogRetentionHours = req.Hours
			data, _ := json.MarshalIndent(cfg, "", "  ")
			os.WriteFile(getConfigPath(), data, 0644)
			configMutex.Unlock()
			return
		}
		cfg := loadConfig()
		json.NewEncoder(w).Encode(map[string]int{"hours": cfg.LogRetentionHours})
	})

	mux.HandleFunc("/api/settings/clear-logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		logMutex.Lock()
		logBuffer = nil
		logMutex.Unlock()
		os.Remove(getLogPath())
		initFileLog()
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/settings/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defaultCfg := Config{Language: "", AutoStart: false, Theme: ThemeSystem, LogRetentionHours: 24}
		data, _ := json.MarshalIndent(defaultCfg, "", "  ")
		os.WriteFile(getConfigPath(), data, 0644)
		setCurrentLang("")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Update endpoints
	mux.HandleFunc("/api/update/check", handleCheckUpdate)
	mux.HandleFunc("/api/update/install", handlePerformUpdate)
	mux.HandleFunc("/api/update/status", handleUpdateStatus)

	return mux
}

func startWebServer() {
	mux := newPanelMux()
	addr := "127.0.0.1:" + strconv.Itoa(PanelPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		writeLog("ERROR: panel listen failed addr=%s err=%v", addr, err)
		fmt.Fprintf(os.Stderr, "ERROR: panel listen failed addr=%s err=%v\n", addr, err)
		os.Exit(1)
	}
	if err := http.Serve(ln, guardMiddleware(mux)); err != nil {
		writeLog("ERROR: panel listen failed addr=%s err=%v", addr, err)
		fmt.Fprintf(os.Stderr, "ERROR: panel listen failed addr=%s err=%v\n", addr, err)
		os.Exit(1)
	}
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
	startLogCleanup()
	tInit := loadTranslations(getCurrentLang())
	writeLog("%s", tInit["LogStarted"])

	args := os.Args[1:]

	// Direct mode flags
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			showBanner()
			fmt.Println("  Usage: orpanel [options]")
			fmt.Println()
			fmt.Println("  Options:")
			fmt.Println("    --web      Open web UI in browser")
			fmt.Println("    --tray     Start in system tray (background)")
			fmt.Println("    --update   Check and install updates")
			fmt.Println("    --version  Show version")
			fmt.Println("    --help     Show this help")
			fmt.Println()
			fmt.Println("  No flags: interactive CLI menu")
			return
		case "--version", "-v":
			fmt.Printf("orpanel v%s\n", AppVersion)
			return
		case "--web":
			startWatchdog()
			go startWebServer()
			fmt.Printf("Server: http://localhost:20127\n")
			openBrowser("http://localhost:20127")
			// Keep alive
			select {}
		case "--tray":
			detachFromConsole()
			startWatchdog()
			go startWebServer()
			systray.Run(onReady, onExit)
			return
		case "--update":
			info := checkForUpdate()
			if !info.UpdateAvailable {
				fmt.Printf("Zaten güncel: v%s\n", info.CurrentVersion)
				return
			}
			fmt.Printf("Güncelleme mevcut: v%s → v%s\n", info.CurrentVersion, info.LatestVersion)
			fmt.Println("İndiriliyor...")
			if err := performUpdate(); err != nil {
				fmt.Printf("Hata: %v\n", err)
				return
			}
			fmt.Println("Güncelleme tamamlandı!")
			return
		}
	}

	// No flags: interactive CLI menu.
	// If stdin is not a terminal (double-click, hidden window), start tray directly.
	if !isTerminal() {
		detachFromConsole()
		startWatchdog()
		go startWebServer()
		systray.Run(onReady, onExit)
		return
	}
	runCLI()
}

func isTerminal() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func onReady() {
	iconBytes, err := fs.ReadFile(webFS, "app.ico")
	if err != nil {
		iconBytes, err = os.ReadFile(getIconPath())
	}
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
				newState := !autostartEnabled()
				autostartApply(newState)
				setMenuChecked(mAutoStart, newState)
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
	t := loadTranslations(getCurrentLang())
	writeLog("=== %s ===", t["TrayQuit"])
	stopOmniroute()
}
