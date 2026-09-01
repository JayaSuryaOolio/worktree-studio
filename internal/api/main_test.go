package api

import (
	"os"
	"testing"

	"worktree-studio/internal/term"
)

// TestMain tears down the isolated tmux server that this package's terminal
// tests create sessions on. Several tests here construct a term.Manager and
// call CreateSession for real; internal/term routes those at a dedicated
// tmux server automatically under `go test` (see term's tmuxSocket comment),
// so nothing here touches the developer's real tmux server or its
// server-wide options.
//
// This package needs its own TestMain because a TestMain is per test
// BINARY: internal/term's covers only its own package. Missing that is
// exactly how the first version of this isolation leaked — the sessions
// these tests create went straight to the real server.
//
// Killing at both ends means a run killed before teardown (a `go test`
// timeout, Ctrl+C) is cleaned up by the next run rather than accumulating.
func TestMain(m *testing.M) {
	term.KillTestTmuxServer()
	code := m.Run()
	// os.Exit skips deferred calls, so tear down explicitly first.
	term.KillTestTmuxServer()
	os.Exit(code)
}
