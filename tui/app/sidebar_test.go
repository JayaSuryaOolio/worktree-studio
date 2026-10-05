package app

import (
	"context"
	"errors"
	"testing"

	"worktree-studio/tui/domain"
)

type fakeWS struct{}

func (fakeWS) Repos(context.Context) ([]domain.Repo, error) { return nil, nil }
func (fakeWS) Worktrees(context.Context, domain.RepoID) ([]domain.Worktree, error) {
	return []domain.Worktree{
		{ID: "a", Active: true}, {ID: "b", Active: false}, {ID: "c", Active: true},
	}, nil
}
func (fakeWS) GitStatus(_ context.Context, _ domain.RepoID, id domain.WorktreeID) (domain.GitStatus, error) {
	if id == "c" {
		return domain.GitStatus{}, errors.New("boom")
	}
	return domain.GitStatus{Dirty: true}, nil
}
func (fakeWS) ClearAttention(context.Context, domain.RepoID, domain.WorktreeID) error { return nil }

func TestLoadFiltersInactiveAndToleratesStatusErrors(t *testing.T) {
	items, err := NewSidebar(fakeWS{}).Load(context.Background(), "r")
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%d err=%v", len(items), err)
	}
	if items[0].Git == nil || !items[0].Git.Dirty {
		t.Error("a should have status")
	}
	if items[1].Git != nil {
		t.Error("c status failed; Git must be nil")
	}
}
