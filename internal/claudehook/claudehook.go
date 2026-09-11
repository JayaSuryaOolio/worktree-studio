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
	"sort"
	"strings"
	"time"
)

// HookPayload is the subset of Claude Code hook stdin JSON this package
// actually uses, across both hook events it's installed for (SessionStart
// and Notification — see install.go). The real payload has more fields
// (transcript_path, source, etc.) — only decoding what's needed keeps this
// resilient to Claude Code adding fields later (encoding/json ignores
// unrecognized keys by default).
type HookPayload struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	// HookEventName distinguishes which of the two installed hooks fired
	// ("SessionStart" or "Notification") — both post to the same
	// /api/claude-hook endpoint via the same installed script (see
	// hookScriptContent), so the server needs this to know how to react.
	HookEventName string `json:"hook_event_name"`
	// Message is only populated on a Notification event — Claude Code's
	// own human-readable text, e.g. "Claude needs your permission to use
	// Bash" or "Claude is waiting for your input".
	Message string `json:"message"`
}

// ParsePayload decodes a hook's raw stdin JSON.
func ParsePayload(raw []byte) (HookPayload, error) {
	var p HookPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return HookPayload{}, fmt.Errorf("decode hook payload: %w", err)
	}
	return p, nil
}

// nonBlockingMessagePhrases are substrings (case-insensitive) that mean a
// Notification hook fired to report ongoing background progress — e.g.
// still waiting on background agents/subagents to finish — rather than
// something that actually needs the user right now. Message is
// unstructured free text with no guaranteed wording (see HookPayload's
// own comment and PLAN.md), so this is a best-effort exclusion list, not
// exhaustive: extend it as new non-blocking phrasings are observed.
var nonBlockingMessagePhrases = []string{
	"waiting for background agent",
	"waiting for background task",
}

// IsBlockingNotification reports whether a Notification hook's message
// represents something that should actually badge the worktree and fire a
// desktop notification — a permission prompt, idle-waiting-for-input, or
// a finished result — as opposed to a transient "still working in the
// background" status update.
func IsBlockingNotification(message string) bool {
	lower := strings.ToLower(message)
	for _, phrase := range nonBlockingMessagePhrases {
		if strings.Contains(lower, phrase) {
			return false
		}
	}
	return true
}

// ContextPayload is what the session-context hook script (see
// contextScriptContent) POSTs, best-effort, to /api/claude-hook-context:
// the cwd it resolved and the exact text it printed to stdout for Claude —
// letting the audit log show what a session was actually told, not just
// that a hook fired. Unlike HookPayload this isn't Claude Code's own hook
// JSON — it's a shape this package's own script constructs.
type ContextPayload struct {
	Cwd     string `json:"cwd"`
	Context string `json:"context"`
}

// ParseContextPayload decodes the session-context hook script's POST body.
func ParseContextPayload(raw []byte) (ContextPayload, error) {
	var p ContextPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return ContextPayload{}, fmt.Errorf("decode context payload: %w", err)
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
//
// message.content isn't always a plain string: Claude Code's own transcript
// format uses a string only for a simple typed message, but an array of
// content blocks (tool_result echoes, or a real message that also attaches
// an image) for most other user-role turns — see extractText. Decoding
// straight into a Go `string` field made every array-content line fail to
// unmarshal and get skipped by the `continue` below, which is why some
// sessions' titles resolved fine (their first real turn happened to be
// plain text) while others (first real turn included an attachment, or the
// transcript's early lines were tool_result echoes from a resumed session)
// silently found nothing and fell back to a bare session id.
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
	return titleFromTranscriptFile(matches[0])
}

// titleFromTranscriptFile is SessionTitle's actual extraction logic,
// factored out so ListSessionsForCwd (which already knows each transcript
// file's path from reading its project directory directly) doesn't need to
// re-glob just to reuse it.
func titleFromTranscriptFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read transcript %s: %w", path, err)
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
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // tolerate any line this loose struct can't decode
		}
		if entry.Type != "user" || entry.IsMeta || entry.Message.Role != "user" {
			continue
		}
		content := strings.TrimSpace(extractText(entry.Message.Content))
		if content == "" || strings.HasPrefix(content, "<") {
			continue
		}
		return clip(content, titleClipLength), nil
	}
	return "", ErrTranscriptNotFound
}

// extractText pulls a human-typed message's text out of a transcript
// message.content field, which is either a plain JSON string (the simple
// case) or an array of content blocks (e.g. `[{"type":"text","text":"..."}]`
// for a message that also attached an image, or `[{"type":"tool_result",
// ...}]` for a synthetic turn nothing was actually typed into). Returns ""
// for anything it can't find real typed text in — the caller treats that
// the same as skipping the line entirely, so a tool_result block never
// gets mistaken for a title.
func extractText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			return b.Text
		}
	}
	return ""
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// projectDirName computes Claude Code's own ~/.claude/projects/<name>
// directory name for an absolute path: every character outside
// [A-Za-z0-9] becomes '-' (verified empirically against real project
// directories — e.g. "/Users/x/.worktree-studio/worktrees/abc/feature"
// becomes "-Users-x--worktree-studio-worktrees-abc-feature", the doubled
// dash from "/." falling out naturally). Undocumented, so this could in
// principle drift if Claude Code changes its own scheme — SessionTitle
// above deliberately doesn't depend on it (it globs across every project
// dir instead), but ListSessionsForCwd needs the reverse direction (path
// -> project dir) to list a worktree's sessions without scanning a
// user's entire ~/.claude/projects history.
func projectDirName(absPath string) string {
	var b strings.Builder
	b.Grow(len(absPath))
	for _, r := range absPath {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// SessionSummary is one past claude session found for a worktree's own
// path — see ListSessionsForCwd.
type SessionSummary struct {
	SessionID string `json:"session_id"`
	// Preview is the same "first real user message" heuristic as
	// SessionTitle, clipped the same way — "" if none was found (an
	// empty/meta-only transcript, or one that hasn't said anything real
	// yet).
	Preview string `json:"preview"`
	// UpdatedAt is the transcript file's own last-modified time (RFC3339)
	// — the best available proxy for "last activity in this session"
	// without parsing every line's own timestamp field.
	UpdatedAt string `json:"updated_at"`
	// SizeBytes is the transcript file's size on disk, letting the UI show
	// e.g. "19 MB" the way the user would recognize from Finder/`ls -lh`,
	// not a token or message count this package doesn't track.
	SizeBytes int64 `json:"size_bytes"`
}

// ListSessionsForCwd returns every past claude session recorded for cwd's
// own project directory under ~/.claude/projects/, newest (by transcript
// mtime) first. A cwd Claude Code has never seen a session start in (no
// matching project directory at all) returns an empty slice, not an error
// — the normal case for a freshly created worktree.
func ListSessionsForCwd(cwd string) ([]SessionSummary, error) {
	dir, err := transcriptDir()
	if err != nil {
		return nil, err
	}
	projectDir := filepath.Join(dir, projectDirName(cwd))
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionSummary{}, nil
		}
		return nil, fmt.Errorf("read project dir %s: %w", projectDir, err)
	}

	sessions := make([]SessionSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue // vanished between ReadDir and Info — skip, not fatal
		}
		path := filepath.Join(projectDir, entry.Name())
		// A missing/unresolvable preview (e.g. an empty transcript) still
		// gets listed — an empty Preview is a fine, honest answer for the
		// UI to fall back on (e.g. just the timestamp/size), unlike
		// SessionTitle's own 404-on-no-title posture for a single lookup.
		preview, _ := titleFromTranscriptFile(path)
		sessions = append(sessions, SessionSummary{
			SessionID: strings.TrimSuffix(entry.Name(), ".jsonl"),
			Preview:   preview,
			UpdatedAt: info.ModTime().UTC().Format(time.RFC3339),
			SizeBytes: info.Size(),
		})
	}

	sort.Slice(sessions, func(i, j int) bool { return sessions[i].UpdatedAt > sessions[j].UpdatedAt })
	return sessions, nil
}
