// Package claudehook handles the Claude Code SessionStart hook payload:
// resolving which worktree (if any) a session belongs to by its reported
// cwd, and reading a session's own transcript for a human-readable title.
//
// This exists to fix two real problems with the earlier approach (a
// client-generated --session-id passed to `claude` at launch, logged by
// worktree-studio itself the moment it decides to start that terminal):
// (1) it only ever saw sessions worktree-studio itself launched, missing
// every session a person starts by hand in a plain shell; (2) the
// auto-launched terminal doesn't always actually get created (an observed,
// not-yet-root-caused race — see PLAN.md), so even the "own terminal" case
// isn't reliable. A real Claude Code hook fires whenever a session starts,
// regardless of how — see PLAN.md's "Claude Code hooks" section for the
// full design and internal/api/hooks.go for the HTTP side.
package claudehook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SessionStartPayload is the subset of Claude Code's SessionStart hook
// stdin JSON this package actually uses. The real payload has more fields
// (transcript_path, source, etc.) — only decoding what's needed keeps this
// resilient to Claude Code adding fields later (encoding/json ignores
// unrecognized keys by default).
type SessionStartPayload struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

// ParsePayload decodes a hook's raw stdin JSON.
func ParsePayload(raw []byte) (SessionStartPayload, error) {
	var p SessionStartPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return SessionStartPayload{}, fmt.Errorf("decode hook payload: %w", err)
	}
	return p, nil
}

// transcriptDir returns ~/.claude/projects, where Claude Code stores one
// subdirectory per project (named after the project's absolute path with
// '/' replaced by '-') containing one JSONL transcript file per session,
// named <session-id>.jsonl.
func transcriptDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// ErrTranscriptNotFound means no transcript file exists for the given
// session id — a completely normal outcome (the session may have been
// deleted, or Claude Code's storage layout may have changed), not a bug.
var ErrTranscriptNotFound = fmt.Errorf("claude session transcript not found")

const titleClipLength = 140

// SessionTitle finds sessionID's transcript under ~/.claude/projects/*/
// (globbed rather than computed from a known project path, since by the
// time this is called the worktree that started the session may itself
// have been archived or deleted — the transcript's location is looked up
// independent of any current worktree-studio state) and returns a clipped
// version of the session's first real user message as a human-readable
// title.
//
// "First real user message" is a heuristic, not exact: transcripts start
// with several non-conversational lines (mode/permission-mode markers,
// hook-attachment records) and the actual first `type:"user"` entry is
// often itself an injected wrapper (e.g. a `<local-command-caveat>` tag)
// rather than anything the person typed — this skips any user-role
// message whose content starts with '<', since real typed prompts don't.
// Good enough for a display label; not a substitute for reading the real
// transcript.
func SessionTitle(sessionID string) (string, error) {
	dir, err := transcriptDir()
	if err != nil {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*", sessionID+".jsonl"))
	if err != nil {
		return "", fmt.Errorf("glob transcripts: %w", err)
	}
	if len(matches) == 0 {
		return "", ErrTranscriptNotFound
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		return "", fmt.Errorf("read transcript %s: %w", matches[0], err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry struct {
			Type    string `json:"type"`
			IsMeta  bool   `json:"isMeta"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // tolerate any line this loose struct can't decode
		}
		if entry.Type != "user" || entry.IsMeta || entry.Message.Role != "user" {
			continue
		}
		content := strings.TrimSpace(entry.Message.Content)
		if content == "" || strings.HasPrefix(content, "<") {
			continue
		}
		return clip(content, titleClipLength), nil
	}
	return "", ErrTranscriptNotFound
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
