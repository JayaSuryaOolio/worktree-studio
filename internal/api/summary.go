// The sidebar's hover-summary popover: a worktree's git status (ahead/
// behind, dirty file list) plus its branch's pull request, if any. See
// gitops.Status/ChangedFiles and gh.PRForBranch for where the actual data
// comes from.
//
// Deliberately NOT cached or rate-limited server-side — the frontend is
// what owns that (a localStorage cache with a several-minute TTL,
// refreshed opportunistically on hover rather than on a timer; see
// web/src/prGitCache.ts), since it's the one place that actually knows how
// often a person is hovering worktree rows. This endpoint just answers
// "what's true right now" on request, the same way handleWorktreeStatus
// already does for the plainer git-only status badge.
package api

import (
	"net/http"

	"worktree-studio/internal/gh"
	"worktree-studio/internal/gitops"
)

type prSummary struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	URL     string `json:"url"`
	IsDraft bool   `json:"is_draft"`
}

type worktreeSummary struct {
	Branch       string     `json:"branch"`
	Ahead        int        `json:"ahead"`
	Behind       int        `json:"behind"`
	HasUpstream  bool       `json:"has_upstream"`
	Dirty        bool       `json:"dirty"`
	ChangedFiles []string   `json:"changed_files"`
	PR           *prSummary `json:"pr"`
}

func (s *Server) handleWorktreeSummary(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	status, err := gitops.Status(wt.Path)
	if err != nil {
		s.Log.Error("git status", "err", err, "worktree_id", wt.ID)
		writeError(w, http.StatusInternalServerError, "failed to get worktree status: "+err.Error())
		return
	}

	// Best-effort beyond this point: a worktree's git status is the load-
	// bearing part of this response (the sidebar already shows a version
	// of it elsewhere), so a changed-files or gh failure degrades to an
	// empty list / no PR rather than failing the whole popover.
	//
	// Always a real (possibly empty) slice, never nil: a nil []string
	// marshals to JSON `null`, not `[]`, and the frontend's TypeScript
	// type promises callers a real array — a real bug found this way, not
	// a guess: the clean-worktree case (the common one) sent
	// changed_files: null, and WorktreeHoverPopover.tsx's unguarded
	// `summary.changed_files.length` threw on it, crashing the whole
	// sidebar on hover.
	changedFiles, err := gitops.ChangedFiles(wt.Path)
	if err != nil {
		s.Log.Warn("list changed files", "err", err, "worktree_id", wt.ID)
	}
	if changedFiles == nil {
		changedFiles = []string{}
	}

	var pr *prSummary
	if prInfo, err := gh.PRForBranch(wt.Path, status.Branch); err != nil {
		s.Log.Warn("look up PR for branch", "err", err, "worktree_id", wt.ID, "branch", status.Branch)
	} else if prInfo != nil {
		pr = &prSummary{
			Number:  prInfo.Number,
			Title:   prInfo.Title,
			State:   prInfo.State,
			URL:     prInfo.URL,
			IsDraft: prInfo.IsDraft,
		}
	}

	writeJSON(w, http.StatusOK, worktreeSummary{
		Branch:       status.Branch,
		Ahead:        status.Ahead,
		Behind:       status.Behind,
		HasUpstream:  status.HasUpstream,
		Dirty:        status.Dirty,
		ChangedFiles: changedFiles,
		PR:           pr,
	})
}
