package term

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// testTmuxSocketPrefix namespaces the dedicated tmux servers that tests run
// against instead of the user's real one.
//
// The full socket name appends the test binary's own name, so each test
// BINARY gets its own server (worktree-studio-tests-term,
// worktree-studio-tests-api, ...). That per-binary split is load-bearing,
// not tidiness: `go test ./...` runs different packages in PARALLEL, and
// each package's TestMain kills its server at startup and teardown. With a
// single shared socket, internal/api's TestMain tore down the server out
// from under internal/term's still-running tests — which showed up as
// TestCurrentPathAfterCd failing only when the two packages ran together,
// and passing every time either ran alone.
const testTmuxSocketPrefix = "worktree-studio-tests"

// tmuxSocketEnv force-selects a tmux socket. Mainly an escape hatch for CI
// or for debugging the isolation itself; ordinary runs never set it.
const tmuxSocketEnv = "WORKTREE_STUDIO_TMUX_SOCKET"

// tmuxSocket returns the socket name every tmux command in this package
// should target, or "" for the default (real) server.
//
// In production this is always "" — worktree-studio deliberately uses the
// ordinary tmux server, so a session survives a server restart and stays
// reachable from a plain `tmux attach` outside the app (see this package's
// doc comment).
//
// Under `go test` it returns a per-test-binary socket name, and that is
// decided AUTOMATICALLY rather than by each test package opting in. That is
// deliberate and was learned the hard way: the first version of this
// isolation was opt-in via a TestMain in this package only, which silently
// missed internal/api's tests — they construct a term.Manager and call
// CreateSession from a *different* test binary, where this package's
// TestMain never runs, so they kept hitting the real server. Nine fresh
// orphans showed up within minutes of "fixing" the leak. Any future package
// that calls into this one would have had the same problem, and would have
// failed just as silently.
//
// What that silent failure costs, concretely:
//
//  1. Tests create real tmux sessions. A passing run cleans up via
//     t.Cleanup, but an abnormal exit — `go test` timeout, Ctrl+C, a panic
//     — skips cleanup and leaks them permanently, with no DB row to ever
//     reconcile against (test stores are throwaway temp-dir SQLite files,
//     so Reconcile cannot see them). 61 such sessions had accumulated on
//     this machine before any of this existed.
//  2. Worse and less obvious: CreateSession sets SERVER-WIDE options
//     (mouse, set-clipboard, set-titles, xterm-keys, extended-keys) and
//     rebinds the global copy-mode key tables. So merely running `go test`
//     reconfigured the developer's live tmux server and every session they
//     had open at the time.
//
// Detection is by test-binary name: `go test` builds and runs a binary
// ending in ".test". This avoids importing the testing package from
// non-test code (which registers test flags as a side effect). An
// explicitly-set WORKTREE_STUDIO_TMUX_SOCKET always wins.
func tmuxSocket() string {
	if s := os.Getenv(tmuxSocketEnv); s != "" {
		return s
	}
	if len(os.Args) > 0 && strings.HasSuffix(os.Args[0], ".test") {
		return testTmuxSocketPrefix + "-" + strings.TrimSuffix(filepath.Base(os.Args[0]), ".test")
	}
	return ""
}

// TmuxCmd builds a tmux command targeting whatever tmuxSocket says. Every
// tmux invocation in this package must go through here rather than calling
// exec.Command directly — one missed call site silently reaches the real
// server and reintroduces exactly what this exists to prevent.
func TmuxCmd(args ...string) *exec.Cmd {
	if s := tmuxSocket(); s != "" {
		return exec.Command("tmux", append([]string{"-L", s}, args...)...)
	}
	return exec.Command("tmux", args...)
}

// KillTestTmuxServer tears down the isolated tmux server belonging to the
// CURRENTLY RUNNING test binary (never another package's — see
// testTmuxSocketPrefix for why that distinction matters). Safe to call when
// no such server exists; that's a no-op, not an error.
//
// Intended for a TestMain to call at both ends of a run: the teardown call
// cleans up after a normal run, and the startup call cleans up after a
// previous run that was killed before it could tear down — which is what
// makes stray test sessions self-healing rather than merely less likely.
//
// Does nothing outside a test binary, so it cannot touch the real server.
func KillTestTmuxServer() {
	s := tmuxSocket()
	if s == "" {
		return
	}
	_ = exec.Command("tmux", "-L", s, "kill-server").Run()
}
