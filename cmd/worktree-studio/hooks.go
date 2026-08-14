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

	if err := claudehook.InstallHook(selfBaseURL); err != nil {
		fmt.Fprintf(os.Stderr, hooksLogPrefix+"install claude session-tracking hook: %v\n", err)
		return 1
	}
	fmt.Println(hooksLogPrefix + "installed claude session-tracking hook")

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
	if err := claudehook.UninstallHook(); err != nil {
		fmt.Fprintf(os.Stderr, hooksLogPrefix+"uninstall claude session-tracking hook: %v\n", err)
		return 1
	}
	fmt.Println(hooksLogPrefix + "uninstalled claude session-tracking hook")

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
