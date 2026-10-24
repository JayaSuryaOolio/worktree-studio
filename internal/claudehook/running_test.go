package claudehook

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRegistry(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListRunningFiltersStaleBadAndOtherCwds(t *testing.T) {
	dir := t.TempDir()
	// Live, in the worktree, with an unknown field that must be ignored.
	writeRegistry(t, dir, "100.json", `{"pid":100,"sessionId":"aaaaaaaa-1","cwd":"/wt","kind":"interactive","status":"busy","name":"one","tmux":"wts-t1:@0.%0","futureField":{"x":1}}`)
	// Live, in a subdirectory of the worktree.
	writeRegistry(t, dir, "101.json", `{"pid":101,"sessionId":"bbbbbbbb-2","cwd":"/wt/sub","kind":"bg","jobId":"bbbbbbbb"}`)
	// Stale: the pid is dead.
	writeRegistry(t, dir, "102.json", `{"pid":102,"sessionId":"cccccccc-3","cwd":"/wt"}`)
	// A sibling path that merely shares a prefix.
	writeRegistry(t, dir, "103.json", `{"pid":103,"sessionId":"dddddddd-4","cwd":"/wt-other"}`)
	// Garbage and missing fields.
	writeRegistry(t, dir, "104.json", `{not json`)
	writeRegistry(t, dir, "105.json", `{"pid":105,"cwd":"/wt"}`)
	writeRegistry(t, dir, "106.key", `ignored`)

	alive := func(pid int) bool { return pid != 102 }
	got := ListRunning(dir, "/wt", alive)
	ids := map[string]RunningSession{}
	for _, r := range got {
		ids[r.SessionID] = r
	}
	if len(got) != 2 || ids["aaaaaaaa-1"].PID != 100 || ids["bbbbbbbb-2"].PID != 101 {
		t.Fatalf("ListRunning = %+v, want sessions 1 and 2 only", got)
	}
	if r := ids["aaaaaaaa-1"]; r.Status != "busy" || r.Name != "one" || r.Kind != "interactive" {
		t.Errorf("fields not parsed: %+v", r)
	}
}

func TestListRunningMissingDir(t *testing.T) {
	if got := ListRunning(filepath.Join(t.TempDir(), "nope"), "/wt", PidAlive); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

func TestPidAlive(t *testing.T) {
	if !PidAlive(os.Getpid()) {
		t.Error("own pid not alive")
	}
	if PidAlive(0) || PidAlive(-1) {
		t.Error("non-positive pid reported alive")
	}
}

func TestShortIDAndTmuxSessionName(t *testing.T) {
	if got := (RunningSession{SessionID: "4cf9a067-93fb", JobID: "4cf9a067"}).ShortID(); got != "4cf9a067" {
		t.Errorf("ShortID with jobId = %q", got)
	}
	if got := (RunningSession{SessionID: "4cf9a067-93fb"}).ShortID(); got != "4cf9a067" {
		t.Errorf("ShortID fallback = %q", got)
	}
	if got := TmuxSessionName("wts-a33d98b72ff364d9:@0.%0"); got != "wts-a33d98b72ff364d9" {
		t.Errorf("TmuxSessionName = %q", got)
	}
}

func TestAncestorInWalksToPtydShell(t *testing.T) {
	// launchd(1) -> ptyd(10) -> shell(20) -> node wrapper(30) -> claude(40)
	parents := map[int]int{40: 30, 30: 20, 20: 10, 10: 1, 50: 1}
	owners := map[int]string{20: "term-a"}
	if got := AncestorIn(40, owners, parents); got != "term-a" {
		t.Errorf("AncestorIn(40) = %q, want term-a", got)
	}
	if got := AncestorIn(50, owners, parents); got != "" {
		t.Errorf("AncestorIn(50) = %q, want none", got)
	}
	if got := AncestorIn(99, owners, map[int]int{99: 98, 98: 99}); got != "" { // cycle
		t.Errorf("AncestorIn(cycle) = %q, want none", got)
	}
}

func TestMergeRunning(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // SessionTitle looks under ~/.claude
	past := []SessionSummary{
		{SessionID: "new-past", UpdatedAt: "2026-10-24T10:00:00Z"},
		{SessionID: "live", UpdatedAt: "2026-10-01T10:00:00Z"},
	}
	running := []RunningSession{
		{PID: 1, SessionID: "live", Kind: "interactive", Status: "idle", Tmux: "wts-t1:@0.%0"},
		{PID: 2, SessionID: "no-transcript", Kind: "bg", JobID: "no-trans", UpdatedAt: 1792842013356},
	}
	got := MergeRunning(past, running, func(r RunningSession) string {
		if r.Tmux != "" {
			return "t1"
		}
		return ""
	})
	if len(got) != 3 {
		t.Fatalf("got %d sessions, want 3: %+v", len(got), got)
	}
	if !got[0].Running || !got[1].Running || got[2].Running || got[2].SessionID != "new-past" {
		t.Fatalf("running not sorted first: %+v", got)
	}
	for _, s := range got[:2] {
		switch s.SessionID {
		case "live":
			if s.TerminalID != "t1" || s.Status != "idle" || s.ShortID != "live" {
				t.Errorf("live = %+v", s)
			}
		case "no-transcript":
			if s.Kind != "bg" || s.ShortID != "no-trans" || s.TerminalID != "" || s.UpdatedAt == "" {
				t.Errorf("no-transcript = %+v", s)
			}
		default:
			t.Errorf("unexpected %+v", s)
		}
	}
}
