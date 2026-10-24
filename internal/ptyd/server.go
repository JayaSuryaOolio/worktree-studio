// Package ptyd is worktree-studio's PTY daemon: a separate long-lived
// process (`wts ptyd`) that owns every terminal's PTY, so shells survive the
// API server restarting or being upgraded (the same split as VS Code's
// ptyHost). The API server, `wts attach` and the TUI are thin clients over a
// unix socket. See docs/ptyd.md for the protocol and the reasoning.
package ptyd

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Wire protocol. Every connection starts with one JSON Request line from
// the client and one JSON response line from the daemon. For "attach" the
// connection then carries frames: [type byte][uint32 big-endian length]
// [payload].
const (
	frameOutput     = 'o' // daemon -> client: pty output
	frameReplayDone = 'r' // daemon -> client: scrollback replay finished, live from here
	frameExit       = 'x' // daemon -> client: process exited; payload is the decimal exit code
	frameInput      = 'i' // client -> daemon: bytes for the pty
	frameResize     = 's' // client -> daemon: 4 bytes, cols then rows (uint16 BE)
)

// maxFrame caps a frame's payload so a corrupt length can't make either side
// allocate gigabytes.
const maxFrame = 16 << 20

// Request is the first line of every connection. Op is one of create, list,
// attach, resize, write, kill.
type Request struct {
	Op   string   `json:"op"`
	ID   string   `json:"id,omitempty"`
	Cwd  string   `json:"cwd,omitempty"`
	Argv []string `json:"argv,omitempty"`
	Env  []string `json:"env,omitempty"`
	Cols uint16   `json:"cols,omitempty"`
	Rows uint16   `json:"rows,omitempty"`
	Data []byte   `json:"data,omitempty"`
}

// Session describes one live PTY.
type Session struct {
	ID           string    `json:"id"`
	Pid          int       `json:"pid"`
	Cwd          string    `json:"cwd"` // where it was started, not where it is now
	Created      time.Time `json:"created"`
	LastActivity time.Time `json:"last_activity"` // last output
}

type response struct {
	Error    string    `json:"error,omitempty"`
	Sessions []Session `json:"sessions,omitempty"`
}

const errNoSessionText = "no such session"

func writeFrame(w io.Writer, typ byte, p []byte) error {
	b := make([]byte, 5, 5+len(p))
	b[0] = typ
	binary.BigEndian.PutUint32(b[1:], uint32(len(p)))
	_, err := w.Write(append(b, p...))
	return err
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > maxFrame {
		return 0, nil, fmt.Errorf("ptyd: frame too large (%d bytes)", n)
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return 0, nil, err
	}
	return hdr[0], p, nil
}

func resizePayload(cols, rows uint16) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b, cols)
	binary.BigEndian.PutUint16(b[2:], rows)
	return b
}

// replyPattern matches what a terminal emulator sends back on its own when
// it processes a query in its output: cursor position / device status /
// device attributes / window reports, DECRQM, and OSC or DCS replies.
var replyPattern = regexp.MustCompile(`^(\x1b\[[?>=]?[0-9;]*[Rcnt]|\x1b\[\?[0-9;]*\$y|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1bP[^\x1b]*\x1b\\)+$`)

// IsTerminalReply reports whether p consists only of terminal-generated
// replies. Clients use it to drop the replies a replay provokes, which would
// otherwise be typed into the shell as if the user had sent them.
func IsTerminalReply(p []byte) bool { return replyPattern.Match(p) }

// Server is the daemon: it owns the PTYs and serves the socket protocol.
type Server struct {
	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	Session
	ptmx *os.File
	cmd  *exec.Cmd

	mu       sync.Mutex // guards everything below, and LastActivity
	buf      scrollback
	subs     map[chan []byte]bool
	exited   bool
	exitCode int
}

// ErrAlreadyRunning means another daemon holds the socket's lock.
var ErrAlreadyRunning = errors.New("ptyd: already running")

// ListenAndServe runs the daemon on socket until it fails. A lock file next
// to the socket makes concurrent auto-starts safe: the loser returns
// ErrAlreadyRunning instead of deleting the winner's socket.
func ListenAndServe(socket string) error {
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(socket+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close() // also keeps the fd (and so the lock) alive while serving
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return ErrAlreadyRunning
	}
	_ = os.Remove(socket) // stale socket from a daemon that died
	ln, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	_ = os.Chmod(socket, 0o600)
	return NewServer().Serve(ln)
}

func NewServer() *Server { return &Server{sessions: map[string]*session{}} }

// Serve accepts connections on ln until it fails.
func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

func reply(conn net.Conn, r response) error {
	b, _ := json.Marshal(r)
	_, err := conn.Write(append(b, '\n'))
	return err
}

func (s *Server) get(id string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

func (s *Server) handle(conn net.Conn) {
	br := bufio.NewReader(conn)
	line, err := br.ReadBytes('\n')
	if err != nil {
		conn.Close()
		return
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		_ = reply(conn, response{Error: "bad request: " + err.Error()})
		conn.Close()
		return
	}
	if req.Op == "attach" {
		s.attach(conn, br, req.ID) // owns conn from here
		return
	}
	defer conn.Close()
	if req.Op == "create" {
		_ = reply(conn, errResponse(s.create(req)))
		return
	}
	if req.Op == "list" {
		_ = reply(conn, response{Sessions: s.list()})
		return
	}
	sess := s.get(req.ID)
	if sess == nil {
		_ = reply(conn, response{Error: errNoSessionText})
		return
	}
	switch req.Op {
	case "write":
		_, err = sess.ptmx.Write(req.Data)
	case "resize":
		err = pty.Setsize(sess.ptmx, &pty.Winsize{Cols: req.Cols, Rows: req.Rows})
	case "kill":
		sess.kill()
	default:
		err = fmt.Errorf("unknown op %q", req.Op)
	}
	_ = reply(conn, errResponse(err))
}

func errResponse(err error) response {
	if err != nil {
		return response{Error: err.Error()}
	}
	return response{}
}

func (s *Server) list() []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sess.mu.Lock()
		out = append(out, sess.Session)
		sess.mu.Unlock()
	}
	return out
}

func (s *Server) create(req Request) error {
	if req.ID == "" || len(req.Argv) == 0 {
		return errors.New("create needs id and argv")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[req.ID] != nil {
		return fmt.Errorf("session %s already exists", req.ID)
	}
	cols, rows := req.Cols, req.Rows
	if cols == 0 || rows == 0 {
		cols, rows = 80, 24
	}
	cmd := exec.Command(req.Argv[0], req.Argv[1:]...)
	cmd.Dir = req.Cwd
	cmd.Env = req.Env
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return fmt.Errorf("start %s: %w", req.Argv[0], err)
	}
	now := time.Now()
	sess := &session{
		Session: Session{ID: req.ID, Pid: cmd.Process.Pid, Cwd: req.Cwd, Created: now, LastActivity: now},
		ptmx:    ptmx,
		cmd:     cmd,
		buf:     scrollback{max: MaxScrollback},
		subs:    map[chan []byte]bool{},
	}
	s.sessions[req.ID] = sess
	go s.run(sess)
	return nil
}

// run pumps a session's output until its process exits, then tears it down.
func (s *Server) run(sess *session) {
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 32*1024)
		for {
			n, err := sess.ptmx.Read(buf)
			if n > 0 {
				sess.broadcast(append([]byte(nil), buf[:n]...))
			}
			if err != nil {
				return
			}
		}
	}()
	_ = sess.cmd.Wait()
	// Let the reader drain output written just before exit; a background job
	// still holding the tty open would otherwise keep it from ever ending.
	select {
	case <-readDone:
	case <-time.After(200 * time.Millisecond):
	}
	sess.ptmx.Close()
	<-readDone

	sess.mu.Lock()
	sess.exited = true
	sess.exitCode = sess.cmd.ProcessState.ExitCode()
	for ch := range sess.subs {
		delete(sess.subs, ch)
		close(ch)
	}
	sess.mu.Unlock()

	s.mu.Lock()
	delete(s.sessions, sess.ID)
	s.mu.Unlock()
}

// subBuffer is how many output chunks an attached client may lag behind
// before it is dropped (it can reattach and get the replay).
const subBuffer = 256

func (sess *session) broadcast(p []byte) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	sess.buf.write(p)
	sess.LastActivity = time.Now()
	for ch := range sess.subs {
		select {
		case ch <- p:
		default: // too slow: drop rather than stall the shell
			delete(sess.subs, ch)
			close(ch)
		}
	}
}

func (sess *session) unsubscribe(ch chan []byte) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.subs[ch] {
		delete(sess.subs, ch)
		close(ch)
	}
}

// kill hangs up the session's process group, like closing a terminal
// window; run() notices the exit and cleans up.
func (sess *session) kill() {
	_ = syscall.Kill(-sess.Pid, syscall.SIGHUP)
	go func() {
		time.Sleep(2 * time.Second)
		sess.mu.Lock()
		defer sess.mu.Unlock()
		if !sess.exited { // it ignored SIGHUP
			_ = syscall.Kill(-sess.Pid, syscall.SIGKILL)
		}
	}()
}

func (s *Server) attach(conn net.Conn, br *bufio.Reader, id string) {
	defer conn.Close()
	sess := s.get(id)
	if sess == nil {
		_ = reply(conn, response{Error: errNoSessionText})
		return
	}
	// Snapshot and subscribe under one lock so no output is lost or doubled
	// between the replay and the live stream.
	ch := make(chan []byte, subBuffer)
	sess.mu.Lock()
	if sess.exited {
		sess.mu.Unlock()
		_ = reply(conn, response{Error: errNoSessionText})
		return
	}
	replay := sess.buf.replay()
	sess.subs[ch] = true
	sess.mu.Unlock()
	defer sess.unsubscribe(ch)

	if reply(conn, response{}) != nil {
		return
	}
	go func() {
		for {
			typ, p, err := readFrame(br)
			if err != nil {
				conn.Close() // unblocks the writer below
				return
			}
			switch typ {
			case frameInput:
				_, _ = sess.ptmx.Write(p)
			case frameResize:
				if len(p) == 4 {
					_ = pty.Setsize(sess.ptmx, &pty.Winsize{Cols: binary.BigEndian.Uint16(p), Rows: binary.BigEndian.Uint16(p[2:])})
				}
			}
		}
	}()

	for len(replay) > 0 {
		n := min(len(replay), 32*1024)
		if writeFrame(conn, frameOutput, replay[:n]) != nil {
			return
		}
		replay = replay[n:]
	}
	if writeFrame(conn, frameReplayDone, nil) != nil {
		return
	}
	for p := range ch {
		if writeFrame(conn, frameOutput, p) != nil {
			return
		}
	}
	sess.mu.Lock()
	exited, code := sess.exited, sess.exitCode
	sess.mu.Unlock()
	if exited {
		_ = writeFrame(conn, frameExit, []byte(strconv.Itoa(code)))
	}
}
