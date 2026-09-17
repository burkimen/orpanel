//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
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
// detachFromConsole hides the console window only when this process owns it
// alone (GetConsoleProcessList count == 1: batch relaunch, double-click, Run
// key). From a shared shell console the hide is skipped so the user's
// terminal is never hidden; FreeConsole + ignored CTRL events still detach
// the tray process so a terminal close cannot take it down. Tray mode only.
func detachFromConsole() {
	hideOwnConsole()
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	freeConsole := kernel32.NewProc("FreeConsole")
	setCtrlHandler := kernel32.NewProc("SetConsoleCtrlHandler")
	freeConsole.Call()
	setCtrlHandler.Call(0, 1) // NULL handler, add=TRUE: ignore CTRL events
}

// hideOwnConsole hides the console window only if no other process shares it.
func hideOwnConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	getProcessList := kernel32.NewProc("GetConsoleProcessList")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")
	var pids [16]uint32
	n, _, _ := getProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n != 1 {
		return
	}
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, 0) // SW_HIDE
	}
}

func relaunchAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200,
		HideWindow:    true,
	}
}
