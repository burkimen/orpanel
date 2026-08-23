//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func spawnDetached() {
	exe, _ := os.Executable()
	if exe == "" {
		exe = filepath.Base(os.Args[0])
	}

	cmd := exec.Command(exe, "--tray")
	cmd.SysProcAttr = nil
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
}

// hideConsole no-op on unix (no console window concept)
func hideConsole() {}
