package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	"text/template"
	"time"

	"github.com/getlantern/systray"
)

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
const LogFileName = "panel.log"
const ConfigFileName = "config.json"
const LocalesDir = "locales"
const OmniPort = 20128
const logMaxBytes = 5 * 1024 * 1024
// ----------------------------------

const ThemeLight = "light"
const ThemeDark = "dark"
const ThemeSystem = "system"

type Config struct {
	Language  string `json:"language"`
	AutoStart bool   `json:"auto_start"`
	Theme     string `json:"theme"`
}

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

func isValidTheme(t string) bool {
	return t == ThemeLight || t == ThemeDark || t == ThemeSystem
}

func loadConfigUnlocked() Config {
	// try exeDir first, then cwd for backward compat
	cfgPath := getConfigPath()
	file, err := os.ReadFile(cfgPath)
	if err != nil {
		// fallback to cwd
		if cfgPath != ConfigFileName {
			file, err = os.ReadFile(ConfigFileName)
		}
	}
	if err != nil {
		return Config{Language: "tr", AutoStart: false, Theme: ThemeSystem}
	}
	var cfg Config
	if err := json.Unmarshal(file, &cfg); err != nil {
		return Config{Language: "tr", AutoStart: false, Theme: ThemeSystem}
	}
	if cfg.Language == "" {
		cfg.Language = "tr"
	}
	if !isValidTheme(cfg.Theme) {
		cfg.Theme = ThemeSystem
	}
	if cfg.Language != "" {
		currentLang = cfg.Language
	}
	return cfg
}

func loadConfig() Config {
	configMutex.Lock()
	defer configMutex.Unlock()
	return loadConfigUnlocked()
}

func saveConfig(lang string, autoStart bool) {
	configMutex.Lock()
	defer configMutex.Unlock()
	existing := loadConfigUnlocked()
	if lang != "" {
		currentLang = lang
		existing.Language = currentLang
	} else if existing.Language == "" {
		existing.Language = currentLang
	}
	existing.AutoStart = autoStart
	if !isValidTheme(existing.Theme) {
		existing.Theme = ThemeSystem
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(getConfigPath(), data, 0644)
}

func saveTheme(theme string) error {
	if !isValidTheme(theme) {
		return fmt.Errorf("invalid theme: %s", theme)
	}
	configMutex.Lock()
	defer configMutex.Unlock()
	cfg := loadConfigUnlocked()
	cfg.Theme = theme
	if cfg.Language == "" {
		cfg.Language = currentLang
	}
	if !isValidTheme(cfg.Theme) {
		cfg.Theme = ThemeSystem
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(getConfigPath(), data, 0644)
}

func loadTranslations(lang string) map[string]string {
	locDir := getLocalesDir()
	filePath := filepath.Join(locDir, lang+".json")
	file, err := os.ReadFile(filePath)
	if err != nil {
		file, err = os.ReadFile(filepath.Join(locDir, "tr.json"))
		if err != nil {
			return map[string]string{
				"Title": "Omniroute Control Panel",
			}
		}
	}
	var t map[string]string
	json.Unmarshal(file, &t)
	return t
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

const htmlTemplate = `
<!DOCTYPE html>
<html lang="{{.Lang}}">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.T.Title}}</title>
    <link rel="icon" type="image/x-icon" href="/favicon.ico">
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.css" />
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
    <link href="https://fonts.googleapis.com/css2?family=Material+Symbols+Rounded:opsz,wght,FILL,GRAD@24,400,1,0" rel="stylesheet" />
    <link rel="stylesheet" href="/themes/common.css">
    <link id="theme-variables" rel="stylesheet" href="/themes/dark/variables.css">
    <script>
        (function(){
            var t="{{.Theme}}";
            var e=t;
            if(t==="system"){
                try{ e=window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'; }catch(err){ e='dark'; }
            }
            var l=document.getElementById('theme-variables');
            if(l) l.href='/themes/'+e+'/variables.css';
        })();
    </script>
</head>
<body>

<div class="container">
    <div class="header">
        <div class="header-left">
            <div class="traffic" aria-hidden="true">
                <span class="dot red"></span>
                <span class="dot yellow"></span>
                <span class="dot green"></span>
            </div>
            <div class="brand">
                <div class="brand-icon"><svg width="18" height="18" viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true"><circle cx="16" cy="16" r="3" fill="white"/><circle cx="8" cy="8" r="2" fill="white"/><circle cx="24" cy="8" r="2" fill="white"/><circle cx="8" cy="24" r="2" fill="white"/><circle cx="24" cy="24" r="2" fill="white"/><circle cx="16" cy="5" r="1.5" fill="white"/><circle cx="16" cy="27" r="1.5" fill="white"/><line x1="16" y1="13" x2="8" y2="8" stroke="white" stroke-width="1.2" stroke-linecap="round"/><line x1="16" y1="13" x2="24" y2="8" stroke="white" stroke-width="1.2" stroke-linecap="round"/><line x1="16" y1="19" x2="8" y2="24" stroke="white" stroke-width="1.2" stroke-linecap="round"/><line x1="16" y1="19" x2="24" y2="24" stroke="white" stroke-width="1.2" stroke-linecap="round"/><line x1="16" y1="13" x2="16" y2="5" stroke="white" stroke-width="1.2" stroke-linecap="round"/><line x1="16" y1="19" x2="16" y2="27" stroke="white" stroke-width="1.2" stroke-linecap="round"/></svg></div>
                <h1>{{.T.Header}}</h1>
            </div>
        </div>
        <div id="statusBadge" class="status-badge">
            <span class="material-symbols-rounded">radio_button_unchecked</span>
            <span id="statusText">{{.T.StatusClosed}}</span>
        </div>
    </div>

    <div class="tabs">
        <button class="tab-btn active" onclick="switchTab('terminal-tab', this)">
            <span class="material-symbols-rounded">terminal</span> {{.T.TabTerminal}}
        </button>
        <button class="tab-btn" onclick="switchTab('logs-tab', this)">
            <span class="material-symbols-rounded">description</span> {{.T.TabLogs}}
        </button>
        <button class="tab-btn" onclick="switchTab('settings-tab', this)">
            <span class="material-symbols-rounded">settings</span> {{.T.TabSettings}}
        </button>
    </div>

    <!-- TERMINAL SEKMESİ -->
    <div id="terminal-tab" class="tab-content active">
        <div id="omniHealthCard" class="health-card" style="display:none">
            <div class="health-header">
                <div class="health-title"><span class="material-symbols-rounded" id="healthIcon">hub</span> <span id="healthTitle">OmniRoute Durumu</span></div>
                <span id="healthBadge" class="health-badge">...</span>
            </div>
            <div id="healthBody" class="health-body"></div>
            <div id="healthMeta" class="health-meta"></div>
            <div id="healthActions" class="health-actions"></div>
        </div>
        <div class="controls">
            <button id="btnStart" class="action-btn btn-start" onclick="sendCommand('start')">
                <span class="material-symbols-rounded">play_arrow</span> {{.T.BtnStart}}
            </button>
            <button id="btnStop" class="action-btn btn-stop" onclick="sendCommand('stop')" disabled>
                <span class="material-symbols-rounded">stop</span> {{.T.BtnStop}}
            </button>
            <button id="btnRestart" class="action-btn btn-restart" onclick="sendCommand('restart')" disabled>
                <span class="material-symbols-rounded">refresh</span> {{.T.BtnRestart}}
            </button>
            
            <div style="flex-grow: 1;"></div>
            
            <button id="btnOpenOmni" class="action-btn btn-omni" onclick="window.open('http://localhost:20128', '_blank')" disabled>
                <span class="material-symbols-rounded">open_in_new</span> {{.T.BtnOpenOmni}}
            </button>
        </div>
        <div id="terminal"></div>
    </div>

    <!-- LOGLAR SEKMESİ -->
    <div id="logs-tab" class="tab-content">
        <div id="server-logs" style="font-family: Consolas, monospace; font-size: 13px; color: var(--md-sys-color-logs-fg); overflow-y: auto; white-space: pre-wrap;">{{.T.Loading}}</div>
    </div>

    <!-- AYARLAR SEKMESİ -->
    <div id="settings-tab" class="tab-content">
        <div class="settings-row">
            <div>
                <strong style="font-size: 14px; font-weight: 500;">{{.T.SettingAutoStart}}</strong>
                <div style="font-size: 12px; color: var(--md-sys-color-on-surface-variant); margin-top: 4px;">{{.T.SettingAutoStartDesc}}</div>
            </div>
            <label class="toggle-switch">
                <input type="checkbox" id="autoStartToggle" onchange="toggleAutoStart()">
                <span class="slider"></span>
            </label>
        </div>

        <div class="settings-row">
            <div>
                <strong style="font-size: 14px; font-weight: 500;">{{.T.SettingLang}}</strong>
                <div style="font-size: 12px; color: var(--md-sys-color-on-surface-variant); margin-top: 4px;">{{.T.SettingLangDesc}}</div>
            </div>
            
            <div class="custom-dropdown" id="langDropdown">
                <div class="dropdown-select" onclick="toggleDropdown()">
                    <div class="dropdown-selected-value">
                        <img id="selectedFlag" src="https://flagcdn.com/w40/tr.png" alt="Flag">
                        <span id="selectedText">Türkçe</span>
                    </div>
                    <span class="material-symbols-rounded arrow">expand_more</span>
                </div>
                <div class="dropdown-options" id="dropdownOptions">
                    <div class="dropdown-option" onclick="changeLanguage('tr', 'Türkçe', 'tr')">
                        <img src="https://flagcdn.com/w40/tr.png" alt="TR"> Türkçe
                    </div>
                    <div class="dropdown-option" onclick="changeLanguage('en', 'English', 'gb')">
                        <img src="https://flagcdn.com/w40/gb.png" alt="EN"> English
                    </div>
                    <div class="dropdown-option" onclick="changeLanguage('es', 'Español', 'es')">
                        <img src="https://flagcdn.com/w40/es.png" alt="ES"> Español
                    </div>
                </div>
            </div>
        </div>

        <div class="settings-row">
            <div>
                <strong style="font-size: 14px; font-weight: 500;">{{.T.SettingTheme}}</strong>
                <div style="font-size: 12px; color: var(--md-sys-color-on-surface-variant); margin-top: 4px;">{{.T.SettingThemeDesc}}</div>
            </div>
            <div class="theme-selector" id="themeSelector">
                <button class="theme-option" data-theme="light" onclick="setTheme('light')">
                    <span class="material-symbols-rounded">light_mode</span> {{.T.ThemeLight}}
                </button>
                <button class="theme-option" data-theme="dark" onclick="setTheme('dark')">
                    <span class="material-symbols-rounded">dark_mode</span> {{.T.ThemeDark}}
                </button>
                <button class="theme-option" data-theme="system" onclick="setTheme('system')">
                    <span class="material-symbols-rounded">computer</span> {{.T.ThemeSystem}}
                </button>
            </div>
        </div>
    </div>
</div>

<script src="https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.js"></script>
<script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8.0/lib/xterm-addon-fit.js"></script>
<script>
    function switchTab(tabId, btn) {
        document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
        document.querySelectorAll('.tab-btn').forEach(el => el.classList.remove('active'));
        document.getElementById(tabId).classList.add('active');
        btn.classList.add('active');
        if (tabId === 'terminal-tab') {
            setTimeout(() => fitAddon.fit(), 50);
        } else if (tabId === 'logs-tab') {
            fetchFileLogs();
        }
    }

    const term = new Terminal({
        theme: { background: '#0b0d10', foreground: '#e2e2e6' },
        fontFamily: 'Consolas, "Courier New", monospace',
        fontSize: 13, cursorBlink: true, convertEol: true
    });
    const fitAddon = new FitAddon.FitAddon();
    term.loadAddon(fitAddon);
    term.open(document.getElementById('terminal'));
    setTimeout(() => fitAddon.fit(), 100);
    window.addEventListener('resize', () => fitAddon.fit());

    const statusBadge = document.getElementById('statusBadge');
    const statusText = document.getElementById('statusText');
    const btnStart = document.getElementById('btnStart');
    const btnStop = document.getElementById('btnStop');
    const btnRestart = document.getElementById('btnRestart');
    const btnOpenOmni = document.getElementById('btnOpenOmni');
    const autoStartToggle = document.getElementById('autoStartToggle');
    const serverLogsBox = document.getElementById('server-logs');
    const healthCard = document.getElementById('omniHealthCard');
    const healthBadge = document.getElementById('healthBadge');
    const healthBody = document.getElementById('healthBody');
    const healthMeta = document.getElementById('healthMeta');
    const healthActions = document.getElementById('healthActions');
    const healthIcon = document.getElementById('healthIcon');

    const txtClosed = "{{.T.StatusClosed}}";
    const txtActive = "{{.T.StatusActive}}";

    // --- Theme management ---
    let currentTheme = "{{.Theme}}";
    const themeLink = document.getElementById('theme-variables');

    function getEffectiveTheme(theme) {
        if (theme === 'system') {
            return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
        }
        return theme;
    }

    function applyTheme(theme) {
        currentTheme = theme;
        const effective = getEffectiveTheme(theme);
        if (themeLink) {
            themeLink.href = '/themes/' + effective + '/variables.css';
        }
        document.querySelectorAll('.theme-option').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.theme === theme);
        });
    }

    async function setTheme(theme) {
        applyTheme(theme);
        try {
            await fetch('/api/theme', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ theme })
            });
        } catch(e) {}
    }

    async function loadTheme() {
        try {
            const res = await fetch('/api/theme');
            const data = await res.json();
            if (data.theme && ['light','dark','system'].includes(data.theme)) {
                applyTheme(data.theme);
            } else {
                applyTheme(currentTheme || 'system');
            }
        } catch(e) {
            applyTheme(currentTheme || 'system');
        }
    }

    // Listen for OS theme changes when in system mode
    try {
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
            if (currentTheme === 'system') {
                applyTheme('system');
            }
        });
    } catch(e) {
        // Safari fallback
        try { window.matchMedia('(prefers-color-scheme: dark)').addListener(() => { if(currentTheme==='system') applyTheme('system'); }); } catch(_){}
    }

    // Initialize theme immediately
    applyTheme(currentTheme || 'system');

    // --- OmniRoute health ---
    async function loadOmniHealth() {
        try {
            const res = await fetch('/api/omni/health');
            const h = await res.json();
            renderHealth(h);
        } catch(e) {
            if (healthCard) { healthCard.style.display='none'; }
        }
    }
    function renderHealth(h) {
        if (!healthCard || !h) return;
        healthCard.style.display = 'flex';
        const isEn = "{{.Lang}}" === "en";
        let badgeText = h.status, badgeCls = "error", icon = "warning";
        if (h.status === "running") { badgeText = isEn ? "Running" : "Çalışıyor"; badgeCls = "ok"; icon = "check_circle"; }
        else if (h.status === "stopped") { badgeText = isEn ? "Stopped" : "Durdu"; badgeCls = "warn"; icon = "pause_circle"; }
        else if (h.status === "not_installed") { badgeText = isEn ? "Not installed" : "Kurulu değil"; badgeCls = "error"; icon = "cloud_off"; }
        else if (h.status === "port_conflict") { badgeText = isEn ? "Port conflict" : "Port çakışması"; badgeCls = "error"; icon = "error"; }
        else if (h.status === "corrupt") { badgeText = isEn ? "Corrupt" : "Bozuk"; badgeCls = "error"; icon = "broken_image"; }
        healthBadge.textContent = badgeText;
        healthBadge.className = "health-badge " + badgeCls;
        healthIcon.textContent = icon;

        let body = h.message || "";
        if (h.installed && h.version) {
            body += (isEn ? "<br><small>Installed:</small> <strong>" : "<br><small>Kurulu:</small> <strong>") + h.version + "</strong> <small>(" + h.path + "</small>)";
            if (h.latest) body += " → <strong>" + h.latest + "</strong>";
            if (h.updateAvailable) body += isEn ? " <span style='color:var(--color-warning)'>update available</span>" : " <span style='color:var(--color-warning)'>güncelleme var</span>";
        }
        if (!h.nodeOk) {
            body += "<br>Node: " + (h.nodeVersion || "bulunamadı") + (isEn ? " (<strong>Node 22+ required</strong>)" : " (<strong>Node 22+ gerekli</strong>)");
        } else if (h.nodeVersion) {
            body += "<br>Node " + h.nodeVersion;
        }
        healthBody.innerHTML = body;

        let meta = "";
        if (h.installed) meta += '<span><span class="material-symbols-rounded" style="font-size:14px">folder</span>' + h.path + '</span>';
        meta += '<span><span class="material-symbols-rounded" style="font-size:14px">lan</span>:' + (h.portFree ? (isEn?"free":"serbest") : (isEn?"occupied":"dolu")) + ' 20128</span>';
        if (h.health) meta += '<span>health: ' + h.health + '</span>';
        healthMeta.innerHTML = meta;

        let acts = "";
        const busy = healthActions.dataset.busy === "1";
        if (!h.installed) {
            acts += '<button class="btn-health primary" '+(busy?"disabled":"")+' onclick="doOmniAction(\'install\')"><span class="material-symbols-rounded">download</span> '+(isEn?"Start OmniRoute Install":"OmniRoute Kurulumunu Şimdi Başlat")+'</button>';
        } else if (h.updateAvailable) {
            acts += '<button class="btn-health warning" '+(busy?"disabled":"")+' onclick="doOmniAction(\'update\')"><span class="material-symbols-rounded">system_update</span> '+(isEn?"Update OmniRoute":"OmniRoute\'u Güncelle")+'</button>';
            acts += '<button class="btn-health ghost" '+(busy?"disabled":"")+' onclick="doOmniAction(\'reinstall\')"><span class="material-symbols-rounded">restart_alt</span> '+(isEn?"Reinstall":"Yeniden Kur")+'</button>';
        } else if (h.health==="port_conflict" || h.status==="corrupt") {
            acts += '<button class="btn-health warning" '+(busy?"disabled":"")+' onclick="doOmniAction(\'repair\')"><span class="material-symbols-rounded">build</span> '+(isEn?"Repair OmniRoute":"OmniRoute\'u Onar")+'</button>';
            acts += '<button class="btn-health ghost" '+(busy?"disabled":"")+' onclick="doOmniAction(\'reinstall\')"><span class="material-symbols-rounded">restart_alt</span> '+(isEn?"Reinstall":"Yeniden Kur")+'</button>';
        } else if (h.status==="stopped") {
            acts += '<button class="btn-health ghost" '+(busy?"disabled":"")+' onclick="doOmniAction(\'reinstall\')"><span class="material-symbols-rounded">restart_alt</span> '+(isEn?"Reinstall":"Yeniden Kur")+'</button>';
        } else {
            // running and up to date
            acts += '<button class="btn-health ghost" '+(busy?"disabled":"")+' onclick="doOmniAction(\'reinstall\')"><span class="material-symbols-rounded">restart_alt</span> '+(isEn?"Reinstall":"Yeniden Kur")+'</button>';
        }
        healthActions.innerHTML = acts;
    }
    async function doOmniAction(action) {
        const btns = healthActions.querySelectorAll('button');
        btns.forEach(b=>b.disabled=true);
        healthActions.dataset.busy="1";
        term.write('\r\n\x1b[38;2;99;102;241m> OmniRoute '+action+' başlatılıyor...\x1b[0m\r\n');
        // switch to terminal tab
        const termBtn = document.querySelector('.tab-btn');
        // ensure terminal visible
        document.querySelectorAll('.tab-content').forEach(el=>el.classList.remove('active'));
        document.getElementById('terminal-tab').classList.add('active');
        document.querySelectorAll('.tab-btn').forEach((el,i)=>{ el.classList.toggle('active', i===0); });
        setTimeout(()=>fitAddon.fit(),50);
        try {
            const res = await fetch('/api/omni/'+action, {method:'POST'});
            const j = await res.json().catch(()=>({}));
            if (!res.ok) {
                term.write('\x1b[31mHata: '+ (j.error || res.statusText) +'\x1b[0m\r\n');
                if (j.error && j.error.includes('Node')) term.write('Node.js 22+ kurun: https://nodejs.org/\r\n');
            } else {
                term.write('\x1b[32mİşlem başlatıldı: '+action+'\x1b[0m\r\n');
                term.write('Loglar aşağı akacak, health 3s içinde güncellenecek...\r\n');
            }
        } catch(e) {
            term.write('\x1b[31mİstek hatası: '+e+'\x1b[0m\r\n');
        }
        // re-enable after 3s and refresh health
        setTimeout(async()=>{ healthActions.dataset.busy="0"; await loadOmniHealth(); }, 3000);
    }

    function updateUI(isRunning) {
        if (isRunning) {
            statusText.textContent = txtActive;
            statusBadge.classList.add("active");
            statusBadge.querySelector('.material-symbols-rounded').textContent = "radio_button_checked";
            btnStart.disabled = true; btnStop.disabled = false; btnRestart.disabled = false;
            btnOpenOmni.disabled = false;
        } else {
            statusText.textContent = txtClosed;
            statusBadge.classList.remove("active");
            statusBadge.querySelector('.material-symbols-rounded').textContent = "radio_button_unchecked";
            btnStart.disabled = false; btnStop.disabled = true; btnRestart.disabled = true;
            btnOpenOmni.disabled = true;
        }
    }

    async function sendCommand(cmd) {
        await fetch('/api/' + cmd, { method: 'POST' });
        checkStatus();
    }

    async function checkStatus() {
        const res = await fetch('/api/status');
        const data = await res.json();
        updateUI(data.isRunning);
    }

    async function checkAutoStart() {
        const res = await fetch('/api/autostart');
        const data = await res.json();
        autoStartToggle.checked = data.isEnabled;
    }

    async function toggleAutoStart() {
        const isEnabled = autoStartToggle.checked;
        await fetch('/api/autostart', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ isEnabled })
        });
    }

    function toggleDropdown() {
        document.getElementById('langDropdown').classList.toggle('active');
        document.getElementById('dropdownOptions').classList.toggle('open');
    }

    window.addEventListener('click', function(e) {
        if (!document.getElementById('langDropdown').contains(e.target)) {
            document.getElementById('langDropdown').classList.remove('active');
            document.getElementById('dropdownOptions').classList.remove('open');
        }
    });

    async function loadSettings() {
        const res = await fetch('/api/language');
        const data = await res.json();
        if(data.language) {
            setDropdownUI(data.language);
        }
    }

    function setDropdownUI(lang) {
        let text = 'Türkçe';
        let flag = 'tr';
        if(lang === 'en') { text = 'English'; flag = 'gb'; }
        if(lang === 'es') { text = 'Español'; flag = 'es'; }
        
        document.getElementById('selectedText').textContent = text;
        document.getElementById('selectedFlag').src = "https://flagcdn.com/w40/" + flag + ".png";
    }

    async function changeLanguage(lang, text, flag) {
        setDropdownUI(lang);
        document.getElementById('langDropdown').classList.remove('active');
        document.getElementById('dropdownOptions').classList.remove('open');
        
        await fetch('/api/language', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ language: lang })
        });
        location.reload();
    }

    async function fetchFileLogs() {
        try {
            const res = await fetch('/api/file-logs');
            const text = await res.text();
            serverLogsBox.textContent = text;
            serverLogsBox.scrollTop = serverLogsBox.scrollHeight;
        } catch(e) {}
    }

    let lastLogIndex = 0;
    async function fetchLogs() {
        try {
            const res = await fetch('/api/logs?last=' + lastLogIndex);
            const data = await res.json();
            if (data.logs && data.logs.length > 0) {
                data.logs.forEach(line => term.write(line + '\r\n'));
                lastLogIndex = data.newIndex;
            }
        } catch(e) {}
    }

    checkStatus();
    checkAutoStart();
    loadSettings();
    loadTheme();
    loadOmniHealth();
    setInterval(checkStatus, 3000);
    setInterval(fetchLogs, 1000);
    setInterval(loadOmniHealth, 8000);
    if (document.getElementById('logs-tab').classList.contains('active')) {
        setInterval(fetchFileLogs, 3000);
    }
    
    term.write('\x1b[38;2;181;204;140m{{.T.TerminalReady}}\x1b[0m\r\n\r\n');
</script>
</body>
</html>
`

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

	http.Handle("/themes/", http.StripPrefix("/themes/", http.FileServer(http.Dir(getThemesDir()))))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		cfg := loadConfig()
		t := loadTranslations(cfg.Language)
		data := map[string]interface{}{
			"Lang":  cfg.Language,
			"Theme": cfg.Theme,
			"T":     t,
		}
		tmpl := template.Must(template.New("index").Parse(htmlTemplate))
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
		for {
			time.Sleep(500 * time.Millisecond)
			
			cmdMutex.Lock()
			isRunning := (cmd != nil && cmd.Process != nil)
			cmdMutex.Unlock()

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
