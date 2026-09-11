package claudehook

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParsePayload(t *testing.T) {
	p, err := ParsePayload([]byte(`{"session_id":"abc-123","cwd":"/tmp/wt","hook_event_name":"SessionStart","transcript_path":"/tmp/x.jsonl"}`))
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	if p.SessionID != "abc-123" {
		t.Errorf("SessionID = %q, want abc-123", p.SessionID)
	}
	if p.Cwd != "/tmp/wt" {
		t.Errorf("Cwd = %q, want /tmp/wt", p.Cwd)
	}
}

func TestParsePayloadIgnoresUnknownFields(t *testing.T) {
	// Real payloads have more fields than SessionStartPayload decodes —
	// this must not fail just because Claude Code adds a field later.
	if _, err := ParsePayload([]byte(`{"session_id":"a","cwd":"/x","some_future_field":{"nested":true}}`)); err != nil {
		t.Fatalf("ParsePayload with an unrecognized field: %v", err)
	}
}

func TestParsePayloadInvalidJSON(t *testing.T) {
	if _, err := ParsePayload([]byte(`not json`)); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestIsBlockingNotification(t *testing.T) {
	cases := []struct {
		message string
		want    bool
	}{
		{"Claude needs your permission to use Bash", true},
		{"Claude is waiting for your input", true},
		{"Done — all tests pass", true},
		{"Claude is waiting for background agents to finish before continuing", false},
		{"Still waiting for background agent to respond", false},
		{"WAITING FOR BACKGROUND AGENT", false}, // case-insensitive
		{"waiting for background task to complete", false},
		{"", true}, // no message at all isn't a reason to skip on its own
	}
	for _, c := range cases {
		if got := IsBlockingNotification(c.message); got != c.want {
			t.Errorf("IsBlockingNotification(%q) = %v, want %v", c.message, got, c.want)
		}
	}
}

// withFakeHome points HOME at a temp dir for the duration of the test, so
// transcriptDir()'s os.UserHomeDir()-based resolution is testable without
// touching the real ~/.claude/projects.
func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeTranscript(t *testing.T, home, project, sessionID, content string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSessionTitleFindsFirstRealUserMessage(t *testing.T) {
	home := withFakeHome(t)
	lines := []string{
		`{"type":"mode","mode":"normal","sessionId":"s1"}`,
		`{"type":"user","isMeta":true,"message":{"role":"user","content":"<local-command-caveat>ignore this</local-command-caveat>"}}`,
		`{"type":"user","message":{"role":"user","content":"<system-injected-tag>also ignore</system-injected-tag>"}}`,
		`{"type":"user","message":{"role":"user","content":"fix the login bug please"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":"sure, looking into it"}}`,
	}
	writeTranscript(t, home, "-tmp-wt", "s1", joinLines(lines))

	title, err := SessionTitle("s1")
	if err != nil {
		t.Fatalf("SessionTitle: %v", err)
	}
	if title != "fix the login bug please" {
		t.Errorf("title = %q, want %q", title, "fix the login bug please")
	}
}

func TestSessionTitleClipsLongMessages(t *testing.T) {
	home := withFakeHome(t)
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	writeTranscript(t, home, "-tmp-wt", "s2", `{"type":"user","message":{"role":"user","content":"`+long+`"}}`)

	title, err := SessionTitle("s2")
	if err != nil {
		t.Fatalf("SessionTitle: %v", err)
	}
	if len([]rune(title)) != titleClipLength+1 { // +1 for the trailing ellipsis rune
		t.Errorf("clipped title length = %d, want %d", len([]rune(title)), titleClipLength+1)
	}
}

// TestSessionTitleSkipsArrayContentToolResults reproduces the real-world
// case that used to break title lookup entirely: a resumed/tool-heavy
// session whose early "user" lines are tool_result echoes (message.content
// is a JSON array, not a string) rather than anything actually typed. The
// old plain-`string` decode target failed to unmarshal these lines (a type
// mismatch), so json.Unmarshal returned an error and the `continue` above
// skipped them along with everything else — including any later, real
// typed message the array-content lines happened to precede in some
// transcripts.
func TestSessionTitleSkipsArrayContentToolResults(t *testing.T) {
	home := withFakeHome(t)
	lines := []string{
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"tool_reference","tool_name":"WebFetch"}]}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"some tool output","tool_use_id":"t2"}]}}`,
		`{"type":"user","message":{"role":"user","content":"actually fix the bug now"}}`,
	}
	writeTranscript(t, home, "-tmp-wt", "s3", joinLines(lines))

	title, err := SessionTitle("s3")
	if err != nil {
		t.Fatalf("SessionTitle: %v", err)
	}
	if title != "actually fix the bug now" {
		t.Errorf("title = %q, want %q", title, "actually fix the bug now")
	}
}

// TestSessionTitleExtractsTextBlockFromArrayContent covers a real typed
// message that also attaches something else (e.g. an image), which Claude
// Code represents as an array of content blocks including one of type
// "text" rather than a bare string.
func TestSessionTitleExtractsTextBlockFromArrayContent(t *testing.T) {
	home := withFakeHome(t)
	line := `{"type":"user","message":{"role":"user","content":[{"type":"image","source":{}},{"type":"text","text":"what does this screenshot show"}]}}`
	writeTranscript(t, home, "-tmp-wt", "s4", line)

	title, err := SessionTitle("s4")
	if err != nil {
		t.Fatalf("SessionTitle: %v", err)
	}
	if title != "what does this screenshot show" {
		t.Errorf("title = %q, want %q", title, "what does this screenshot show")
	}
}

func TestProjectDirName(t *testing.T) {
	// Verified against a real ~/.claude/projects/ directory name observed
	// on disk for this exact path shape (a worktree-studio worktree path,
	// which is where this function actually gets used).
	got := projectDirName("/Users/jayasurya/.worktree-studio/worktrees/52d305545bcee229/attendance-ui")
	want := "-Users-jayasurya--worktree-studio-worktrees-52d305545bcee229-attendance-ui"
	if got != want {
		t.Errorf("projectDirName = %q, want %q", got, want)
	}
}

func TestListSessionsForCwdNoProjectDir(t *testing.T) {
	withFakeHome(t)
	sessions, err := ListSessionsForCwd("/nowhere/claude/has/ever/seen")
	if err != nil {
		t.Fatalf("ListSessionsForCwd: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("sessions = %+v, want empty (not an error) for an unseen cwd", sessions)
	}
}

func TestListSessionsForCwdNewestFirstWithPreviewAndSize(t *testing.T) {
	home := withFakeHome(t)
	cwd := "/tmp/some-worktree"
	project := projectDirName(cwd)

	older := `{"type":"user","message":{"role":"user","content":"first session, older"}}` + "\n"
	newer := `{"type":"user","message":{"role":"user","content":"second session, newer"}}` + "\n"
	writeTranscript(t, home, project, "s-old", older)
	writeTranscript(t, home, project, "s-new", newer)

	oldPath := filepath.Join(home, ".claude", "projects", project, "s-old.jsonl")
	newPath := filepath.Join(home, ".claude", "projects", project, "s-new.jsonl")
	oldTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	sessions, err := ListSessionsForCwd(cwd)
	if err != nil {
		t.Fatalf("ListSessionsForCwd: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2: %+v", len(sessions), sessions)
	}
	if sessions[0].SessionID != "s-new" || sessions[1].SessionID != "s-old" {
		t.Errorf("order = [%s, %s], want newest first [s-new, s-old]", sessions[0].SessionID, sessions[1].SessionID)
	}
	if sessions[0].Preview != "second session, newer" {
		t.Errorf("sessions[0].Preview = %q, want %q", sessions[0].Preview, "second session, newer")
	}
	if sessions[0].SizeBytes != int64(len(newer)) {
		t.Errorf("sessions[0].SizeBytes = %d, want %d", sessions[0].SizeBytes, len(newer))
	}
	if sessions[0].UpdatedAt == "" {
		t.Error("sessions[0].UpdatedAt should not be empty")
	}
}

func TestSessionTitleNotFound(t *testing.T) {
	withFakeHome(t)
	if _, err := SessionTitle("does-not-exist"); err != ErrTranscriptNotFound {
		t.Errorf("err = %v, want ErrTranscriptNotFound", err)
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}
