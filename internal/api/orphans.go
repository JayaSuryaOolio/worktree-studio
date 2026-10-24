// Orphan tmux session cleanup: a live tmux session in worktree-studio's own
// namespace with no terminal_sessions row, e.g. a leaked test session or
// one created by hand outside the app. Distinct from Reconcile (a DB row
// with no live session, handled at startup) — this is the opposite
// mismatch, and it's exposed here rather than left as an ad-hoc shell
// exercise precisely because "kill whatever looks orphaned" was already
// tried informally once and swept up real, still-in-use sessions along
// with it. See internal/term/orphans.go for the actual safeguard. ptyd
// sessions with no row are listed too, named "ptyd:<id>" (term.FindOrphans).
package api

import (
	"net/http"
	"strconv"
	"time"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/term"
)

// defaultOrphanMinAge is the safety window used when a caller doesn't
// specify one: a session touched more recently than this is never killed.
const defaultOrphanMinAge = 7 * 24 * time.Hour

// parseMinAge reads an optional ?min_age_hours= query param, falling back
// to defaultOrphanMinAge when it's missing or not a valid non-negative
// number. 0 is valid and deliberately not treated as "unset" — it means
// "protect nothing," used by tests and by anyone who's already certain
// there's nothing worth protecting.
func parseMinAge(r *http.Request) time.Duration {
	if v := r.URL.Query().Get("min_age_hours"); v != "" {
		if hours, err := strconv.ParseFloat(v, 64); err == nil && hours >= 0 {
			return time.Duration(hours * float64(time.Hour))
		}
	}
	return defaultOrphanMinAge
}

type orphanTmuxSessionJSON struct {
	Name         string `json:"name"`
	LastActivity string `json:"last_activity,omitempty"`
	Protected    bool   `json:"protected"`
}

func toOrphanJSON(o term.OrphanTmuxSession) orphanTmuxSessionJSON {
	out := orphanTmuxSessionJSON{Name: o.Name, Protected: o.Protected}
	if !o.LastActivity.IsZero() {
		out.LastActivity = o.LastActivity.UTC().Format(time.RFC3339)
	}
	return out
}

// handleListOrphanTmuxSessions previews what a prune would do (see
// term.FindOrphanTmuxSessions) without killing anything — what
// `worktree-studio orphans` shows by default.
func (s *Server) handleListOrphanTmuxSessions(w http.ResponseWriter, r *http.Request) {
	orphans, err := s.Term.FindOrphans(parseMinAge(r))
	if err != nil {
		s.Log.Error("find orphan tmux sessions", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list orphan tmux sessions")
		return
	}
	out := make([]orphanTmuxSessionJSON, 0, len(orphans))
	for _, o := range orphans {
		out = append(out, toOrphanJSON(o))
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePruneOrphanTmuxSessions kills every orphan tmux session that isn't
// protected by the activity safeguard (see term.KillOrphanTmuxSessions,
// which enforces this itself — not repeated here) and audit-logs each kill.
func (s *Server) handlePruneOrphanTmuxSessions(w http.ResponseWriter, r *http.Request) {
	minAge := parseMinAge(r)
	killed, protected, err := s.Term.KillOrphans(minAge)
	if err != nil {
		s.Log.Error("kill orphan tmux sessions", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to prune orphan tmux sessions")
		return
	}
	for _, name := range killed {
		s.auditLog(audit.EventOrphanTmuxKill, map[string]any{
			"tmux_session_name": name,
			"min_age_hours":     minAge.Hours(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"killed": killed, "protected": protected})
}

// parseIdleDays reads an optional ?days= query param, falling back to the
// idle reaper setting (0, off, if unset).
func (s *Server) parseIdleDays(r *http.Request) int {
	if n, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && n >= 0 {
		return n
	}
	return s.Term.IdleReapDays()
}

// handleListIdlePtydSessions previews the idle reaper (term.FindIdle): ptyd
// terminals idle for the window, busy ones marked. Nothing is closed.
func (s *Server) handleListIdlePtydSessions(w http.ResponseWriter, r *http.Request) {
	idle, err := s.Term.FindIdle(s.parseIdleDays(r))
	if err != nil {
		s.Log.Error("find idle ptyd sessions", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list idle ptyd sessions")
		return
	}
	if idle == nil {
		idle = []term.IdleTerminal{}
	}
	writeJSON(w, http.StatusOK, idle)
}

// handleReapIdlePtydSessions runs the idle reaper now (see ReapIdlePtyd).
func (s *Server) handleReapIdlePtydSessions(w http.ResponseWriter, r *http.Request) {
	reaped, busy, err := s.ReapIdlePtyd(s.parseIdleDays(r))
	if err != nil {
		s.Log.Error("reap idle ptyd sessions", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to reap idle ptyd sessions")
		return
	}
	if reaped == nil {
		reaped = []term.IdleTerminal{}
	}
	if busy == nil {
		busy = []term.IdleTerminal{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reaped": reaped, "busy": busy})
}

// ReapIdlePtyd closes ptyd terminals idle for days (term.ReapIdle; each close
// is audit-logged there) and logs the busy ones it left alone. days 0 is a
// no-op.
func (s *Server) ReapIdlePtyd(days int) (reaped, busy []term.IdleTerminal, err error) {
	reaped, busy, err = s.Term.ReapIdle(days)
	if err == nil && len(reaped)+len(busy) > 0 {
		s.Log.Info("idle ptyd reaper", "days", days, "reaped", len(reaped), "busy_left_running", len(busy))
	}
	return reaped, busy, err
}
