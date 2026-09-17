//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func spawnDetached() {
	exe, _ := os.Executable()
	if exe == "" {
		exe = filepath.Base(os.Args[0])
	}

	cmd := exec.Command(exe, "--tray")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
		HideWindow:    true,
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		panic(err)
	}
}

// hideConsole hides the console window when launched via double-click
// (no interactive terminal). Uses Windows API GetConsoleWindow + ShowWindow.
func hideConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, 0) // SW_HIDE
	}
}

// detachFromConsole hides any attached console window, then detaches the
// process from it (FreeConsole) and ignores console control events, so a
// stray black window can neither show nor take the tray process down when
// its close box is used. Tray mode only; never call on interactive paths.
func detachFromConsole() {
	hideConsole()
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	freeConsole := kernel32.NewProc("FreeConsole")
	setCtrlHandler := kernel32.NewProc("SetConsoleCtrlHandler")
	freeConsole.Call()
	setCtrlHandler.Call(0, 1) // NULL handler, add=TRUE: ignore CTRL events
}

func relaunchAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200,
		HideWindow:    true,
	}
}
