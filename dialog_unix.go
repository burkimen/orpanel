//go:build !windows

package main

import (
	"os/exec"
	"strings"
)

func showConfirmDialog(title, message string) bool {
	// Try zenity (Linux)
	if _, err := exec.LookPath("zenity"); err == nil {
		cmd := exec.Command("zenity", "--question", "--title="+title, "--text="+message, "--width=350")
		if err := cmd.Run(); err == nil {
			return true
		}
		return false
	}
	// Try kdialog (KDE)
	if _, err := exec.LookPath("kdialog"); err == nil {
		cmd := exec.Command("kdialog", "--yesno", message, "--title", title)
		if err := cmd.Run(); err == nil {
			return true
		}
		return false
	}
	// Try osascript (macOS)
	if _, err := exec.LookPath("osascript"); err == nil {
		script := `display dialog "` + strings.ReplaceAll(message, `"`, `\"`) + `" with title "` + strings.ReplaceAll(title, `"`, `\"`) + `" buttons {"No", "Yes"} default button "Yes"`
		cmd := exec.Command("osascript", "-e", script)
		out, err := cmd.Output()
		if err == nil && strings.Contains(string(out), "Yes") {
			return true
		}
		return false
	}
	// Fallback: no GUI, assume yes and log
	return true
}

func hideWindow(cmd *exec.Cmd) {
	// no-op on unix
}

func configureStartCmd(cmd *exec.Cmd) {
	// no-op on unix
}
