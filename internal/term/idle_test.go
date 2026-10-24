package term

import (
	"testing"
	"time"

	"worktree-studio/internal/ptyd"
)

func TestIdleState(t *testing.T) {
	now := time.Now()
	old := now.Add(-8 * 24 * time.Hour)
	week := 7 * 24 * time.Hour
	for _, tc := range []struct {
		name           string
		s              ptyd.Session
		fg             int
		idle, reapable bool
	}{
		{"quiet shell", ptyd.Session{Pid: 10, LastActivity: old}, 10, true, true},
		{"quiet program in the foreground", ptyd.Session{Pid: 10, LastActivity: old}, 42, true, false},
		{"foreground unknown", ptyd.Session{Pid: 10, LastActivity: old}, 0, true, false},
		{"someone attached", ptyd.Session{Pid: 10, LastActivity: old, Clients: 1}, 10, false, false},
		{"recent output", ptyd.Session{Pid: 10, LastActivity: now.Add(-time.Hour)}, 10, false, false},
	} {
		idle, reapable := idleState(tc.s, tc.fg, week, now)
		if idle != tc.idle || reapable != tc.reapable {
			t.Errorf("%s: idle=%v reapable=%v, want %v %v", tc.name, idle, reapable, tc.idle, tc.reapable)
		}
	}
}

// The foreground check the reaper relies on, against a real shell: the
// shell itself when it's at its prompt, something else while a program runs.
func TestForegroundPgrp(t *testing.T) {
	m := newPtydManager(t)
	if err := m.Ptyd.Create("fg", t.TempDir(), []string{"/bin/sh"}, []string{"PATH=/usr/bin:/bin"}, 80, 24); err != nil {
		t.Fatal(err)
	}
	s, err := m.Ptyd.Get("fg")
	if err != nil {
		t.Fatal(err)
	}
	if fg := foregroundPgrp(s.Pid); fg != s.Pid {
		t.Fatalf("idle shell: foreground pgrp %d, want the shell %d", fg, s.Pid)
	}
	if err := m.Ptyd.Write("fg", []byte("sleep 30\n")); err != nil {
		t.Fatal(err)
	}
	fg := foregroundPgrp(s.Pid)
	for i := 0; i < 50 && fg == s.Pid; i++ {
		time.Sleep(100 * time.Millisecond)
		fg = foregroundPgrp(s.Pid)
	}
	if fg == s.Pid || fg == 0 {
		t.Fatalf("running sleep: foreground pgrp %d, want a pgrp other than the shell's %d", fg, s.Pid)
	}
}
