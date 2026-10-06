// Package ptyterm implements app.Attacher: it runs `tmux attach` in a pty
// and feeds the output through an in-process terminal emulator, so a
// Bubble Tea view can show (and type into) a worktree's tmux session.
package ptyterm

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"worktree-studio/tui/app"
	"worktree-studio/tui/domain"
)

type Attacher struct{}

func (Attacher) Attach(s domain.TerminalSession, w, h int) (app.Screen, error) {
	cmd := exec.Command("tmux", "attach-session", "-t", s.TmuxName)
	// Drop TMUX so this works when wts-tui itself runs inside tmux; the
	// session lives on the default server, which is what tmux picks anyway.
	cmd.Env = append(filterEnv(os.Environ(), "TMUX", "TERM"), "TERM=xterm-256color", "COLORTERM=truecolor")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	if err != nil {
		return nil, err
	}
	sc := &screen{emu: vt.NewSafeEmulator(w, h), ptmx: ptmx, cmd: cmd, updates: make(chan struct{}, 1)}
	sc.cursorVisible.Store(true)
	sc.emu.SetCallbacks(vt.Callbacks{CursorVisibility: sc.cursorVisible.Store})
	go sc.pump()
	return sc, nil
}

type screen struct {
	emu     *vt.SafeEmulator
	ptmx    *os.File
	cmd     *exec.Cmd
	updates chan struct{}
	once    sync.Once

	// mu makes Render's draw-cursor/render/restore atomic against pump's writes.
	mu            sync.Mutex
	cursorVisible atomic.Bool // apps like Claude Code hide it and draw their own
}

// pump copies pty output into the emulator, and the emulator's own replies
// (cursor-position reports, etc.) back to the pty.
func (s *screen) pump() {
	defer close(s.updates)
	go io.Copy(s.ptmx, s.emu) // ends when the emulator or pty closes
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.emu.Write(buf[:n])
			s.mu.Unlock()
			select { // non-blocking: the UI coalesces redraws
			case s.updates <- struct{}{}:
			default:
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *screen) Write(p []byte) error { _, err := s.ptmx.Write(p); return err }

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
	_ = pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
}

// Close detaches our tmux client; the session itself keeps running.
func (s *screen) Close() {
	s.once.Do(func() {
		s.ptmx.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
			go s.cmd.Wait()
		}
		s.emu.Close()
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
