package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeyBytes(t *testing.T) {
	cases := []struct {
		k    tea.KeyMsg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("é")}, "é"},
		{tea.KeyMsg{Type: tea.KeyEnter}, "\r"},
		{tea.KeyMsg{Type: tea.KeyCtrlC}, "\x03"},
		{tea.KeyMsg{Type: tea.KeyBackspace}, "\x7f"},
		{tea.KeyMsg{Type: tea.KeyUp}, "\x1b[A"},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b"), Alt: true}, "\x1bb"},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi"), Paste: true}, "\x1b[200~hi\x1b[201~"},
	}
	for _, c := range cases {
		if got := string(keyBytes(c.k)); got != c.want {
			t.Errorf("%v: got %q want %q", c.k, got, c.want)
		}
	}
}
