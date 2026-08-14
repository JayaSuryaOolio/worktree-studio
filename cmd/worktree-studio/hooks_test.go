package main

import (
	"os"
	"path/filepath"
	"testing"

	"worktree-studio/internal/claudehook"
)

func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestInstallHooksInstallsClaudeHookAndSkill(t *testing.T) {
	home := withFakeHome(t)

	if code := installHooks(); code != 0 {
		t.Fatalf("installHooks() = %d, want 0", code)
	}

	installed, err := claudehook.IsHookInstalled()
	if err != nil {
		t.Fatalf("IsHookInstalled: %v", err)
	}
	if !installed {
		t.Error("expected claude session-tracking hook to be installed")
	}

	skillPath := filepath.Join(home, ".claude", "skills", "worktree-studio", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("expected skill file at %s: %v", skillPath, err)
	}
}

func TestUninstallHooksRemovesClaudeHookAndSkill(t *testing.T) {
	home := withFakeHome(t)

	if code := installHooks(); code != 0 {
		t.Fatalf("installHooks() = %d, want 0", code)
	}
	if code := uninstallHooks(); code != 0 {
		t.Fatalf("uninstallHooks() = %d, want 0", code)
	}

	installed, err := claudehook.IsHookInstalled()
	if err != nil {
		t.Fatalf("IsHookInstalled: %v", err)
	}
	if installed {
		t.Error("expected claude session-tracking hook to be uninstalled")
	}

	skillDir := filepath.Join(home, ".claude", "skills", "worktree-studio")
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Errorf("expected skill directory to be removed, stat err = %v", err)
	}
}

func TestUninstallHooksNoOpWhenNeverInstalled(t *testing.T) {
	withFakeHome(t)
	if code := uninstallHooks(); code != 0 {
		t.Fatalf("uninstallHooks() on a machine where hooks were never installed = %d, want 0", code)
	}
}

func TestRunHooksCommandFallsThroughForUnknownArgs(t *testing.T) {
	if runHooksCommand(nil) {
		t.Error("runHooksCommand(nil) = true, want false (no subcommand given)")
	}
	if runHooksCommand([]string{"serve"}) {
		t.Error(`runHooksCommand(["serve"]) = true, want false (not a hooks subcommand)`)
	}
}
