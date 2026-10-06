package ui

import "strings"

// action is something the app does in response to a key it owns.
type action int

const (
	actFocusSidebar action = iota + 1
	actFocusPanes
	actTab // tab number is the key's last digit
	actLeader
	actCancel
	actNewClaude
	actNewShell
	actNewWorktree
	actQuit
	actUp
	actDown
	actOpen
	actPrevRepo
	actNextRepo
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
	{[]string{"alt+left"}, "alt+←", "sidebar", actFocusSidebar},
	{[]string{"alt+right"}, "alt+→", "terminal", actFocusPanes},
	{altDigits(), "alt+1-9", "tab", actTab},
	{[]string{"ctrl+@"}, "ctrl+space", "menu", actLeader}, // ctrl+space sends NUL
}

// leaderKeys follow ctrl+space; the menu lists them, so they're never memorised.
var leaderKeys = []binding{
	{[]string{"c"}, "c", "new Claude tab", actNewClaude},
	{[]string{"t"}, "t", "new shell tab", actNewShell},
	{[]string{"n"}, "n", "new worktree", actNewWorktree},
	{[]string{"q"}, "q", "quit (sessions keep running)", actQuit},
	{[]string{"esc", "ctrl+@"}, "esc", "close menu", actCancel},
}

var sidebarKeys = []binding{
	{[]string{"up"}, "↑↓", "move", actUp},
	{[]string{"down"}, "", "", actDown},
	{[]string{"enter"}, "enter", "open", actOpen},
	{[]string{"left"}, "←→", "repo", actPrevRepo},
	{[]string{"right"}, "", "", actNextRepo},
	{[]string{"ctrl+c"}, "", "", actQuit},
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
func hints(table []binding) string {
	var parts []string
	for _, b := range table {
		if b.label != "" {
			parts = append(parts, textStyle.Render(b.label)+" "+dimStyle.Render(b.help))
		}
	}
	return strings.Join(parts, "  ")
}
