package ptyd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// socketEnv overrides the socket path (a second instance, or tests).
const socketEnv = "WORKTREE_STUDIO_PTYD_SOCKET"

// DefaultSocket is ~/.worktree-studio/ptyd.sock unless overridden by
// WORKTREE_STUDIO_PTYD_SOCKET.
func DefaultSocket() string {
	if s := os.Getenv(socketEnv); s != "" {
		return s
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".worktree-studio", "ptyd.sock")
}

// ErrNoSession means the daemon has no session by that id (it exited, was
// killed, or the daemon itself isn't running).
var ErrNoSession = errors.New("ptyd: " + errNoSessionText)

// Client talks to the daemon at Socket.
type Client struct {
	Socket string
	// AutoStart lets Create spawn the daemon (this executable, `ptyd`
	// subcommand) when nothing answers on Socket.
	AutoStart bool
}

func (c *Client) dial() (net.Conn, error) { return net.DialTimeout("unix", c.Socket, time.Second) }

// Running reports whether a daemon answers on Socket. Never starts one.
func (c *Client) Running() bool {
	conn, err := c.dial()
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// start spawns the daemon fully detached (own session, no inherited stdio)
// so it outlives the API server, then waits for its socket.
func (c *Client) start() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(c.Socket)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(dir, "ptyd.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command(self, "ptyd")
	cmd.Env = append(os.Environ(), socketEnv+"="+c.Socket)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ptyd: %w", err)
	}
	go cmd.Wait() // reap it if it exits early (e.g. lost the start race)
	for i := 0; i < 50; i++ {
		if c.Running() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("ptyd did not come up on %s; see %s", c.Socket, filepath.Join(dir, "ptyd.log"))
}

// roundTrip sends req and reads the response line, leaving conn open for
// attach's frames.
func (c *Client) roundTrip(req Request) (net.Conn, *bufio.Reader, response, error) {
	conn, err := c.dial()
	if err != nil {
		return nil, nil, response{}, err
	}
	b, _ := json.Marshal(req)
	if _, err := conn.Write(append(b, '\n')); err != nil {
		conn.Close()
		return nil, nil, response{}, err
	}
	br := bufio.NewReader(conn)
	line, err := br.ReadBytes('\n')
	if err != nil {
		conn.Close()
		return nil, nil, response{}, err
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		conn.Close()
		return nil, nil, response{}, err
	}
	if resp.Error != "" {
		conn.Close()
		if resp.Error == errNoSessionText {
			return nil, nil, resp, ErrNoSession
		}
		return nil, nil, resp, errors.New("ptyd: " + resp.Error)
	}
	return conn, br, resp, nil
}

func (c *Client) do(req Request) (response, error) {
	conn, _, resp, err := c.roundTrip(req)
	if err != nil {
		return resp, err
	}
	conn.Close()
	return resp, nil
}

// Create starts argv in a new PTY under id.
func (c *Client) Create(id, cwd string, argv, env []string, cols, rows uint16) error {
	if c.AutoStart && !c.Running() {
		if err := c.start(); err != nil {
			return err
		}
	}
	_, err := c.do(Request{Op: "create", ID: id, Cwd: cwd, Argv: argv, Env: env, Cols: cols, Rows: rows})
	return err
}

// List returns the live sessions; none (not an error) if no daemon runs.
func (c *Client) List() ([]Session, error) {
	if !c.Running() {
		return nil, nil
	}
	resp, err := c.do(Request{Op: "list"})
	return resp.Sessions, err
}

// Get returns one live session, or ErrNoSession.
func (c *Client) Get(id string) (Session, error) {
	all, err := c.List()
	if err != nil {
		return Session{}, err
	}
	for _, s := range all {
		if s.ID == id {
			return s, nil
		}
	}
	return Session{}, ErrNoSession
}

func (c *Client) Write(id string, p []byte) error {
	_, err := c.do(Request{Op: "write", ID: id, Data: p})
	return err
}

func (c *Client) Resize(id string, cols, rows uint16) error {
	_, err := c.do(Request{Op: "resize", ID: id, Cols: cols, Rows: rows})
	return err
}

// Kill hangs up the session. A missing session or daemon is not an error.
func (c *Client) Kill(id string) error {
	_, err := c.do(Request{Op: "kill", ID: id})
	if errors.Is(err, ErrNoSession) || !c.Running() {
		return nil
	}
	return err
}

// Attach connects to a session. It returns the scrollback replay (already
// cut at a clean boundary) and a Stream that carries live output from the
// moment the replay was taken.
func (c *Client) Attach(id string) ([]byte, *Stream, error) {
	conn, br, _, err := c.roundTrip(Request{Op: "attach", ID: id})
	if err != nil {
		if !errors.Is(err, ErrNoSession) && !c.Running() {
			err = ErrNoSession
		}
		return nil, nil, err
	}
	var replay []byte
	for {
		typ, p, err := readFrame(br)
		if err != nil {
			conn.Close()
			return nil, nil, err
		}
		if typ == frameReplayDone {
			break
		}
		replay = append(replay, p...)
	}
	return replay, &Stream{conn: conn, br: br, ExitCode: -1}, nil
}

// Stream is one attached client. Read returns live output and io.EOF once
// the session's process exits (ExitCode is then set) or the daemon drops
// the connection (ExitCode stays -1).
type Stream struct {
	conn     net.Conn
	br       *bufio.Reader
	wmu      sync.Mutex
	pending  []byte
	ExitCode int
}

func (s *Stream) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		typ, data, err := readFrame(s.br)
		if err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrUnexpectedEOF) {
				err = io.EOF
			}
			return 0, err
		}
		switch typ {
		case frameOutput:
			s.pending = data
		case frameExit:
			s.ExitCode, _ = strconv.Atoi(string(data))
			return 0, io.EOF
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

func (s *Stream) Write(p []byte) (int, error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := writeFrame(s.conn, frameInput, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *Stream) Resize(cols, rows uint16) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return writeFrame(s.conn, frameResize, resizePayload(cols, rows))
}

// Close detaches; the session keeps running.
func (s *Stream) Close() error { return s.conn.Close() }
