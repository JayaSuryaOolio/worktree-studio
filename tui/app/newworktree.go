package app

import (
	"context"
	"errors"
	"strings"
	"sync"

	"worktree-studio/tui/domain"
)

// Provisioner is the port for creating worktrees.
type Provisioner interface {
	NameSuggestion(ctx context.Context, repo domain.RepoID) (string, error)
	Branches(ctx context.Context, repo domain.RepoID) (domain.BranchChoices, error)
	CreateWorktree(ctx context.Context, repo domain.RepoID, name, sourceBranch string) (domain.Worktree, error)
}

// Draft is what the new-worktree form starts with.
type Draft struct {
	Name     string
	Branches domain.BranchChoices
}

type NewWorktree struct{ p Provisioner }

func NewNewWorktree(p Provisioner) *NewWorktree { return &NewWorktree{p: p} }

// Prepare fetches the suggested name and branch list concurrently. A failed
// name suggestion is tolerated (the user can type one); a failed branch
// lookup is not, since the form can't offer a source.
func (n *NewWorktree) Prepare(ctx context.Context, repo domain.RepoID) (Draft, error) {
	var (
		wg      sync.WaitGroup
		d       Draft
		brErr   error
		nameErr error
	)
	wg.Add(2)
	go func() { defer wg.Done(); d.Name, nameErr = n.p.NameSuggestion(ctx, repo) }()
	go func() { defer wg.Done(); d.Branches, brErr = n.p.Branches(ctx, repo) }()
	wg.Wait()
	if brErr != nil {
		return Draft{}, brErr
	}
	if nameErr != nil {
		d.Name = ""
	}
	return d, nil
}

var ErrEmptyName = errors.New("name is required")

func (n *NewWorktree) Create(ctx context.Context, repo domain.RepoID, name, sourceBranch string) (domain.Worktree, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Worktree{}, ErrEmptyName
	}
	return n.p.CreateWorktree(ctx, repo, name, sourceBranch)
}
