//go:build !windows

package main

func tuiDiagOn() bool                   { return false }
func tuiDiagLog(string, ...interface{}) {}
func tuiDiagConsoleState(string)        {}
func tuiDiagDumpStacks(string)          {}
func tuiDiagReadCells(int, int, int) string {
	return "<unix>"
}
