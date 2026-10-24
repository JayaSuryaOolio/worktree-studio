package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"

	"worktree-studio/internal/ptyd"
	wtsterm "worktree-studio/internal/term"
)

// ptydServe is `wts ptyd`: the PTY daemon in the foreground. The API server
// normally starts it on demand (see ptyd.Client.start); see docs/ptyd.md.
func ptydServe() {
	signal.Ignore(syscall.SIGHUP) // outlive whatever terminal started us
	sock := ptyd.DefaultSocket()
	fmt.Println("ptyd listening on", sock)
	err := ptyd.ListenAndServe(sock)
	if errors.Is(err, ptyd.ErrAlreadyRunning) {
		fmt.Println("ptyd already running on", sock)
		return
	}
	die("ptyd: %v", err)
}

// detachKey (Ctrl+\, as in dtach/abduco) ends `wts attach` without
// touching the session.
const detachKey = 0x1c

// attachTerminal is `wts attach <id>`: this terminal becomes a client of a
// terminal session. ptyd sessions attach straight to the daemon; anything
// else is assumed to be a tmux one (`tmux attach -t wts-<id>`).
func attachTerminal(id string) {
	c := &ptyd.Client{Socket: ptyd.DefaultSocket()}
	replay, st, err := c.Attach(id)
	if errors.Is(err, ptyd.ErrNoSession) {
		cmd := wtsterm.TmuxCmd("attach-session", "-t", "wts-"+id)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if cmd.Run() != nil {
			die("no ptyd or tmux terminal session %q", id)
		}
		return
	}
	if err != nil {
		die("attach: %v", err)
	}
	defer st.Close()

	fd := os.Stdin.Fd()
	state, err := term.MakeRaw(fd)
	if err != nil {
		die("attach needs a terminal: %v", err)
	}
	restore := func() { _ = term.Restore(fd, state) }
	defer restore()

	resize := func() {
		if ws, err := pty.GetsizeFull(os.Stdin); err == nil {
			_ = st.Resize(ws.Cols, ws.Rows)
		}
	}
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			resize()
		}
	}()
	resize()

	os.Stdout.Write(replay)
	replayed := time.Now()

	detached := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			p := buf[:n]
			// Our terminal answering queries in the replay we just printed;
			// the shell must not see those as typed input.
			// ponytail: time window, not exact; a marker like the TUI's
			// isn't possible since the real terminal's replies aren't ours.
			if time.Since(replayed) < 250*time.Millisecond && ptyd.IsTerminalReply(p) {
				continue
			}
			for i, b := range p {
				if b == detachKey {
					st.Write(p[:i])
					close(detached)
					return
				}
			}
			if _, err := st.Write(p); err != nil {
				return
			}
		}
	}()
	ended := make(chan struct{})
	go func() {
		_, _ = io.Copy(os.Stdout, st)
		close(ended)
	}()
	select {
	case <-detached:
		restore()
		fmt.Println("\r\n[detached]")
	case <-ended:
		restore()
		if st.ExitCode >= 0 {
			fmt.Printf("\r\n[process exited with code %d]\n", st.ExitCode)
		} else {
			fmt.Println("\r\n[connection to ptyd lost]")
		}
	}
}
