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
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err == nil {
		res := strings.TrimSpace(string(output))
		return res == "Yes" || res == "6"
	}
	return true
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// runNpmHidden runs npm via PowerShell with WindowStyle Hidden to guarantee no console window
func runNpmHidden(args ...string) (string, error) {
	npmCmd := "npm " + strings.Join(args, " ")
	psCmd := fmt.Sprintf("Start-Process -FilePath 'cmd.exe' -ArgumentList '/c','%s' -WindowStyle Hidden -Wait -PassThru | Out-Null", npmCmd)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func configureStartCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000 | 0x00000200,
	}
}
