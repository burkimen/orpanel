package main

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// tui_keymap.go: the single keymap table — the only source of truth for
// bindings. Bar, footer, and help are all generated from it, so a key shown
// in the UI is always actually bound.
//
// Two sections: actions (the action bar, left to right) and navigation
// (global + pane-local keys shown in the footer and the help modal).
// scope is one of "action", "global", or "pane" (pane-local, see panes).

// Scopes for keymap entries.
const (
	tuiScopeAction = "action"
	tuiScopeGlobal = "global"
	tuiScopePane   = "pane"
)

// tuiBinding declares one binding: its id, primary key, label key, section
// scope, the panes it applies to (empty = all), whether it needs a confirm
// modal, and a short hint label key for the footer.
type tuiBinding struct {
	act     int
	key     rune
	keyName string // display name for non-rune keys ("Tab", "Enter", ...)
	label   string
	hint    string // footer hint label key
	scope   string
	panes   []int // pane-local only; empty = all panes
	confirm bool
}

// tuiNavActs are pseudo-action ids for navigation entries (never dispatched).
const (
	tuiNavSelect = 100 + iota
	tuiNavPane
	tuiNavActivate
	tuiNavCancel
	tuiNavScroll
	tuiNavEdges
	tuiNavHelp
	tuiNavQuit
)

// tuiKeymap is ordered for the action bar left to right, then navigation.
var tuiKeymap = []tuiBinding{
	{act: tuiActStart, key: 's', label: "TuiStart", scope: tuiScopeAction, confirm: false},
	{act: tuiActStop, key: 'x', label: "TuiStop", scope: tuiScopeAction, confirm: true},
	{act: tuiActRestart, key: 'r', label: "TuiRestart", scope: tuiScopeAction, confirm: true},
	{act: tuiActUpdate, key: 'u', label: "TuiUpdate", scope: tuiScopeAction, confirm: true},
	{act: tuiActRepair, key: 'R', label: "TuiRepair", scope: tuiScopeAction, confirm: true},
	{act: tuiActInstall, key: 'i', label: "TuiInstall", scope: tuiScopeAction, confirm: false},
	{act: tuiActAutostart, key: 'a', label: "TuiAutostart", scope: tuiScopeAction, confirm: false},
	{act: tuiActLanguage, key: 'l', label: "TuiLanguage", scope: tuiScopeAction, confirm: false},
	{act: tuiActTheme, key: 't', label: "TuiTheme", scope: tuiScopeAction, confirm: false},
	{act: tuiActWebUI, key: 'w', label: "TuiWebUI", scope: tuiScopeAction, confirm: false},
	{act: tuiNavSelect, key: 'j', keyName: "↑↓/j/k", label: "TuiNavSelect", hint: "TuiHintSelect", scope: tuiScopePane, panes: []int{tuiPaneActions}},
	{act: tuiNavPane, key: '\t', keyName: "Tab", label: "TuiNavPane", hint: "TuiHintPane", scope: tuiScopeGlobal},
	{act: tuiNavActivate, key: '\r', keyName: "Enter", label: "TuiNavActivate", hint: "TuiHintActivate", scope: tuiScopePane, panes: []int{tuiPaneActions}},
	{act: tuiNavCancel, key: 0x1b, keyName: "Esc", label: "TuiNavCancel", hint: "TuiHintCancel", scope: tuiScopeGlobal},
	{act: tuiNavScroll, key: 'k', keyName: "PgUp/PgDn", label: "TuiNavScroll", hint: "TuiHintScroll", scope: tuiScopePane, panes: []int{tuiPaneLogs}},
	{act: tuiNavEdges, key: 'g', keyName: "g/G", label: "TuiNavEdges", hint: "TuiHintEdges", scope: tuiScopePane, panes: []int{tuiPaneLogs}},
	{act: tuiNavHelp, key: '?', label: "TuiHelp", hint: "TuiHintHelp", scope: tuiScopeGlobal},
	{act: tuiNavQuit, key: 'q', label: "TuiClose", hint: "TuiHintQuit", scope: tuiScopeGlobal},
}

// tuiActionBindings returns only the action-bar entries, in bar order.
func tuiActionBindings() []tuiBinding {
	var out []tuiBinding
	for _, b := range tuiKeymap {
		if b.scope == tuiScopeAction {
			out = append(out, b)
		}
	}
	return out
}

// tuiFooterBindings returns the footer hints for the focused pane: global
// entries plus pane-local entries for that pane, in keymap order.
func tuiFooterBindings(pane int) []tuiBinding {
	var out []tuiBinding
	for _, b := range tuiKeymap {
		switch b.scope {
		case tuiScopeGlobal:
			out = append(out, b)
		case tuiScopePane:
			for _, p := range b.panes {
				if p == pane {
					out = append(out, b)
					break
				}
			}
		}
	}
	return out
}

// tuiBindingByKey finds an ACTION binding by rune, case-insensitive.
// 'R' (repair) wins over 'r' (restart) only on exact uppercase match.
// Navigation entries are never returned here (they are not dispatched).
func tuiBindingByKey(r rune) (tuiBinding, bool) {
	for _, b := range tuiKeymap {
		if b.scope != tuiScopeAction {
			continue
		}
		if b.key == r {
			return b, true
		}
	}
	// Case-insensitive fallback for single-case bindings.
	lower := r
	if lower >= 'A' && lower <= 'Z' {
		lower = lower - 'A' + 'a'
	}
	for _, b := range tuiKeymap {
		if b.scope != tuiScopeAction {
			continue
		}
		if b.key == 'R' {
			continue // R is uppercase-only (repair vs restart).
		}
		bl := b.key
		if bl >= 'A' && bl <= 'Z' {
			bl = bl - 'A' + 'a'
		}
		if bl == lower {
			return b, true
		}
	}
	return tuiBinding{}, false
}

func tuiBindingByAct(act int) (tuiBinding, bool) {
	for _, b := range tuiKeymap {
		if b.act == act {
			return b, true
		}
	}
	return tuiBinding{}, false
}

// tuiKeyName returns the display name for a binding's key.
func tuiKeyName(b tuiBinding) string {
	if b.keyName != "" {
		return b.keyName
	}
	return string(b.key)
}

// tuiConfirmNeeded reports whether an action needs an in-TUI confirm step.
func tuiConfirmNeeded(act int) bool {
	if b, ok := tuiBindingByAct(act); ok {
		return b.confirm
	}
	return false
}

// tuiEventKey converts a tcell key event to our tuiKey. tcell decodes all
// console input (Windows ConIn raw mode, arrows, PgUp/Dn, Home/End, mouse)
// so no raw-mode code of ours remains in the path.
func tuiEventKey(ev *tcell.EventKey) tuiKey {
	switch ev.Key() {
	case tcell.KeyEscape:
		return tuiKey{esc: true, raw: "\x1b"}
	case tcell.KeyEnter:
		return tuiKey{r: '\r'}
	case tcell.KeyTab:
		return tuiKey{r: '\t'}
	case tcell.KeyBacktab:
		return tuiKey{r: '\t', raw: "backtab"}
	case tcell.KeyUp:
		return tuiKey{esc: true, raw: "[A"}
	case tcell.KeyDown:
		return tuiKey{esc: true, raw: "[B"}
	case tcell.KeyRight:
		return tuiKey{esc: true, raw: "[C"}
	case tcell.KeyLeft:
		return tuiKey{esc: true, raw: "[D"}
	case tcell.KeyPgUp:
		return tuiKey{esc: true, raw: "[5~"}
	case tcell.KeyPgDn:
		return tuiKey{esc: true, raw: "[6~"}
	case tcell.KeyHome:
		return tuiKey{esc: true, raw: "[H"}
	case tcell.KeyEnd:
		return tuiKey{esc: true, raw: "[F"}
	case tcell.KeyCtrlC:
		return tuiKey{raw: "\x03"}
	case tcell.KeyRune:
		return tuiKey{r: ev.Rune()}
	}
	if r := ev.Rune(); r != 0 {
		return tuiKey{r: r}
	}
	return tuiKey{raw: "unknown:" + strings.TrimSpace(ev.Name())}
}
