package ptyd

import (
	"bytes"
	"unicode/utf8"
)

// MaxScrollback is how many bytes of output each session keeps for replay
// on attach.
const MaxScrollback = 4 << 20

// scrollback keeps the last max bytes written to it.
// ponytail: append-and-trim instead of a true ring; copies max bytes roughly
// every max bytes written (amortised O(1)/byte, up to ~2x max resident).
type scrollback struct {
	max     int
	buf     []byte
	trimmed bool // older output was dropped, so buf[0] is an arbitrary byte
}

func (s *scrollback) write(p []byte) {
	s.buf = append(s.buf, p...)
	if len(s.buf) > s.max {
		s.buf = s.buf[len(s.buf)-s.max:]
		s.trimmed = true
	}
}

// replay returns a copy of the buffer to send to a newly attached client,
// starting at a clean boundary if older output was dropped.
func (s *scrollback) replay() []byte {
	b := s.buf
	if s.trimmed {
		b = b[cleanStart(b):]
	}
	return append([]byte(nil), b...)
}

// cleanBoundaryWindow bounds how far cleanStart looks for a boundary.
const cleanBoundaryWindow = 64 << 10

// cleanStart returns where a replay of a trimmed buffer should start so it
// doesn't begin mid escape sequence or mid UTF-8 rune: just after the first
// newline (a CSI never contains one), else at the first ESC (the start of a
// sequence), else at the first rune start.
// ponytail: a newline inside an OSC/DCS payload (rare) would still cut that
// string; a vt-based screen snapshot is the upgrade if that ever bites.
func cleanStart(b []byte) int {
	w := b[:min(len(b), cleanBoundaryWindow)]
	if i := bytes.IndexByte(w, '\n'); i >= 0 {
		return i + 1
	}
	if i := bytes.IndexByte(w, 0x1b); i >= 0 {
		return i
	}
	i := 0
	for i < len(w) && !utf8.RuneStart(w[i]) {
		i++
	}
	return i
}
