// Package domain holds the TUI's pure model of what the sidebar shows:
// worktrees, their git state, and which ones have a Claude session waiting.
// No I/O and no UI imports — the same concepts web/src/api.ts models, so
// they can later be shared with the browser client.
package domain

import (
	"fmt"
	"strings"
)

type (
	RepoID     string
	WorktreeID string
)

type Repo struct {
	ID   RepoID
	Name string
	Path string
}

type Worktree struct {
	ID     WorktreeID
	RepoID RepoID
	Name   string
	Branch string
	Pinned bool
	Active bool // lifecycle "active" (vs archived/deleted)
}

type GitStatus struct {
	Dirty       bool
	HasUpstream bool
	Ahead       int
	Behind      int
}

// Ticks renders "↑2↓1"; empty when there is no upstream or nothing to sync.
func (g GitStatus) Ticks() string {
	if !g.HasUpstream || (g.Ahead == 0 && g.Behind == 0) {
		return ""
	}
	s := ""
	if g.Ahead > 0 {
		s += fmt.Sprintf("↑%d", g.Ahead)
	}
	if g.Behind > 0 {
		s += fmt.Sprintf("↓%d", g.Behind)
	}
	return s
}

// SidebarItem is one worktree row: the worktree plus its git status when
// known (nil if the status lookup failed).
type SidebarItem struct {
	Worktree Worktree
	Git      *GitStatus
}

// Attention is the set of worktrees whose Claude session needs the user,
// keyed to the message it is waiting with.
type Attention map[WorktreeID]string

// AttentionEvent mirrors the /ws/attention protocol: a full snapshot on
// connect, then one update per change.
type AttentionEvent struct {
	Snapshot  bool
	Pending   Attention // set when Snapshot
	Worktree  WorktreeID
	IsPending bool
	Message   string
}

// Apply returns the attention set after ev, without mutating the receiver.
func (a Attention) Apply(ev AttentionEvent) Attention {
	if ev.Snapshot {
		next := Attention{}
		for k, v := range ev.Pending {
			next[k] = v
		}
		return next
	}
	next := make(Attention, len(a)+1)
	for k, v := range a {
		next[k] = v
	}
	if ev.IsPending {
		next[ev.Worktree] = ev.Message
	} else {
		delete(next, ev.Worktree)
	}
	return next
}

// BranchChoices are the branches a new worktree can start from, plus the
// one the server would use if the user picked nothing.
type BranchChoices struct {
	Branches []string
	Default  string
}

// NewBranchChoices guarantees Default is selectable: the server may resolve
// it to a bare name with no matching listed ref (see NewWorktreeDialog.tsx),
// so it is prepended when missing.
func NewBranchChoices(branches []string, def string) BranchChoices {
	if def != "" && !contains(branches, def) {
		branches = append([]string{def}, branches...)
	}
	return BranchChoices{Branches: branches, Default: def}
}

// DefaultIndex is the position of Default in Branches (0 if unset).
func (b BranchChoices) DefaultIndex() int {
	for i, br := range b.Branches {
		if br == b.Default {
			return i
		}
	}
	return 0
}

// Filter returns the branches containing q (case-insensitive), in order.
func (b BranchChoices) Filter(q string) []string {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return b.Branches
	}
	var out []string
	for _, br := range b.Branches {
		if strings.Contains(strings.ToLower(br), q) {
			out = append(out, br)
		}
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// TerminalSession is a server-managed (tmux-backed) shell in a worktree.
type TerminalSession struct {
	ID       string
	Worktree WorktreeID
	TmuxName string
	Label    string
}

// TerminalKind is what a new terminal tab runs. Command is typed into the
// fresh shell by the server (empty = a plain shell); Label names the tab.
type TerminalKind struct{ Label, Command string }

var (
	ShellTerminal  = TerminalKind{Label: "shell"}
	ClaudeTerminal = TerminalKind{Label: "claude", Command: "claude"}
)
