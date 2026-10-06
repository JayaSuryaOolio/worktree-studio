package ui

import (
	"runtime"
	"strings"

	"worktree-studio/tui/domain"
)

// action is something the app does in response to a key it owns.
type action int

const (
	actMove action = iota + 1 // direction is the key's arrow
	actTab                    // tab number is the key's last digit
	actLeader
	actSidebar
	actNextTab
	actPalette
	actCancel
	actNewClaude
	actNewShell
	actNewWorktree
	actSplit // direction is the key's arrow
	actClosePane
	actCloseTab
	actZoom
	actPick
	actResize // direction is the key's arrow; shift+ without alt is a big step
	actResizeMode
	actDone
	actQuit
	actUp
	actDown
	actOpen
	actExpand
	actCollapse
	actClearFilter
	actBackspace
)

// binding is one key (or key family) the app owns. Each table below both
// dispatches keys and renders the on-screen hints, so the two can't drift.
type binding struct {
	keys  []string // tea.KeyMsg.String() values
	label string   // how the key is shown
	help  string
	act   action
}

// mod is what the alt key is called on this keyboard: Option on a Mac.
var mod = map[bool]string{true: "option", false: "alt"}[runtime.GOOS == "darwin"]

// optionGlyphs are what option+1…9 type on a US Mac layout when the
// terminal doesn't send Option as Meta (the default in Terminal.app, iTerm2
// and Warp); onKey reads them as alt+1…9.
var optionGlyphs = map[string]string{"¡": "1", "™": "2", "£": "3", "¢": "4", "∞": "5", "§": "6", "¶": "7", "•": "8", "ª": "9"}

func digits(prefix string) []string {
	var ks []string
	for d := '1'; d <= '9'; d++ {
		ks = append(ks, prefix+string(d))
	}
	return ks
}

func altDigits() []string {
	var ks []string
	for d := '1'; d <= '9'; d++ {
		ks = append(ks, "alt+"+string(d))
	}
	return ks
}

// globalKeys work in every mode, including while typing into Claude: only
// ctrl+space and alt combos, which shells and Claude leave alone. ctrl+space
// works in every terminal; alt arrows need Option sent as Meta on a Mac.
var globalKeys = []binding{
	{[]string{"ctrl+@"}, "ctrl+space", "menu", actLeader}, // ctrl+space sends NUL
	{[]string{"alt+left", "alt+right", "alt+up", "alt+down"}, mod + "+arrows", "move", actMove},
	{[]string{"alt+shift+left", "alt+shift+right", "alt+shift+up", "alt+shift+down"}, mod + "+shift+arrows", "resize", actResize},
	{altDigits(), mod + "+1-9", "tab", actTab},
}

// leaderKeys follow ctrl+space; the menu lists them, so they're never memorised.
var leaderKeys = []binding{
	{[]string{" "}, "space", "command palette", actPalette},
	{[]string{"left", "right", "up", "down"}, "←→↑↓", "move between panes", actMove},
	{[]string{"s"}, "s", "sidebar ↔ panes", actSidebar},
	{digits(""), "1-9", "go to tab", actTab},
	{[]string{"tab"}, "tab", "next tab", actNextTab},
	{[]string{"c"}, "c", "new Claude tab", actNewClaude},
	{[]string{"t"}, "t", "new shell tab", actNewShell},
	{[]string{"|", "-"}, "| -", "split right / down", actSplit},
	{[]string{"x"}, "x", "close pane (session keeps running)", actClosePane},
	{[]string{"w"}, "w", "close tab (sessions keep running)", actCloseTab},
	{[]string{"z"}, "z", "zoom pane (toggle)", actZoom},
	{[]string{"r"}, "r", "resize mode", actResizeMode},
	{[]string{"n"}, "n", "new worktree", actNewWorktree},
	{[]string{"q"}, "q", "quit (sessions keep running)", actQuit},
	{[]string{"esc", "ctrl+@"}, "esc", "close menu", actCancel},
}

// sidebarKeys: any other printable key types into the filter (Root's
// fallthrough), so the hint for it lives here as a label-only row.
var sidebarKeys = []binding{
	{nil, "type", "filter", 0},
	{[]string{"up"}, "↑↓", "move", actUp},
	{[]string{"down"}, "", "", actDown},
	{[]string{"right"}, "→←", "open/close", actExpand},
	{[]string{"left"}, "", "", actCollapse},
	{[]string{"enter"}, "enter", "open", actOpen},
	{[]string{"esc"}, "", "", actClearFilter},
	{[]string{"backspace"}, "", "", actBackspace},
	{[]string{"ctrl+c"}, "", "", actQuit},
}

// resizeKeys are the whole keymap while resize mode is on.
var resizeKeys = []binding{
	{[]string{"left", "right", "up", "down"}, "arrows", "resize", actResize},
	{[]string{"shift+left", "shift+right", "shift+up", "shift+down"}, "shift", "bigger", actResize},
	{[]string{"esc", "enter"}, "esc", "done", actDone},
}

// pickKeys drive the "what runs here?" list in an empty pane.
var pickKeys = []binding{
	{[]string{"up"}, "↑↓", "choose", actUp},
	{[]string{"down"}, "", "", actDown},
	{[]string{"enter"}, "enter", "open", actPick},
	{[]string{"esc"}, "esc", "close pane", actClosePane},
}

func lookup(table []binding, key string) (action, bool) {
	for _, b := range table {
		for _, k := range b.keys {
			if k == key {
				return b.act, true
			}
		}
	}
	return 0, false
}

// hints renders a table as "key help  key help"; bindings without a label
// are shown by a sibling (↑↓ covers down).
// arrow maps an arrow key ("alt+left", "right", …) to a direction.
func arrow(key string) domain.Dir {
	switch {
	case strings.HasSuffix(key, "left"):
		return domain.Left
	case strings.HasSuffix(key, "up"):
		return domain.Up
	case strings.HasSuffix(key, "down"):
		return domain.Down
	}
	return domain.Right
}

func hints(table []binding) string {
	var parts []string
	for _, b := range table {
		if b.label != "" {
			parts = append(parts, textStyle.Render(b.label)+" "+dimStyle.Render(b.help))
		}
	}
	return strings.Join(parts, "  ")
}
