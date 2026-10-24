// Package ptyterm implements app.Attacher: it connects to a worktree's
// terminal session (`tmux attach` in a pty, or the ptyd daemon directly) and
// feeds the output through an in-process terminal emulator, so a Bubble Tea
// view can show (and type into) it.
package ptyterm

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"worktree-studio/internal/ptyd"
	"worktree-studio/tui/app"
	"worktree-studio/tui/domain"
)

type Attacher struct {
	// PtydSocket overrides ptyd.DefaultSocket() (tests).
	PtydSocket string
}

func (a Attacher) Attach(s domain.TerminalSession, w, h int) (app.Screen, error) {
	if s.Backend == "ptyd" {
		return a.attachPtyd(s.ID, w, h)
	}
	cmd := exec.Command("tmux", "attach-session", "-t", s.TmuxName)
	// Drop TMUX so this works when wts-tui itself runs inside tmux; the
	// session lives on the default server, which is what tmux picks anyway.
	cmd.Env = append(filterEnv(os.Environ(), "TMUX", "TERM"), "TERM=xterm-256color", "COLORTERM=truecolor")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	if err != nil {
		return nil, err
	}
	sc := newScreen(w, h, ptmx, func(w, h int) { _ = pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)}) })
	sc.cmd = cmd
	go sc.pump(nil)
	return sc, nil
}

// attachPtyd talks to the daemon directly: no pty of our own, the replayed
// scrollback is fed into the emulator before live output.
func (a Attacher) attachPtyd(id string, w, h int) (app.Screen, error) {
	sock := a.PtydSocket
	if sock == "" {
		sock = ptyd.DefaultSocket()
	}
	replay, st, err := (&ptyd.Client{Socket: sock}).Attach(id)
	if err != nil {
		return nil, err
	}
	_ = st.Resize(uint16(w), uint16(h))
	sc := newScreen(w, h, st, func(w, h int) { _ = st.Resize(uint16(w), uint16(h)) })
	go sc.pump(replay)
	return sc, nil
}

func newScreen(w, h int, conn io.ReadWriteCloser, resize func(w, h int)) *screen {
	sc := &screen{emu: vt.NewSafeEmulator(w, h), conn: conn, resize: resize, updates: make(chan struct{}, 1)}
	sc.cursorVisible.Store(true)
	sc.emu.SetCallbacks(vt.Callbacks{CursorVisibility: sc.cursorVisible.Store})
	return sc
}

type screen struct {
	emu     *vt.SafeEmulator
	conn    io.ReadWriteCloser // the tmux client's pty, or a ptyd stream
	resize  func(w, h int)
	cmd     *exec.Cmd // the tmux client; nil for ptyd
	updates chan struct{}
	once    sync.Once

	// mu makes Render's draw-cursor/render/restore atomic against pump's writes.
	mu            sync.Mutex
	cursorVisible atomic.Bool // apps like Claude Code hide it and draw their own
}

// pump copies session output into the emulator, and the emulator's own
// replies (cursor-position reports, etc.) back to the session. Replies the
// emulator makes while parsing replay are dropped: they answer queries from
// the past, and the shell would read them as typed input. The emulator's
// reply pipe is ordered and never merges writes, so everything before our
// marker coming back out belongs to the replay.
func (s *screen) pump(replay []byte) {
	defer close(s.updates)
	defer s.Close() // the attach exited: release the pty, or they run out system-wide
	var marker []byte
	if len(replay) > 0 {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		marker = []byte("\x00wts-replay-" + hex.EncodeToString(b))
	}
	go func() { // ends when the emulator or the connection closes
		buf := make([]byte, 4096)
		for {
			n, err := s.emu.Read(buf)
			if err != nil {
				return
			}
			if marker != nil {
				if bytes.Equal(buf[:n], marker) {
					marker = nil
				}
				continue
			}
			if _, err := s.conn.Write(buf[:n]); err != nil {
				return
			}
		}
	}()
	if marker != nil {
		s.mu.Lock()
		s.emu.Write(replay)
		s.mu.Unlock()
		_, _ = s.emu.InputPipe().Write(marker)
		s.notify()
	}
	buf := make([]byte, 32*1024)
	for {
		n, err := s.conn.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.emu.Write(buf[:n])
			s.mu.Unlock()
			s.notify()
		}
		if err != nil {
			return
		}
	}
}

func (s *screen) notify() {
	select { // non-blocking: the UI coalesces redraws
	case s.updates <- struct{}{}:
	default:
	}
}

func (s *screen) Write(p []byte) error { _, err := s.conn.Write(p); return err }

// Render returns the frame with the cursor cell in reverse video (the
// emulator's own Render doesn't draw one).
func (s *screen) Render() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.cursorVisible.Load() {
		return s.emu.Render()
	}
	pos := s.emu.CursorPosition()
	orig := s.emu.CellAt(pos.X, pos.Y)
	cell := uv.Cell{Content: " ", Width: 1}
	if orig != nil {
		cell = *orig
	}
	cell.Style.Attrs |= uv.AttrReverse
	s.emu.SetCell(pos.X, pos.Y, &cell)
	out := s.emu.Render()
	s.emu.SetCell(pos.X, pos.Y, orig)
	return out
}
func (s *screen) Updates() <-chan struct{} { return s.updates }

func (s *screen) Scroll(x, y int, up bool) {
	b := uv.MouseWheelDown
	if up {
		b = uv.MouseWheelUp
	}
	s.emu.SendMouse(uv.MouseWheelEvent{X: x, Y: y, Button: b})
}

func (s *screen) Resize(w, h int) {
	s.emu.Resize(w, h)
	s.resize(w, h)
}

// Close detaches (our tmux client, or the ptyd stream); the session itself
// keeps running.
func (s *screen) Close() {
	s.once.Do(func() {
		s.conn.Close()
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
			go s.cmd.Wait()
		}
		// Close the emulator's reply pipe, not the emulator: that ends pump's
		// io.Copy with EOF, whereas emu.Close writes vt's unguarded closed flag
		// while that copy is still blocked in emu.Read (a data race).
		if pw, ok := s.emu.InputPipe().(io.Closer); ok {
			_ = pw.Close()
		} else {
			s.emu.Close()
		}
	})
}

func filterEnv(env []string, drop ...string) []string {
	out := env[:0:0]
outer:
	for _, e := range env {
		for _, d := range drop {
			if len(e) > len(d) && e[:len(d)+1] == d+"=" {
				continue outer
			}
		}
		out = append(out, e)
	}
	return out
}
