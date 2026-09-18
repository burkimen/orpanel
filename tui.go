package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"
)

// tui.go: terminal setup, main loop, actions, plain fallback.
// Rendering primitives and key handling live in tui_keys.go.

// tuiTerminalSize returns the real console size, degraded safely.
func tuiTerminalSize() (int, int) {
	if w, h, err := tuiConsoleSize(); err == nil && w >= 40 && h >= 10 {
		return w, h
	}
	return 80, 24
}

func takeTuiSnapshot() tuiSnapshot {
	cfg := loadConfig()
	h := checkOmniHealth()
	logMutex.Lock()
	logs := append([]string(nil), logBuffer...)
	logMutex.Unlock()
	if len(logs) > 200 {
		logs = logs[len(logs)-200:]
	}
	updateMu.Lock()
	opPhase := ""
	if updatePhaseNow != updateIdle && updatePhaseNow != updateFailed {
		opPhase = string(updatePhaseNow)
	}
	updateMu.Unlock()
	if isOmniOpRunning() && opPhase == "" {
		opPhase = "op"
	}
	return tuiSnapshot{
		appVer:   AppVersion,
		omniVer:  h.Version,
		lang:     cfg.Language,
		theme:    cfg.Theme,
		status:   h.Status,
		probe:    h.ProbeStatus,
		port:     fmt.Sprintf("%d", OmniPort),
		nodeVer:  h.NodeVersion,
		external: h.ExternallyManaged,
		opPhase:  opPhase,
		logs:     logs,
	}
}

// runTUI runs the full-screen interface. It restores the terminal on every
// exit path. If stdout is not a terminal it prints the plain summary instead.
func runTUI() {
	if !isTerminalOut() {
		printPlainSummary()
		return
	}
	t := loadTranslations(getCurrentLang())
	restore, err := tuiEnter()
	if err != nil {
		printPlainSummary()
		return
	}
	var restored bool
	restoreOnce := func() {
		if !restored {
			restored = true
			restore()
		}
	}
	defer restoreOnce()
	defer func() {
		if r := recover(); r != nil {
			restoreOnce()
			panic(r)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() {
		<-sig
		restoreOnce()
		os.Exit(130)
	}()
	st := tuiState{pane: tuiPaneActions, rows: tuiActionRows(t)}
	msg := ""
	keys := tuiKeyReader()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		snap := takeTuiSnapshot()
		snap.msg = msg
		w, h := tuiTerminalSize()
		fmt.Print("\x1b[H" + composeFrame(st, snap, t, w, h, time.Now()))
		select {
		case k, ok := <-keys:
			if !ok {
				return
			}
			st = handleKey(st, k)
			if st.quit {
				return
			}
			msg = tuiDoAction(st.lastAct, t)
			st.rows = tuiActionRows(loadTranslations(getCurrentLang()))
		case <-tick.C:
		}
	}
}

// tuiKeyReader decodes stdin bytes into tuiKey values. Arrows arrive as
// ESC [ A / ESC [ B. The goroutine exits when stdin closes.
func tuiKeyReader() <-chan tuiKey {
	ch := make(chan tuiKey, 16)
	go func() {
		defer close(ch)
		var one [1]byte
		for {
			n, err := os.Stdin.Read(one[:])
			if err != nil || n == 0 {
				return
			}
			c := one[0]
			if c == 0x1b {
				var seq [2]byte
				m, err := os.Stdin.Read(seq[:1])
				if err != nil || m == 0 || seq[0] != '[' {
					continue
				}
				m, err = os.Stdin.Read(seq[1:2])
				if err != nil || m == 0 {
					continue
				}
				ch <- tuiKey{esc: true, raw: "[" + string(seq[1])}
				continue
			}
			switch c {
			case '\t':
				ch <- tuiKey{r: '\t'}
			case '\r', '\n':
				ch <- tuiKey{r: '\r'}
			case 0x03:
				ch <- tuiKey{raw: "\x03"}
			default:
				ch <- tuiKey{r: rune(c)}
			}
		}
	}()
	return ch
}

// tuiDoAction executes one action id. Long ops run guarded by existing mutexes.
func tuiDoAction(act int, t map[string]string) string {
	tr := func(k, fb string) string {
		if v, ok := t[k]; ok && v != "" {
			return v
		}
		return fb
	}
	switch act {
	case tuiActStart:
		if isOmniOpRunning() {
			return tr("TuiBusyOp", "busy")
		}
		go startOmniroute()
		return tr("TuiOpStarted", "Started")
	case tuiActStop:
		go stopOmniroute()
		return tr("TuiOpStarted", "Started")
	case tuiActRestart:
		if isOmniOpRunning() {
			return tr("TuiBusyOp", "busy")
		}
		go func() { stopOmniroute(); startOmniroute() }()
		return tr("TuiOpStarted", "Started")
	case tuiActUpdate:
		if isOmniOpRunning() {
			return tr("TuiBusyOp", "busy")
		}
		go runNpmOmni("update", "update", "-g", "omniroute@latest")
		return tr("TuiOpStarted", "Started")
	case tuiActRepair:
		if isOmniOpRunning() {
			return tr("TuiBusyOp", "busy")
		}
		go runNpmOmni("repair", "install", "-g", "omniroute@latest", "--force")
		return tr("TuiOpStarted", "Started")
	case tuiActInstall:
		if isOmniOpRunning() {
			return tr("TuiBusyOp", "busy")
		}
		go runNpmOmni("install", "install", "-g", "omniroute@latest")
		return tr("TuiOpStarted", "Started")
	case tuiActAutostart:
		_ = setAutoStart(!isAutoStartEnabled())
		return tr("TuiAutostart", "Autostart")
	case tuiActLanguage:
		cfg := loadConfig()
		next := "en"
		switch cfg.Language {
		case "en":
			next = "tr"
		case "tr":
			next = "es"
		default:
			next = "en"
		}
		saveConfig(next, cfg.AutoStart)
		return next
	case tuiActTheme:
		cfg := loadConfig()
		next := ThemeLight
		switch cfg.Theme {
		case ThemeLight:
			next = ThemeDark
		case ThemeDark:
			next = ThemeSystem
		default:
			next = ThemeLight
		}
		_ = saveTheme(next)
		return next
	case tuiActWebUI:
		openBrowser("http://localhost:20127")
		return tr("TuiOpenedBrowser", "Opened")
	default:
		return ""
	}
}

// plainSummaryText builds the non-TTY fallback: stable order, no ESC bytes.
func plainSummaryText() string {
	cfg := loadConfig()
	h := checkOmniHealth()
	lines := []string{
		fmt.Sprintf("orpanel v%s", AppVersion),
		fmt.Sprintf("language=%s theme=%s autostart=%v", cfg.Language, cfg.Theme, isAutoStartEnabled()),
		fmt.Sprintf("omniroute status=%s probe=%s port=%d node=%s version=%s", h.Status, h.ProbeStatus, OmniPort, h.NodeVersion, h.Version),
		fmt.Sprintf("update=%s", getUpdateStatus().Phase),
	}
	return strings.Join(lines, "\n")
}

// printPlainSummary is the non-TTY fallback entry point.
func printPlainSummary() {
	fmt.Println(plainSummaryText())
}
