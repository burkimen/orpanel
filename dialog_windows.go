//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func showConfirmDialog(title, message string) bool {
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
	return true
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func configureStartCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000 | 0x00000200,
	}
}
