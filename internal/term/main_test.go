package term

import (
	"os"
	"testing"
)

// TestMain bookends this package's tests with a teardown of the isolated
// tmux server (see tmuxSocket's comment for why the isolation exists — note
// that the isolation itself is automatic, so this TestMain is hygiene, not
// the mechanism that provides it).
//
// Killing at BOTH ends is the point: the startup call clears whatever a
// previously-killed run left behind, so a `go test` timeout or Ctrl+C
// can't accumulate strays across runs.
func TestMain(m *testing.M) {
	KillTestTmuxServer()
	code := m.Run()
	// os.Exit skips deferred calls, so tear down explicitly first.
	KillTestTmuxServer()
	os.Exit(code)
}
