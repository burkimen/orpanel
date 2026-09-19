//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"syscall"
	"time"
	"unsafe"

	"github.com/gdamore/tcell/v2"
	"golang.org/x/sys/windows"
)

// tuiDiagScreen is the tcell.Screen subset the read-back needs.
type tuiDiagScreen interface {
	GetContent(x, y int) (rune, []rune, tcell.Style, int)
	Size() (int, int)
}

var modKernel32 = syscall.NewLazyDLL("kernel32.dll")
var procReadConsoleOutputCharacter = modKernel32.NewProc("ReadConsoleOutputCharacterW")

// tuiDiagReadCells reads n chars at (x,y) from the active output buffer.
func tuiDiagReadCells(x, y, n int) string {
	if !tuiDiagOn() {
		return "<diag-off>"
	}
	hOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return fmt.Sprintf("<handle-err %v>", err)
	}
	buf := make([]uint16, n)
	var read uint32
	coord := uintptr(uint32(y)<<16 | uint32(x)&0xffff)
	r, _, e := procReadConsoleOutputCharacter.Call(
		uintptr(hOut),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(n),
		coord,
		uintptr(unsafe.Pointer(&read)),
	)
	if r == 0 {
		return fmt.Sprintf("<read-err %v>", e)
	}
	return fmt.Sprintf("%q(read=%d)", syscall.UTF16ToString(buf[:read]), read)
}

// tuiDiagScreenRow reads n cells of row y from the tcell screen tview just
// painted (the after-draw callback parameter). Same object, same draw call:
// no console round-trip, no stale buffer. Env-gated, inert when unset.
func tuiDiagScreenRow(screen tuiDiagScreen, x, y, n int) string {
	if !tuiDiagOn() {
		return "<diag-off>"
	}
	runes := make([]rune, 0, n)
	for i := range n {
		main, comb, _, _ := screen.GetContent(x+i, y)
		if main == 0 && len(comb) == 0 {
			runes = append(runes, ' ')
			continue
		}
		if main == 0 && len(comb) > 0 {
			runes = append(runes, comb[0])
			continue
		}
		runes = append(runes, main)
	}
	return fmt.Sprintf("%q", string(runes))
}
// tuiDiag is env-gated instrumentation (ORPANEL_TUI_DIAG=1) writing stage
// lines to %TEMP%/orpanel-tui-diag.log. Opt-in only; normal path untouched.
func tuiDiagOn() bool { return os.Getenv("ORPANEL_TUI_DIAG") == "1" }

func tuiDiagLog(format string, args ...interface{}) {
	if !tuiDiagOn() {
		return
	}
	p := os.ExpandEnv(`$TEMP/orpanel-tui-diag.log`)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}

func tuiDiagConsoleState(stage string) {
	if !tuiDiagOn() {
		return
	}
	hOut, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	var mode uint32
	modeErr := windows.GetConsoleMode(hOut, &mode)
	cp, cpErr := windows.GetConsoleOutputCP()
	var info windows.ConsoleScreenBufferInfo
	infoErr := windows.GetConsoleScreenBufferInfo(hOut, &info)
	tuiDiagLog("%s: mode=0x%x(modeErr=%v) cp=%d(cpErr=%v) buf=%dx%d cursor=%d,%d win=(%d,%d)-(%d,%d) infoErr=%v hOut=%d",
		stage, mode, modeErr, cp, cpErr,
		info.Size.X, info.Size.Y, info.CursorPosition.X, info.CursorPosition.Y,
		info.Window.Left, info.Window.Top, info.Window.Right, info.Window.Bottom, infoErr, hOut)
}

func tuiDiagDumpStacks(reason string) {
	if !tuiDiagOn() {
		return
	}
	p := os.ExpandEnv(`$TEMP/orpanel-tui-diag.log`)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s STACKDUMP(%s):\n", time.Now().Format("15:04:05.000"), reason)
	_ = pprof.Lookup("goroutine").WriteTo(f, 1)
}
