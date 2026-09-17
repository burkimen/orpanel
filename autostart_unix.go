//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// setAutoStart persists the toggle and writes/removes the plist/desktop entry.
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
		return os.WriteFile(plistPath, []byte(plistContent(exePath)), 0644)
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
	return os.WriteFile(desktopPath, []byte(desktopContent(exePath)), 0644)
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
