package ui

import tea "github.com/charmbracelet/bubbletea"

var csiKeys = map[tea.KeyType]string{
	tea.KeyUp: "\x1b[A", tea.KeyDown: "\x1b[B", tea.KeyRight: "\x1b[C", tea.KeyLeft: "\x1b[D",
	tea.KeyShiftUp: "\x1b[1;2A", tea.KeyShiftDown: "\x1b[1;2B", tea.KeyShiftRight: "\x1b[1;2C", tea.KeyShiftLeft: "\x1b[1;2D",
	tea.KeyCtrlUp: "\x1b[1;5A", tea.KeyCtrlDown: "\x1b[1;5B", tea.KeyCtrlRight: "\x1b[1;5C", tea.KeyCtrlLeft: "\x1b[1;5D",
	tea.KeyHome: "\x1b[H", tea.KeyEnd: "\x1b[F", tea.KeyPgUp: "\x1b[5~", tea.KeyPgDown: "\x1b[6~",
	tea.KeyDelete: "\x1b[3~", tea.KeyInsert: "\x1b[2~", tea.KeyShiftTab: "\x1b[Z",
	tea.KeyF1: "\x1bOP", tea.KeyF2: "\x1bOQ", tea.KeyF3: "\x1bOR", tea.KeyF4: "\x1bOS",
	tea.KeyF5: "\x1b[15~", tea.KeyF6: "\x1b[17~", tea.KeyF7: "\x1b[18~", tea.KeyF8: "\x1b[19~",
	tea.KeyF9: "\x1b[20~", tea.KeyF10: "\x1b[21~", tea.KeyF11: "\x1b[23~", tea.KeyF12: "\x1b[24~",
}

// keyBytes translates a Bubble Tea key into the bytes a terminal would send,
// or nil for keys with no encoding. Control keys share their byte value with
// their tea.KeyType (Enter=13, Tab=9, Esc=27, Backspace=127).
func keyBytes(k tea.KeyMsg) []byte {
	var out []byte
	switch {
	case k.Type == tea.KeyRunes:
		out = []byte(string(k.Runes))
		if k.Paste {
			out = append(append([]byte("\x1b[200~"), out...), "\x1b[201~"...)
			return out
		}
	case k.Type == tea.KeySpace:
		out = []byte(" ")
	case (k.Type >= 0 && k.Type <= 31) || k.Type == tea.KeyBackspace:
		out = []byte{byte(k.Type)}
	default:
		s, ok := csiKeys[k.Type]
		if !ok {
			return nil
		}
		out = []byte(s)
	}
	if k.Alt {
		out = append([]byte{0x1b}, out...)
	}
	return out
}
