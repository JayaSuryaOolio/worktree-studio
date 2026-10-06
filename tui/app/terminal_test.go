package app

import (
	"context"
	"testing"

	"worktree-studio/tui/domain"
)

type fakeDir struct {
	sessions []domain.TerminalSession
	kinds    []domain.TerminalKind
}

func (f *fakeDir) Sessions(context.Context, domain.RepoID, domain.WorktreeID) ([]domain.TerminalSession, error) {
	return f.sessions, nil
}
func (f *fakeDir) Create(_ context.Context, _ domain.RepoID, _ domain.WorktreeID, k domain.TerminalKind) (domain.TerminalSession, error) {
	s := domain.TerminalSession{ID: string(rune('a' + len(f.sessions))), Label: k.Label}
	f.kinds = append(f.kinds, k)
	f.sessions = append(f.sessions, s)
	return s, nil
}

func TestTabsCreatesShellOnlyWhenEmpty(t *testing.T) {
	d := &fakeDir{}
	uc := NewTerminals(d, nil)
	tabs, _ := uc.Tabs(context.Background(), domain.Worktree{})
	if len(tabs) != 1 || d.kinds[0] != domain.ShellTerminal {
		t.Fatalf("%+v %+v", tabs, d.kinds)
	}
	tabs, _ = uc.Tabs(context.Background(), domain.Worktree{})
	if len(tabs) != 1 || len(d.kinds) != 1 {
		t.Fatal("must not create again when a session exists")
	}
}

func TestAddReturnsIndexOfNewTab(t *testing.T) {
	d := &fakeDir{sessions: []domain.TerminalSession{{ID: "x"}}}
	tabs, idx, err := NewTerminals(d, nil).Add(context.Background(), domain.Worktree{}, domain.ClaudeTerminal)
	if err != nil || len(tabs) != 2 || idx != 1 || tabs[idx].Label != "claude" || d.kinds[0].Command != "claude" {
		t.Fatalf("%+v idx=%d err=%v", tabs, idx, err)
	}
}
