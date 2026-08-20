package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/getlantern/systray"
	"golang.org/x/sys/windows/registry"
)

// --- EVRENSEL VE PLATFORMA DUYARLI YOL YAPILANDIRMASI ---
func getOmniroutePath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "npm", "node_modules", "omniroute")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".npm", "lib", "node_modules", "omniroute")
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
	file, err := os.ReadFile(ConfigFileName)
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
	os.WriteFile(ConfigFileName, data, 0644)
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
	return os.WriteFile(ConfigFileName, data, 0644)
}

func loadTranslations(lang string) map[string]string {
	filePath := filepath.Join(LocalesDir, lang+".json")
	file, err := os.ReadFile(filePath)
	if err != nil {
		file, err = os.ReadFile(filepath.Join(LocalesDir, "tr.json"))
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
                <div class="brand-icon"><span class="material-symbols-rounded">route</span></div>
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
    setInterval(checkStatus, 3000);
    setInterval(fetchLogs, 1000);
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
	fileLogWriter, err = os.OpenFile(LogFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Log dosyası açılamadı: %v\n", err)
	}
}

func writeLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamped := fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), msg)
	
	logMutex.Lock()
	logBuffer = append(logBuffer, timestamped)
	if len(logBuffer) > maxLogSize {
		logBuffer = logBuffer[1:]
	}
	logMutex.Unlock()

	if fileLogWriter != nil {
		fileLogWriter.WriteString(timestamped + "\n")
	}
}

func setAutoStart(enable bool) error {
	cfg := loadConfig()
	saveConfig(cfg.Language, enable)

	if runtime.GOOS == "windows" {
		k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.ALL_ACCESS)
		if err != nil {
			return err
		}
		defer k.Close()

		if enable {
			exePath, _ := os.Executable()
			return k.SetStringValue(AppName, `"`+exePath+`"`)
		}
		return k.DeleteValue(AppName)
	}
	return nil
}

func isAutoStartEnabled() bool {
	cfg := loadConfig()
	if runtime.GOOS == "windows" {
		k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
		if err != nil {
			return cfg.AutoStart
		}
		defer k.Close()
		_, _, err = k.GetStringValue(AppName)
		return err == nil
	}
	return cfg.AutoStart
}

func showConfirmDialog(title, message string) bool {
	if runtime.GOOS == "windows" {
		psCommand := fmt.Sprintf(
			"Add-Type -AssemblyName PresentationFramework; [System.Windows.MessageBox]::Show('%s', '%s', 'YesNo', 'Warning')",
			message, title,
		)
		cmd := exec.Command("powershell", "-NoProfile", "-Command", psCommand)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		output, err := cmd.Output()
		if err == nil {
			res := strings.TrimSpace(string(output))
			return res == "Yes" || res == "6"
		}
	}
	return true
}

func startOmniroute() {
	cmdMutex.Lock()
	defer cmdMutex.Unlock()

	if cmd != nil && cmd.Process != nil {
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

	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: 0x08000000 | 0x00000200,
		}
	}

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
	t := loadTranslations(currentLang)
	writeLog("%s", t["LogStopSignal"])

	if cmd != nil && cmd.Process != nil {
		if runtime.GOOS == "windows" {
			exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		} else {
			cmd.Process.Kill()
		}
		cmd = nil
		writeLog("%s", t["LogProcessesKilled"])
	}
}

func startWatchdog() {
	go func() {
		for {
			time.Sleep(5 * time.Second)
			cmdMutex.Lock()
			if cmd == nil && !isIntentionalStop {
				cmdMutex.Unlock()
				t := loadTranslations(currentLang)
				writeLog("%s", t["LogWatchdog"])
				startOmniroute()
			} else {
				cmdMutex.Unlock()
			}
		}
	}()
}

func openBrowser(url string) {
	if runtime.GOOS == "windows" {
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	} else if runtime.GOOS == "darwin" {
		exec.Command("open", url).Start()
	} else {
		exec.Command("xdg-open", url).Start()
	}
}

func startWebServer() {
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "app.ico")
	})

	http.Handle("/themes/", http.StripPrefix("/themes/", http.FileServer(http.Dir("themes"))))

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
		data, err := os.ReadFile(LogFileName)
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
	iconBytes, err := os.ReadFile("app.ico")
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
