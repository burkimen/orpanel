//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func setAutoStart(enable bool) error {
	cfg := loadConfig()
	saveConfig(cfg.Language, enable)

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()

	if runtime.GOOS == "darwin" {
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.burkimen.orpanel.plist")
		if !enable {
			_ = os.Remove(plistPath)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
			return err
		}
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.burkimen.orpanel</string>
<key>ProgramArguments</key><array><string>%s</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><false/>
</dict></plist>
`, exePath)
		return os.WriteFile(plistPath, []byte(plist), 0644)
	}
	// linux: .desktop
	desktopPath := filepath.Join(home, ".config", "autostart", "orpanel.desktop")
	if !enable {
		_ = os.Remove(desktopPath)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(desktopPath), 0755); err != nil {
		return err
	}
	desktop := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=Orpanel\nExec=%s\nHidden=false\nNoDisplay=false\nX-GNOME-Autostart-enabled=true\n", exePath)
	return os.WriteFile(desktopPath, []byte(desktop), 0644)
}

func isAutoStartEnabled() bool {
	home, _ := os.UserHomeDir()
	var exists bool
	if runtime.GOOS == "darwin" {
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.burkimen.orpanel.plist")
		_, err := os.Stat(plistPath)
		exists = err == nil
	} else {
		desktopPath := filepath.Join(home, ".config", "autostart", "orpanel.desktop")
		_, err := os.Stat(desktopPath)
		exists = err == nil
	}
	return exists
}
