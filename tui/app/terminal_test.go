package app

import (
	"context"
	"testing"

	"worktree-studio/tui/domain"
)

type fakeDir struct {
	existing []domain.TerminalSession
	created  int
}

func (f *fakeDir) Sessions(context.Context, domain.RepoID, domain.WorktreeID) ([]domain.TerminalSession, error) {
	return f.existing, nil
}
func (f *fakeDir) Create(context.Context, domain.RepoID, domain.WorktreeID, string) (domain.TerminalSession, error) {
	f.created++
	return domain.TerminalSession{TmuxName: "new"}, nil
}

type fakeAtt struct{ got string }

func (f *fakeAtt) Attach(s domain.TerminalSession, _, _ int) (Screen, error) {
	f.got = s.TmuxName
	return nil, nil
}

func TestOpenReusesFirstSessionElseCreates(t *testing.T) {
	d, a := &fakeDir{existing: []domain.TerminalSession{{TmuxName: "one"}, {TmuxName: "two"}}}, &fakeAtt{}
	NewTerminals(d, a).Open(context.Background(), domain.Worktree{}, 80, 24)
	if a.got != "one" || d.created != 0 {
		t.Fatalf("got %q created %d", a.got, d.created)
	}
	d.existing = nil
	NewTerminals(d, a).Open(context.Background(), domain.Worktree{}, 80, 24)
	if a.got != "new" || d.created != 1 {
		t.Fatalf("got %q created %d", a.got, d.created)
	}
}
