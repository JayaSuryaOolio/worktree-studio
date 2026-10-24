package api

import (
	"net/http"

	"worktree-studio/internal/claudehook"
	"worktree-studio/internal/term"
)

// handleListWorktreeClaudeSessions returns every past claude session found
// for a worktree's own path under ~/.claude/projects/ (see
// claudehook.ListSessionsForCwd) — the dockview watermark's "past
// sessions" welcome screen, letting someone pick up an old conversation
// instead of only ever starting fresh. Independent of the audit log/DB:
// this reflects whatever Claude Code itself still has on disk, so it also
// surfaces sessions started by hand outside worktree-studio entirely.
//
// Sessions running right now in the worktree (or a subdirectory) are
// marked running and listed first, from Claude Code's live-session
// registry (see claudehook.ListRunning), with the worktree-studio terminal
// hosting each one when there is one. The registry is best-effort: if it
// can't be read the list is just the transcripts, as before.
func (s *Server) handleListWorktreeClaudeSessions(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	sessions, err := claudehook.ListSessionsForCwd(wt.Path)
	if err != nil {
		s.Log.Error("list claude sessions", "err", err, "worktree_id", wt.ID)
		writeError(w, http.StatusInternalServerError, "failed to list claude sessions: "+err.Error())
		return
	}
	if dir, err := claudehook.SessionRegistryDir(); err == nil {
		if running := claudehook.ListRunning(dir, wt.Path, claudehook.PidAlive); len(running) > 0 {
			sessions = claudehook.MergeRunning(sessions, running, s.claudeTerminalFinder(wt.ID))
		}
	}
	writeJSON(w, http.StatusOK, sessions)
}

// claudeTerminalFinder returns a lookup from a running claude session to
// the id of this worktree's terminal hosting it: by the registry's tmux
// session name for tmux terminals, and for ptyd terminals by walking the
// claude process's parents up to a shell the daemon owns.
func (s *Server) claudeTerminalFinder(worktreeID string) func(claudehook.RunningSession) string {
	terminals, _ := s.Store.ListTerminalSessions(worktreeID)
	byTmux := map[string]string{}
	ptydIDs := map[string]bool{}
	for _, ts := range terminals {
		if ts.Backend == term.BackendPtyd {
			ptydIDs[ts.ID] = true
		} else {
			byTmux[ts.TmuxSessionName] = ts.ID
		}
	}

	var byShell map[int]string // ptyd shell pid -> terminal id, built on first need
	var parents map[int]int
	return func(rs claudehook.RunningSession) string {
		if rs.Tmux != "" {
			return byTmux[claudehook.TmuxSessionName(rs.Tmux)]
		}
		if len(ptydIDs) == 0 || s.Term == nil || s.Term.Ptyd == nil {
			return ""
		}
		if byShell == nil {
			byShell = map[int]string{}
			live, _ := s.Term.Ptyd.List()
			for _, p := range live {
				if ptydIDs[p.ID] {
					byShell[p.Pid] = p.ID
				}
			}
			parents = claudehook.ProcessParents()
		}
		return claudehook.AncestorIn(rs.PID, byShell, parents)
	}
}
