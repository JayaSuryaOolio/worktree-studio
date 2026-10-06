// Package app holds the TUI's use cases and the ports they need. The
// infrastructure (HTTP/websocket) and the UI both depend on this package;
// it depends only on domain.
package app

import (
	"context"
	"sync"

	"worktree-studio/tui/domain"
)

// Workspace is the port to the worktree-studio server's registry.
type Workspace interface {
	Repos(ctx context.Context) ([]domain.Repo, error)
	Worktrees(ctx context.Context, repo domain.RepoID) ([]domain.Worktree, error)
	GitStatus(ctx context.Context, repo domain.RepoID, wt domain.WorktreeID) (domain.GitStatus, error)
	ClearAttention(ctx context.Context, repo domain.RepoID, wt domain.WorktreeID) error
}

// AttentionFeed is the port to the live "Claude needs you" stream. It
// reconnects internally and closes the channel only when ctx is done.
type AttentionFeed interface {
	Subscribe(ctx context.Context) <-chan domain.AttentionEvent
}

type Sidebar struct{ ws Workspace }

func NewSidebar(ws Workspace) *Sidebar { return &Sidebar{ws: ws} }

// Active returns the active worktrees of repo in the server's order (pinned
// first), without git status — cheap enough to do for every repo up front.
func (s *Sidebar) Active(ctx context.Context, repo domain.RepoID) ([]domain.SidebarItem, error) {
	all, err := s.ws.Worktrees(ctx, repo)
	if err != nil {
		return nil, err
	}
	var items []domain.SidebarItem
	for _, wt := range all {
		if wt.Active {
			items = append(items, domain.SidebarItem{Worktree: wt})
		}
	}
	return items, nil
}

// Load is Active plus each row's git status, fetched concurrently. A failed
// status lookup leaves that row's Git nil rather than failing the sidebar.
func (s *Sidebar) Load(ctx context.Context, repo domain.RepoID) ([]domain.SidebarItem, error) {
	items, err := s.Active(ctx, repo)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	for i := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if g, err := s.ws.GitStatus(ctx, repo, items[i].Worktree.ID); err == nil {
				items[i].Git = &g
			}
		}()
	}
	wg.Wait()
	return items, nil
}

func (s *Sidebar) Repos(ctx context.Context) ([]domain.Repo, error) { return s.ws.Repos(ctx) }

// MarkSeen dismisses a worktree's attention badge, like opening it in the browser.
func (s *Sidebar) MarkSeen(ctx context.Context, wt domain.Worktree) error {
	return s.ws.ClearAttention(ctx, wt.RepoID, wt.ID)
}
