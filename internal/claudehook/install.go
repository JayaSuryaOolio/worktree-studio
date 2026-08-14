package claudehook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// This file owns the one genuinely sensitive operation in this package:
// editing the user's REAL, GLOBAL `~/.claude/settings.json` to add or
// remove our SessionStart and Notification hooks. That file is shared with
// every other tool that registers a Claude Code hook (see the
// PreToolUse/UserPromptSubmit/Stop/etc. entries already present on a
// typical machine) — this code must never overwrite the file wholesale,
// only ever merge one clearly-marked entry into each of hookEventNames'
// arrays, and always back the file up first. Every entry-point here is
// only ever reached via an explicit user action in the settings UI (see
// internal/api/settings.go) — never automatically on server startup.

// hookEventNames are the Claude Code hook events this package installs the
// same script under: SessionStart (claude.session.create logging) and
// Notification (fires when Claude is waiting on a permission prompt or
// user input — see internal/attention). One script, one endpoint
// (/api/claude-hook) — the posted payload's own hook_event_name field is
// what the server uses to tell them apart, so both installs/uninstalls
// always happen together rather than needing two separate dependencies in
// the settings UI.
var hookEventNames = []string{"SessionStart", "Notification"}

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

// HookScriptPath returns the path this package writes its own hook script
// to — shared by both SessionStart and Notification (see hookEventNames),
// since the script itself is event-agnostic (just forwards stdin). Its
// uniqueness (nothing else on the machine writes here) is what makes it
// safe to use as the marker identifying "our" entry in each of
// settings.json's hooks.<event> arrays, for both idempotent install and
// precise uninstall.
func HookScriptPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".worktree-studio", "hooks", "session-start.sh"), nil
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
# you uninstall the SessionStart/Notification hooks from Claude Code's
# settings. Shared by both events; the JSON payload's own hook_event_name
# field is what tells the server which one fired.
curl -s -m 2 -X POST -H 'Content-Type: application/json' -d @- '%s/api/claude-hook' >/dev/null 2>&1 || true
exit 0
`, serverBaseURL)
}

// IsHookInstalled reports whether our entry is present under every event
// in hookEventNames. A missing settings file, or one with no `hooks` key
// at all, is a normal "not installed" result, not an error. Requiring ALL
// of them (not just one) means a partially-applied install — e.g. someone
// hand-edited settings.json and only one entry survived — correctly shows
// as "not installed" rather than silently missing the Notification half.
func IsHookInstalled() (bool, error) {
	scriptPath, err := HookScriptPath()
	if err != nil {
		return false, err
	}
	settings, _, err := readSettings()
	if err != nil {
		return false, err
	}
	for _, event := range hookEventNames {
		if findOurEntry(settings, event, scriptPath) == -1 {
			return false, nil
		}
	}
	return true, nil
}

// InstallHook writes the hook script (pointed at serverBaseURL, e.g.
// "http://localhost:8787") and merges an entry referencing it into
// ~/.claude/settings.json under every event in hookEventNames. Idempotent
// per event: an event that already has our entry is left alone; a missing
// one is added — so a partial install (see IsHookInstalled) self-heals on
// the next InstallHook call instead of needing an uninstall first. Backs
// up settings.json once before any writes.
func InstallHook(serverBaseURL string) error {
	scriptPath, err := HookScriptPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		return fmt.Errorf("create hook script dir: %w", err)
	}
	// Always refresh the script content (e.g. if the server's address
	// changed since a previous install) — cheap, and re-installing is the
	// natural "fix it" action a user takes from the settings UI anyway.
	if err := os.WriteFile(scriptPath, []byte(hookScriptContent(serverBaseURL)), 0o755); err != nil {
		return fmt.Errorf("write hook script: %w", err)
	}

	settings, mode, err := readSettings()
	if err != nil {
		return err
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	changed := false
	for _, event := range hookEventNames {
		if findOurEntry(settings, event, scriptPath) != -1 {
			continue // already installed for this event
		}
		group, _ := hooks[event].([]any)
		group = append(group, map[string]any{
			"matcher": "",
			"hooks": []any{
				map[string]any{"type": "command", "command": scriptPath},
			},
		})
		hooks[event] = group
		changed = true
	}
	if !changed {
		return nil
	}
	settings["hooks"] = hooks

	return writeSettings(settings, mode)
}

// UninstallHook removes our entry from every event in hookEventNames,
// leaving every other hook (ours or anyone else's) untouched. A no-op, not
// an error, if it was never installed. Does not remove the script file
// itself — harmless to leave behind, and simpler than reasoning about
// whether anything else might reference it.
func UninstallHook() error {
	scriptPath, err := HookScriptPath()
	if err != nil {
		return err
	}
	settings, mode, err := readSettings()
	if err != nil {
		return err
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}

	changed := false
	for _, event := range hookEventNames {
		idx := findOurEntry(settings, event, scriptPath)
		if idx == -1 {
			continue
		}
		group := hooks[event].([]any)
		group = append(group[:idx], group[idx+1:]...)
		if len(group) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = group
		}
		changed = true
	}
	if !changed {
		return nil
	}
	settings["hooks"] = hooks

	return writeSettings(settings, mode)
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
// settings["hooks"][event] (a []any of matcher-groups), or -1 if not
// present. Matches on scriptPath appearing as any hook's "command" — exact
// match, not a substring check, so nothing else could accidentally collide
// with it.
func findOurEntry(settings map[string]any, event, scriptPath string) int {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return -1
	}
	entryGroups, ok := hooks[event].([]any)
	if !ok {
		return -1
	}
	for i, entryGroup := range entryGroups {
		groupMap, ok := entryGroup.(map[string]any)
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
