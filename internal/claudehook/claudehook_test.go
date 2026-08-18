package claudehook

import (
	"os"
	"path/filepath"
	"testing"
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
