package claudehook

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// This file owns the one genuinely sensitive operation in this package:
// editing the user's REAL, GLOBAL `~/.claude/settings.json` to add or
// remove SessionStart hooks. That file is shared with every other tool
// that registers a Claude Code hook (see the PreToolUse/UserPromptSubmit/
// Stop/etc. entries already present on a typical machine) — this code
// must never overwrite the file wholesale, only ever merge clearly-marked
// entries into its `hooks.SessionStart` array, and always back the file up
// first. Every entry-point here is only ever reached via an explicit user
// action in the settings UI (see internal/api/settings.go) — never
// automatically on server startup.
//
// Hooks are registered in hookRegistry below, one entry per independently
// installable script — each gets its own generated file, its own
// SessionStart entry, and its own install/uninstall/status, so it can be
// reasoned about, tested, and toggled without touching any other hook.
// internal/api/settings.go derives its whole hooks list (and the settings
// UI its whole "Claude Code hooks" section) from Hooks() — adding a new
// hook here is the only change needed for it to show up there too.

const hookEventName = "SessionStart"

// hookSpec is one registered SessionStart hook: metadata for display, the
// script file it owns, and the function that generates that script's
// content. content takes serverBaseURL even for hooks that ignore it, so
// every spec has the same shape regardless of whether it happens to talk
// to the worktree-studio server.
type hookSpec struct {
	id       string
	name     string
	hint     string // shown in the settings UI when not installed
	fileName string
	content  func(serverBaseURL string) string
}

var hookRegistry = []hookSpec{
	{
		id:       "session-tracking",
		name:     "Claude session-tracking hook",
		hint:     "install to track claude sessions started by hand, not just ones worktree-studio auto-starts",
		fileName: "session-start.sh",
		content:  hookScriptContent,
	},
	{
		id:       "session-context",
		name:     "Claude worktree-context hook",
		hint:     "install to give Claude basic orientation (folder, branch, open PRs) when it opens inside a worktree-studio worktree",
		fileName: "session-context.sh",
		content:  contextScriptContent,
	},
}

// ErrUnknownHook is returned by the ByID functions below for an id not
// present in hookRegistry — should only ever happen from a stale client
// (an old settings UI tab open across a downgrade, a hand-crafted request).
var ErrUnknownHook = errors.New("unknown hook id")

func findSpec(id string) (hookSpec, error) {
	for _, spec := range hookRegistry {
		if spec.id == id {
			return spec, nil
		}
	}
	return hookSpec{}, fmt.Errorf("%w: %q", ErrUnknownHook, id)
}

// HookInfo is the public, read-only view of a registered hook — everything
// the settings UI needs to render a row, without exposing the script
// generator itself.
type HookInfo struct {
	ID   string
	Name string
	Hint string
}

// Hooks returns metadata for every registered SessionStart hook, in
// registry order. The settings UI renders one row per entry returned here.
func Hooks() []HookInfo {
	infos := make([]HookInfo, len(hookRegistry))
	for i, spec := range hookRegistry {
		infos[i] = HookInfo{ID: spec.id, Name: spec.name, Hint: spec.hint}
	}
	return infos
}

// ClaudeSettingsPath returns ~/.claude/settings.json — Claude Code's
// user-level (not project-level) settings file, which is where a hook
// needs to live to fire for every session on the machine regardless of
// which worktree/project it's started in.
func ClaudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// scriptPath returns where a hook's generated script file lives. Its
// uniqueness (nothing else on the machine writes here) is what makes it
// safe to use as the marker identifying "our" entry in settings.json's
// hooks.SessionStart array, for both idempotent install and precise
// uninstall.
func scriptPath(fileName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".worktree-studio", "hooks", fileName), nil
}

func hookScriptContent(serverBaseURL string) string {
	// Deliberately fire-and-forget: a short curl timeout, errors and
	// non-2xx responses silently discarded, and the script always exits 0
	// — a claude session must never fail or hang at startup because
	// worktree-studio isn't running or is unreachable. cat pipes stdin
	// (the hook's JSON payload) straight through as the request body; this
	// server's own handler (handleClaudeHook) does the actual parsing.
	return fmt.Sprintf(`#!/bin/sh
# Installed by worktree-studio (internal/claudehook) — safe to delete if
# you uninstall the SessionStart hook from Claude Code's settings.
curl -s -m 2 -X POST -H 'Content-Type: application/json' -d @- '%s/api/claude-hook' >/dev/null 2>&1 || true
exit 0
`, serverBaseURL)
}

// contextScriptContent gives Claude some basic orientation at session
// start: the current directory always, and, when that directory is inside
// a worktree-studio-managed worktree (~/.worktree-studio/...), the current
// branch and any open GitHub PRs for it. For SessionStart hooks
// specifically, Claude Code adds whatever a hook prints to stdout straight
// into the session's context as plain text — no JSON envelope needed —
// and runs the hook with cwd already set to the session's own directory,
// so none of this needs the hook's stdin payload at all. The stdout
// printf is unconditional and doesn't wait on anything below it.
//
// The `gh pr list` call is the one part of this that can be slow (a real
// network request) or hang (a wedged connection) — bounded with a manual
// background-and-kill timeout since macOS ships no `timeout`/`gtimeout`.
// That's best-effort, not an airtight guarantee (some /bin/sh
// implementations may not map the backgrounded subshell's PID onto `gh`
// itself), consistent with this package's existing fire-and-forget stance
// on the audit POST in hookScriptContent. Missing `gh`, no auth, no
// matching PR, or no network are all silently treated the same way:
// nothing to report.
//
// After printing, it also best-effort POSTs the exact text just printed
// to serverBaseURL/api/claude-hook-context, so that text shows up in this
// worktree's audit log (see internal/api/hooks.go's
// handleClaudeHookContext) — direct follow-up to "can I get logs into
// what context is injected... in the worktree logs?" (2026-08-15). That
// POST is gated on `jq` being available: hand-building JSON for arbitrary
// multi-line text (a PR title can contain quotes or backslashes) in plain
// shell is exactly the fragile string-escaping this project avoids
// elsewhere, so this skips logging entirely rather than risk emitting
// malformed JSON — the stdout context Claude already has is completely
// unaffected either way.
func contextScriptContent(serverBaseURL string) string {
	return fmt.Sprintf(`#!/bin/sh
# Installed by worktree-studio (internal/claudehook) — safe to delete if
# you uninstall the SessionStart hook from Claude Code's settings.
cwd="$(pwd)"
context="Ooga. Claude wake up in cave (folder): $cwd"

case "$cwd" in
  "$HOME"/.worktree-studio/*)
    branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null)"
    if [ -n "$branch" ] && [ "$branch" != "HEAD" ]; then
      context="$context
Ooo, worktree-studio cave! Branch-mark say: $branch"
      if command -v gh >/dev/null 2>&1; then
        tmp="$(mktemp 2>/dev/null)"
        if [ -n "$tmp" ]; then
          ( gh pr list --head "$branch" --json number,title,url --jq '.[] | "PR #\(.number): \(.title) (\(.url))"' >"$tmp" 2>/dev/null ) &
          gh_pid=$!
          ( sleep 3; kill "$gh_pid" 2>/dev/null ) >/dev/null 2>&1 &
          watcher_pid=$!
          wait "$gh_pid" 2>/dev/null
          kill "$watcher_pid" 2>/dev/null
          wait "$watcher_pid" 2>/dev/null
          prs="$(cat "$tmp" 2>/dev/null)"
          rm -f "$tmp" 2>/dev/null
          if [ -n "$prs" ]; then
            context="$context
Grug look sky-scroll (PR)... Grug find:
$prs"
          else
            context="$context
Grug look sky-scroll (PR)... find nothing. Sad Grug."
          fi
        fi
      fi
    fi
    ;;
esac

printf '%%s\n' "$context"

if command -v jq >/dev/null 2>&1; then
  jq -n --arg cwd "$cwd" --arg context "$context" '{cwd: $cwd, context: $context}' 2>/dev/null | curl -s -m 2 -X POST -H 'Content-Type: application/json' -d @- '%s/api/claude-hook-context' >/dev/null 2>&1 || true
fi

exit 0
`, serverBaseURL)
}

// IsHookInstalledByID reports whether the named hook's SessionStart entry
// is already present in ~/.claude/settings.json. A missing settings file,
// or one with no `hooks` or no `hooks.SessionStart` key, is a normal "not
// installed" result, not an error.
func IsHookInstalledByID(id string) (bool, error) {
	spec, err := findSpec(id)
	if err != nil {
		return false, err
	}
	path, err := scriptPath(spec.fileName)
	if err != nil {
		return false, err
	}
	settings, _, err := readSettings()
	if err != nil {
		return false, err
	}
	return findOurEntry(settings, path) != -1, nil
}

// InstallHookByID writes the named hook's script (serverBaseURL, e.g.
// "http://localhost:8787", is passed to every hook's content generator but
// only ones that need it use it) and merges its SessionStart entry into
// ~/.claude/settings.json. Idempotent: calling it again when already
// installed does not duplicate the entry, though it does refresh the
// script's content unconditionally (e.g. if the server's address changed,
// or this package shipped a new script version, since a previous install)
// — cheap, and re-installing is the natural "fix it" action a user takes
// from the settings UI anyway. Backs up settings.json before any write.
func InstallHookByID(id, serverBaseURL string) error {
	spec, err := findSpec(id)
	if err != nil {
		return err
	}
	path, err := scriptPath(spec.fileName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create hook script dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(spec.content(serverBaseURL)), 0o755); err != nil {
		return fmt.Errorf("write hook script: %w", err)
	}

	settings, mode, err := readSettings()
	if err != nil {
		return err
	}
	if !addSessionStartEntry(settings, path) {
		return nil // already installed, script content refreshed above
	}
	return writeSettings(settings, mode)
}

// UninstallHookByID removes the named hook's entry from
// ~/.claude/settings.json's hooks.SessionStart array, leaving every other
// hook (ours or anyone else's) untouched. A no-op, not an error, if it was
// never installed. Does not remove the script file itself — harmless to
// leave behind, and simpler than reasoning about whether anything else
// might reference it.
func UninstallHookByID(id string) error {
	spec, err := findSpec(id)
	if err != nil {
		return err
	}
	path, err := scriptPath(spec.fileName)
	if err != nil {
		return err
	}
	settings, mode, err := readSettings()
	if err != nil {
		return err
	}
	if !removeSessionStartEntry(settings, path) {
		return nil
	}
	return writeSettings(settings, mode)
}

// addSessionStartEntry merges a single-command SessionStart entry for
// scriptPath into settings["hooks"], creating the "hooks" and
// "SessionStart" keys as needed. Reports whether it actually added
// anything (false if scriptPath's entry was already present).
func addSessionStartEntry(settings map[string]any, scriptPath string) bool {
	if findOurEntry(settings, scriptPath) != -1 {
		return false
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	sessionStart, _ := hooks[hookEventName].([]any)
	sessionStart = append(sessionStart, map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{"type": "command", "command": scriptPath},
		},
	})
	hooks[hookEventName] = sessionStart
	settings["hooks"] = hooks
	return true
}

// removeSessionStartEntry removes scriptPath's SessionStart entry from
// settings["hooks"] if present, cleaning up the now-empty "SessionStart"
// key if it was the last entry. Reports whether it actually removed
// anything.
func removeSessionStartEntry(settings map[string]any, scriptPath string) bool {
	idx := findOurEntry(settings, scriptPath)
	if idx == -1 {
		return false
	}
	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks[hookEventName].([]any)
	sessionStart = append(sessionStart[:idx], sessionStart[idx+1:]...)
	if len(sessionStart) == 0 {
		delete(hooks, hookEventName)
	} else {
		hooks[hookEventName] = sessionStart
	}
	settings["hooks"] = hooks
	return true
}

// readSettings loads ~/.claude/settings.json as a generic map, preserving
// every key this package doesn't know or care about (env, permissions,
// model, enabledPlugins, other tools' hooks, ...) so a write-back never
// drops anything. A missing file reads as an empty settings object, not
// an error — Claude Code itself treats no file as "all defaults."
func readSettings() (map[string]any, os.FileMode, error) {
	path, err := ClaudeSettingsPath()
	if err != nil {
		return nil, 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, 0o600, nil
		}
		return nil, 0, fmt.Errorf("read %s: %w", path, err)
	}
	info, err := os.Stat(path)
	mode := os.FileMode(0o600)
	if err == nil {
		mode = info.Mode()
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, 0, fmt.Errorf("parse %s: %w (refusing to touch a file that isn't valid JSON)", path, err)
	}
	return settings, mode, nil
}

// writeSettings backs up the current file (if it exists) to
// ~/.worktree-studio/backups/ before overwriting it — the one piece of
// the broader backup/restore TODO (see PLAN.md) implemented so far,
// scoped narrowly to this one sensitive write.
func writeSettings(settings map[string]any, mode os.FileMode) error {
	path, err := ClaudeSettingsPath()
	if err != nil {
		return err
	}
	if err := backupIfExists(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, data, mode)
}

func backupIfExists(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s for backup: %w", path, err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	backupDir := filepath.Join(home, ".worktree-studio", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	backupPath := filepath.Join(backupDir, fmt.Sprintf("claude-settings-%d.json", time.Now().UnixNano()))
	return os.WriteFile(backupPath, data, 0o600)
}

// findOurEntry returns the index of our hook entry within
// settings["hooks"]["SessionStart"] (a []any of matcher-groups), or -1 if
// not present. Matches on scriptPath appearing as any hook's "command" —
// exact match, not a substring check, so nothing else could accidentally
// collide with it.
func findOurEntry(settings map[string]any, scriptPath string) int {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return -1
	}
	sessionStart, ok := hooks[hookEventName].([]any)
	if !ok {
		return -1
	}
	for i, group := range sessionStart {
		groupMap, ok := group.(map[string]any)
		if !ok {
			continue
		}
		entries, ok := groupMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, e := range entries {
			entryMap, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := entryMap["command"].(string); cmd == scriptPath {
				return i
			}
		}
	}
	return -1
}
