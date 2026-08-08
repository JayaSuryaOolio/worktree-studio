// Global settings endpoints: dependency status ("is tmux installed?", "is
// our claude hook wired up?") and the install/uninstall actions for the
// ones this server can actually act on (claude hook, skill). Every
// install/uninstall action here is only ever reached via an explicit user
// click in the settings UI — never run automatically, since editing the
// user's real ~/.claude/settings.json is not something to do without
// asking. See internal/claudehook/install.go for the actual file-editing
// logic and its safety rationale.
package api

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"worktree-studio/internal/claudehook"
	"worktree-studio/internal/skillasset"
)

type dependencyStatus struct {
	Installed   bool   `json:"installed"`
	Detail      string `json:"detail,omitempty"`
	InstallHint string `json:"install_hint,omitempty"`
}

func (s *Server) handleGetDependencyStatus(w http.ResponseWriter, r *http.Request) {
	result := map[string]dependencyStatus{
		"tmux":        checkOnPath("tmux", "brew install tmux (macOS) or your package manager's tmux"),
		"spotlight":   checkSpotlight(),
		"skill":       checkSkillInstalled(),
		"claude_hook": checkClaudeHook(),
		"vscode_cli":  checkOnPath("code", `install from VS Code: Cmd+Shift+P -> "Shell Command: Install 'code' command in PATH"`),
	}
	writeJSON(w, http.StatusOK, result)
}

func checkOnPath(bin, hint string) dependencyStatus {
	path, err := exec.LookPath(bin)
	if err != nil {
		return dependencyStatus{Installed: false, InstallHint: hint}
	}
	return dependencyStatus{Installed: true, Detail: path}
}

func checkSpotlight() dependencyStatus {
	if path, err := exec.LookPath("spotlight"); err == nil {
		return dependencyStatus{Installed: true, Detail: path}
	}
	// Mirrors internal/spotlight's own PATH-then-~/.local/bin fallback.
	if home, err := os.UserHomeDir(); err == nil {
		fallback := filepath.Join(home, ".local", "bin", "spotlight")
		if info, err := os.Stat(fallback); err == nil && !info.IsDir() {
			return dependencyStatus{Installed: true, Detail: fallback}
		}
	}
	return dependencyStatus{
		Installed:   false,
		InstallHint: "see docs/spotlight-sync.md — github.com/JayaSuryaOolio/spotlight, plus `brew install fswatch`",
	}
}

func checkSkillInstalled() dependencyStatus {
	home, err := os.UserHomeDir()
	if err != nil {
		return dependencyStatus{Installed: false, InstallHint: "could not resolve home directory"}
	}
	path := filepath.Join(home, ".claude", "skills", "worktree-studio", "SKILL.md")
	if _, err := os.Stat(path); err == nil {
		return dependencyStatus{Installed: true, Detail: path}
	}
	return dependencyStatus{Installed: false, InstallHint: "install from this settings page to use worktree-studio from any project"}
}

func checkClaudeHook() dependencyStatus {
	installed, err := claudehook.IsHookInstalled()
	if err != nil {
		return dependencyStatus{Installed: false, InstallHint: "failed to read ~/.claude/settings.json: " + err.Error()}
	}
	if installed {
		return dependencyStatus{Installed: true}
	}
	return dependencyStatus{Installed: false, InstallHint: "install from this settings page to track claude sessions started by hand, not just ones worktree-studio auto-starts"}
}

func (s *Server) handleInstallClaudeHook(w http.ResponseWriter, r *http.Request) {
	if err := claudehook.InstallHook(s.SelfBaseURL); err != nil {
		s.Log.Error("install claude hook", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to install claude hook: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "installed"})
}

func (s *Server) handleUninstallClaudeHook(w http.ResponseWriter, r *http.Request) {
	if err := claudehook.UninstallHook(); err != nil {
		s.Log.Error("uninstall claude hook", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to uninstall claude hook: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled"})
}

func (s *Server) handleInstallSkill(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve home directory")
		return
	}
	dir := filepath.Join(home, ".claude", "skills", "worktree-studio")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.Log.Error("install skill: mkdir", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to create skill directory: "+err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillasset.Content), 0o644); err != nil {
		s.Log.Error("install skill: write", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to write skill file: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "installed"})
}
