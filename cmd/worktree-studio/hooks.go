package main

import (
	"fmt"
	"os"
	"path/filepath"

	"worktree-studio/internal/claudehook"
	"worktree-studio/internal/skillasset"
)

// hooksLogPrefix namespaces every line these subcommands print, so
// install.sh/uninstall.sh's output is unambiguous about which tool it came
// from when run alongside other projects' own install scripts.
const hooksLogPrefix = "worktree-studio__hooks: "

// runHooksCommand handles the `install-hooks`/`uninstall-hooks` subcommands
// (see install/install.sh and install/uninstall.sh), letting those scripts
// drive the exact same claude-hook and skill install/uninstall logic the
// settings UI's "Installation" tab already uses (internal/api/settings.go)
// without needing a running server to hit over HTTP. Returns whether args[0]
// was one of these subcommands at all — false means main() should fall
// through to running the server as normal.
func runHooksCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "install-hooks":
		os.Exit(installHooks())
	case "uninstall-hooks":
		os.Exit(uninstallHooks())
	default:
		return false
	}
	return true
}

func installHooks() int {
	addr := defaultAddr
	if v := os.Getenv("WORKTREE_STUDIO_ADDR"); v != "" {
		addr = v
	}
	selfBaseURL := "http://localhost" + addrPort(addr)

	// Every registered hook (internal/claudehook/install.go's hookRegistry)
	// gets installed here — not just the original session-tracking one —
	// so this CLI path stays in sync with the settings UI's "Claude Code
	// hooks" list without needing to name each hook by hand.
	for _, hook := range claudehook.Hooks() {
		if err := claudehook.InstallHookByID(hook.ID, selfBaseURL); err != nil {
			fmt.Fprintf(os.Stderr, hooksLogPrefix+"install %s: %v\n", hook.Name, err)
			return 1
		}
		fmt.Println(hooksLogPrefix + "installed " + hook.Name)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, hooksLogPrefix+"resolve home dir: %v\n", err)
		return 1
	}
	skillDir := filepath.Join(home, ".claude", "skills", "worktree-studio")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, hooksLogPrefix+"create skill directory: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillasset.Content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, hooksLogPrefix+"write skill file: %v\n", err)
		return 1
	}
	fmt.Println(hooksLogPrefix + "installed worktree-studio skill to " + skillDir)
	return 0
}

func uninstallHooks() int {
	for _, hook := range claudehook.Hooks() {
		if err := claudehook.UninstallHookByID(hook.ID); err != nil {
			fmt.Fprintf(os.Stderr, hooksLogPrefix+"uninstall %s: %v\n", hook.Name, err)
			return 1
		}
		fmt.Println(hooksLogPrefix + "uninstalled " + hook.Name)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, hooksLogPrefix+"resolve home dir: %v\n", err)
		return 1
	}
	skillDir := filepath.Join(home, ".claude", "skills", "worktree-studio")
	if err := os.RemoveAll(skillDir); err != nil {
		fmt.Fprintf(os.Stderr, hooksLogPrefix+"remove skill directory: %v\n", err)
		return 1
	}
	fmt.Println(hooksLogPrefix + "removed worktree-studio skill from " + skillDir)
	return 0
}
