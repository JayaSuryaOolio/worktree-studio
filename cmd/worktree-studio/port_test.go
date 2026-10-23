package main

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListenFirstFreeSkipsBusyPort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	ln, addr, err := listenFirstFree(busy.Addr().String(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if addr == busy.Addr().String() {
		t.Fatalf("bound the busy port %s", addr)
	}
}

func TestServerBaseURLPrefersEnvThenRunningServer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WORKTREE_STUDIO_ADDR", "")
	if got := serverBaseURL(); got != "http://localhost:8787" {
		t.Fatalf("no server, no env: got %s", got)
	}

	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()
	dir := filepath.Join(home, ".worktree-studio")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "server.addr"), []byte(strings.TrimPrefix(ts.URL, "http://")), 0o644)
	if got, want := serverBaseURL(), "http://localhost:"+strings.Split(ts.URL, ":")[2]; got != want {
		t.Fatalf("running server: got %s want %s", got, want)
	}

	t.Setenv("WORKTREE_STUDIO_ADDR", ":9000")
	if got := serverBaseURL(); got != "http://localhost:9000" {
		t.Fatalf("env override: got %s", got)
	}
}
