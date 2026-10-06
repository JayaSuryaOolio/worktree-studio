package ui

import (
	"strings"

	"worktree-studio/tui/domain"
)

// action is something the app does in response to a key it owns.
type action int

const (
	actMove action = iota + 1 // direction is the key's arrow
	actTab // tab number is the key's last digit
	actLeader
	actCancel
	actNewClaude
	actNewShell
	actNewWorktree
	actSplit // direction is the key's arrow
	actClosePane
	actZoom
	actPick
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

func altDigits() []string {
	var ks []string
	for d := '1'; d <= '9'; d++ {
		ks = append(ks, "alt+"+string(d))
	}
	return ks
}

// globalKeys work in every mode, including while typing into Claude: only
// alt combos and ctrl+space, which shells and Claude leave alone.
var globalKeys = []binding{
	{[]string{"alt+left", "alt+right", "alt+up", "alt+down"}, "alt+arrows", "move", actMove},
	{altDigits(), "alt+1-9", "tab", actTab},
	{[]string{"ctrl+@"}, "ctrl+space", "menu", actLeader}, // ctrl+space sends NUL
}

// leaderKeys follow ctrl+space; the menu lists them, so they're never memorised.
var leaderKeys = []binding{
	{[]string{"c"}, "c", "new Claude tab", actNewClaude},
	{[]string{"t"}, "t", "new shell tab", actNewShell},
	{[]string{"right", "down"}, "→ ↓", "split right / down", actSplit},
	{[]string{"x"}, "x", "close pane (session keeps running)", actClosePane},
	{[]string{"z"}, "z", "zoom pane (toggle)", actZoom},
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
