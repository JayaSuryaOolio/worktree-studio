package term

import (
	"testing"
	"time"
)

// TestFindOrphanTmuxSessionsIgnoresKnownSessions confirms a session created
// (and thus recorded) through Manager.CreateSession never shows up as an
// orphan, regardless of activity — it has a terminal_sessions row, so it's
// not an orphan by definition.
func TestFindOrphanTmuxSessionsIgnoresKnownSessions(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)
	m := &Manager{Store: st}

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { _ = m.CloseSession(ts) })

	orphans, err := FindOrphanTmuxSessions(st, 0)
	if err != nil {
		t.Fatalf("FindOrphanTmuxSessions: %v", err)
	}
	for _, o := range orphans {
		if o.Name == ts.TmuxSessionName {
			t.Fatalf("expected a known (DB-backed) session to never be reported as an orphan, got %+v", o)
		}
	}
}

// TestFindOrphanTmuxSessionsDetectsUnknownSession confirms a tmux session
// created directly (not through Manager.CreateSession, so it has no DB
// row) — simulating a leaked test session or one made by hand outside the
// app — is detected as an orphan.
func TestFindOrphanTmuxSessionsDetectsUnknownSession(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)

	name := tmuxNamePrefix + "orphantest1"
	if err := TmuxCmd("new-session", "-d", "-s", name).Run(); err != nil {
		t.Fatalf("tmux new-session: %v", err)
	}
	t.Cleanup(func() { _ = TmuxCmd("kill-session", "-t", name).Run() })

	orphans, err := FindOrphanTmuxSessions(st, time.Hour)
	if err != nil {
		t.Fatalf("FindOrphanTmuxSessions: %v", err)
	}
	var found *OrphanTmuxSession
	for i := range orphans {
		if orphans[i].Name == name {
			found = &orphans[i]
		}
	}
	if found == nil {
		t.Fatalf("expected %s to be reported as an orphan, got %+v", name, orphans)
	}
}

// TestFindOrphanTmuxSessionsIgnoresOutsideNamespace confirms a plain tmux
// session the user created themselves (no "wts-" prefix) is never reported
// as an orphan — this package must never touch a session it didn't create.
func TestFindOrphanTmuxSessionsIgnoresOutsideNamespace(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)

	name := "some-users-own-session"
	if err := TmuxCmd("new-session", "-d", "-s", name).Run(); err != nil {
		t.Fatalf("tmux new-session: %v", err)
	}
	t.Cleanup(func() { _ = TmuxCmd("kill-session", "-t", name).Run() })

	orphans, err := FindOrphanTmuxSessions(st, 0)
	if err != nil {
		t.Fatalf("FindOrphanTmuxSessions: %v", err)
	}
	for _, o := range orphans {
		if o.Name == name {
			t.Fatalf("expected a non-wts- session to never be reported as an orphan, got %+v", o)
		}
	}
}

// TestKillOrphanTmuxSessionsProtectsRecentActivity is the core safeguard
// this whole file exists for: an orphan session with recent activity (here,
// just created — its session_activity is effectively "now") must survive a
// kill sweep when minAge says "protect anything touched in the last hour."
func TestKillOrphanTmuxSessionsProtectsRecentActivity(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)

	name := tmuxNamePrefix + "orphantest2"
	if err := TmuxCmd("new-session", "-d", "-s", name).Run(); err != nil {
		t.Fatalf("tmux new-session: %v", err)
	}
	t.Cleanup(func() { _ = TmuxCmd("kill-session", "-t", name).Run() })

	killed, protected, err := KillOrphanTmuxSessions(st, time.Hour)
	if err != nil {
		t.Fatalf("KillOrphanTmuxSessions: %v", err)
	}
	for _, k := range killed {
		if k == name {
			t.Fatalf("expected a recently-active orphan to be protected, not killed (killed=%v)", killed)
		}
	}
	foundProtected := false
	for _, p := range protected {
		if p == name {
			foundProtected = true
		}
	}
	if !foundProtected {
		t.Fatalf("expected %s in the protected list, got protected=%v", name, protected)
	}

	live, err := ListLiveTmuxSessionNames()
	if err != nil {
		t.Fatalf("ListLiveTmuxSessionNames: %v", err)
	}
	if !live[name] {
		t.Fatalf("expected the protected session to still be alive after the sweep")
	}
}

// TestKillOrphanTmuxSessionsKillsStaleOrphan confirms the sweep actually
// kills an orphan whose activity falls outside the safety window — the
// other half of the safeguard: it has to do something, not just protect
// everything unconditionally. minAge=0 means "protect nothing," which is
// the only way to exercise this deterministically without waiting on real
// wall-clock time to pass.
func TestKillOrphanTmuxSessionsKillsStaleOrphan(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)

	name := tmuxNamePrefix + "orphantest3"
	if err := TmuxCmd("new-session", "-d", "-s", name).Run(); err != nil {
		t.Fatalf("tmux new-session: %v", err)
	}
	t.Cleanup(func() { _ = TmuxCmd("kill-session", "-t", name).Run() })

	killed, _, err := KillOrphanTmuxSessions(st, 0)
	if err != nil {
		t.Fatalf("KillOrphanTmuxSessions: %v", err)
	}
	found := false
	for _, k := range killed {
		if k == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s to be killed with minAge=0, got killed=%v", name, killed)
	}

	live, err := ListLiveTmuxSessionNames()
	if err != nil {
		t.Fatalf("ListLiveTmuxSessionNames: %v", err)
	}
	if live[name] {
		t.Fatalf("expected the stale orphan to be gone after the sweep")
	}
}
