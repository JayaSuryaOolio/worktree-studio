package ptyd

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScrollbackTrimsAndCutsCleanly(t *testing.T) {
	s := scrollback{max: 16}
	s.write([]byte("hello\n"))
	if got := string(s.replay()); got != "hello\n" {
		t.Fatalf("untrimmed replay = %q", got)
	}
	s.write([]byte("\x1b[31mred line\nnext"))
	if len(s.buf) != 16 || !s.trimmed {
		t.Fatalf("buf %q trimmed=%v, want last 16 bytes and trimmed", s.buf, s.trimmed)
	}
	if got := string(s.replay()); got != "next" {
		t.Fatalf("trimmed replay = %q, want cut after the first newline", got)
	}
}

func TestCleanStart(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"31mabc\ndef", "def"},           // mid-CSI: skip to after newline
		{"5;1Hfoo\x1b[0mbar", "\x1b[0mbar"}, // no newline: start at an ESC
		{"\xa9caf\xc3\xa9", "caf\xc3\xa9"},  // mid-rune: skip continuation bytes
		{"plain", "plain"},
	} {
		if got := tc.in[cleanStart([]byte(tc.in)):]; got != tc.want {
			t.Errorf("cleanStart(%q) -> %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsTerminalReply(t *testing.T) {
	for in, want := range map[string]bool{
		"\x1b[12;1R":              true,
		"\x1b[?1;2c":              true,
		"\x1b[>0;276;0c\x1b[0n":   true,
		"\x1b]11;rgb:0/0/0\x1b\\": true,
		"\x1b[A":                  false, // arrow key
		"ls\r":                    false,
	} {
		if IsTerminalReply([]byte(in)) != want {
			t.Errorf("IsTerminalReply(%q) != %v", in, want)
		}
	}
}

// startTestDaemon serves an in-process daemon on a short socket path (macOS
// caps unix socket paths at 104 bytes, which t.TempDir() can exceed).
func startTestDaemon(t *testing.T) *Client {
	t.Helper()
	dir, err := os.MkdirTemp("", "ptyd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer()
	go srv.Serve(ln)
	c := &Client{Socket: sock}
	t.Cleanup(func() {
		for _, s := range srv.list() {
			_ = c.Kill(s.ID)
		}
		ln.Close()
	})
	return c
}

// readUntil reads r until its accumulated output contains want.
func readUntil(t *testing.T, r io.Reader, want string) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		var all bytes.Buffer
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			all.Write(buf[:n])
			if strings.Contains(all.String(), want) || err != nil {
				got <- all.String()
				return
			}
		}
	}()
	select {
	case s := <-got:
		if !strings.Contains(s, want) {
			t.Fatalf("stream ended without %q; got %q", want, s)
		}
		return s
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %q", want)
		return ""
	}
}

// The point of ptyd: a client (the API server) going away doesn't take the
// shell with it, and reattaching replays what was printed before.
func TestDetachReattachReplays(t *testing.T) {
	c := startTestDaemon(t)
	if err := c.Create("t1", t.TempDir(), []string{"/bin/sh"}, []string{"PS1=$ ", "PATH=/usr/bin:/bin"}, 80, 24); err != nil {
		t.Fatal(err)
	}
	_, st, err := c.Attach("t1")
	if err != nil {
		t.Fatal(err)
	}
	// Split so the echoed command line itself doesn't match.
	st.Write([]byte("echo before-''detach\n"))
	readUntil(t, st, "before-detach")
	st.Close() // the API server going away

	replay, st2, err := c.Attach("t1")
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if !strings.Contains(string(replay), "before-detach") {
		t.Fatalf("replay %q lacks earlier output", replay)
	}
	st2.Write([]byte("echo still-''alive\n"))
	readUntil(t, st2, "still-alive")

	if sessions, _ := c.List(); len(sessions) != 1 || sessions[0].ID != "t1" || sessions[0].Pid == 0 {
		t.Fatalf("List = %+v", sessions)
	}
	st2.Write([]byte("exit 3\n"))
	io.Copy(io.Discard, st2)
	if st2.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", st2.ExitCode)
	}
	if _, err := c.Get("t1"); err != ErrNoSession {
		t.Fatalf("Get after exit: %v, want ErrNoSession", err)
	}
}

func TestKillEndsSession(t *testing.T) {
	c := startTestDaemon(t)
	if err := c.Create("k", "", []string{"/bin/sh"}, nil, 0, 0); err != nil {
		t.Fatal(err)
	}
	_, st, err := c.Attach("k")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Kill("k"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { io.Copy(io.Discard, st); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("attach stream did not end after kill")
	}
	if _, _, err := c.Attach("k"); err != ErrNoSession {
		t.Fatalf("Attach after kill: %v, want ErrNoSession", err)
	}
	if err := c.Kill("k"); err != nil {
		t.Fatalf("Kill of a gone session should be a no-op, got %v", err)
	}
}
