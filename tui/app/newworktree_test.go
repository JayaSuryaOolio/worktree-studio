package app

import (
	"context"
	"errors"
	"testing"

	"worktree-studio/tui/domain"
)

type fakeProv struct {
	nameErr, brErr error
	created        [2]string
}

func (f *fakeProv) NameSuggestion(context.Context, domain.RepoID) (string, error) {
	return "calm-otter", f.nameErr
}
func (f *fakeProv) Branches(context.Context, domain.RepoID) (domain.BranchChoices, error) {
	return domain.NewBranchChoices([]string{"main"}, "main"), f.brErr
}
func (f *fakeProv) CreateWorktree(_ context.Context, _ domain.RepoID, n, s string) (domain.Worktree, error) {
	f.created = [2]string{n, s}
	return domain.Worktree{}, nil
}

func TestPrepareToleratesNameFailureButNotBranches(t *testing.T) {
	d, err := NewNewWorktree(&fakeProv{nameErr: errors.New("x")}).Prepare(context.Background(), "r")
	if err != nil || d.Name != "" || len(d.Branches.Branches) != 1 {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := NewNewWorktree(&fakeProv{brErr: errors.New("x")}).Prepare(context.Background(), "r"); err == nil {
		t.Fatal("branch failure must surface")
	}
}

func TestCreateTrimsAndRejectsEmpty(t *testing.T) {
	p := &fakeProv{}
	uc := NewNewWorktree(p)
	if _, err := uc.Create(context.Background(), "r", "  ", "main"); !errors.Is(err, ErrEmptyName) {
		t.Fatal("empty name must be rejected")
	}
	uc.Create(context.Background(), "r", " feat-x ", "main")
	if p.created != [2]string{"feat-x", "main"} {
		t.Fatalf("%v", p.created)
	}
}
