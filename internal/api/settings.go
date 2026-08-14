// Global settings endpoints: dependency status ("is tmux installed?") and
// the install/uninstall actions for the ones this server can actually act
// on (the claude hooks in internal/claudehook's registry, and the skill).
// The hooks list is dynamic — handleGetHooks/handleInstallHook/
// handleUninstallHook all operate on whatever claudehook.Hooks() returns,
// so a new hook registered there needs no change here. Every
// install/uninstall action here is only ever reached via an explicit user
// click in the settings UI — never run automatically, since editing the
// user's real ~/.claude/settings.json is not something to do without
// asking. See internal/claudehook/install.go for the actual file-editing
// logic and its safety rationale.
package api

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/go-chi/chi/v5"

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
		"tmux":       checkOnPath("tmux", "brew install tmux (macOS) or your package manager's tmux"),
		"spotlight":  checkSpotlight(),
		"skill":      checkSkillInstalled(),
		"vscode_cli": checkOnPath("code", `install from VS Code: Cmd+Shift+P -> "Shell Command: Install 'code' command in PATH"`),
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

// hookStatus is one row of the dynamic "Claude Code hooks" list: every
// hook claudehook.Hooks() knows about, plus its live installed state. The
// settings UI renders this list as-is — adding a hook to
// internal/claudehook's registry is the only change needed for a new row
// to show up here, with no further wiring on either side.
type hookStatus struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Hint      string `json:"hint,omitempty"`
	Installed bool   `json:"installed"`
}

func (s *Server) handleGetHooks(w http.ResponseWriter, r *http.Request) {
	infos := claudehook.Hooks()
	result := make([]hookStatus, 0, len(infos))
	for _, info := range infos {
		installed, err := claudehook.IsHookInstalledByID(info.ID)
		if err != nil {
			s.Log.Error("check hook status", "id", info.ID, "err", err)
			writeError(w, http.StatusInternalServerError, "failed to read ~/.claude/settings.json: "+err.Error())
			return
		}
		result = append(result, hookStatus{ID: info.ID, Name: info.Name, Hint: info.Hint, Installed: installed})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleInstallHook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := claudehook.InstallHookByID(id, s.SelfBaseURL); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, claudehook.ErrUnknownHook) {
			status = http.StatusNotFound
		}
		s.Log.Error("install hook", "id", id, "err", err)
		writeError(w, status, "failed to install hook: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "installed"})
}

func (s *Server) handleUninstallHook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := claudehook.UninstallHookByID(id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, claudehook.ErrUnknownHook) {
			status = http.StatusNotFound
		}
		s.Log.Error("uninstall hook", "id", id, "err", err)
		writeError(w, status, "failed to uninstall hook: "+err.Error())
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
