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

func TestInstallHooksInstallsEveryRegisteredHookAndSkill(t *testing.T) {
	home := withFakeHome(t)

	if code := installHooks(); code != 0 {
		t.Fatalf("installHooks() = %d, want 0", code)
	}

	// Every hook in the registry, not just the original session-tracking
	// one — installHooks loops over claudehook.Hooks() precisely so this
	// CLI path never falls behind the settings UI's own hooks list.
	for _, hook := range claudehook.Hooks() {
		installed, err := claudehook.IsHookInstalledByID(hook.ID)
		if err != nil {
			t.Fatalf("IsHookInstalledByID(%q): %v", hook.ID, err)
		}
		if !installed {
			t.Errorf("expected %s (%s) to be installed", hook.Name, hook.ID)
		}
	}

	skillPath := filepath.Join(home, ".claude", "skills", "worktree-studio", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("expected skill file at %s: %v", skillPath, err)
	}
}

func TestUninstallHooksRemovesEveryRegisteredHookAndSkill(t *testing.T) {
	home := withFakeHome(t)

	if code := installHooks(); code != 0 {
		t.Fatalf("installHooks() = %d, want 0", code)
	}
	if code := uninstallHooks(); code != 0 {
		t.Fatalf("uninstallHooks() = %d, want 0", code)
	}

	for _, hook := range claudehook.Hooks() {
		installed, err := claudehook.IsHookInstalledByID(hook.ID)
		if err != nil {
			t.Fatalf("IsHookInstalledByID(%q): %v", hook.ID, err)
		}
		if installed {
			t.Errorf("expected %s (%s) to be uninstalled", hook.Name, hook.ID)
		}
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
