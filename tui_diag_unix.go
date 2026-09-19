//go:build !windows

package main

import "github.com/gdamore/tcell/v2"

// tuiDiagScreen is the tcell.Screen subset the read-back needs.
type tuiDiagScreen interface {
	GetContent(x, y int) (rune, []rune, tcell.Style, int)
	Size() (int, int)
}

func tuiDiagOn() bool                   { return false }
func tuiDiagLog(string, ...interface{}) {}
func tuiDiagConsoleState(string)        {}
func tuiDiagDumpStacks(string)          {}
func tuiDiagReadCells(int, int, int) string {
	return "<unix>"
}

func tuiDiagScreenRow(_ tuiDiagScreen, _, _, _ int) string {
	return "<unix>"
}

func tuiDiagConsoleRow(_, _, _ int) string {
	return "<unix>"
}

func tuiDiagCellStyle(_ tuiDiagScreen, _, _ int) string {
	return "<unix>"
}

func tuiDiagThemeName() string {
	return "<unix>"
}
