package app

import (
	"context"

	"worktree-studio/tui/domain"
)

// TerminalDirectory is the port to the server's terminal-session registry.
type TerminalDirectory interface {
	Sessions(ctx context.Context, repo domain.RepoID, wt domain.WorktreeID) ([]domain.TerminalSession, error)
	Create(ctx context.Context, repo domain.RepoID, wt domain.WorktreeID, kind domain.TerminalKind) (domain.TerminalSession, error)
}

// Screen is a live, attached terminal: bytes in (keystrokes), a rendered
// frame out, and a channel that ticks whenever the frame changed. Updates
// closes when the underlying session ends.
type Screen interface {
	Write(p []byte) error
	Render() string
	Resize(w, h int)
	Updates() <-chan struct{}
	Close()
}

// Attacher is the port that connects a Screen to a session.
type Attacher interface {
	Attach(s domain.TerminalSession, w, h int) (Screen, error)
}

type Terminals struct {
	dir TerminalDirectory
	att Attacher
}

func NewTerminals(dir TerminalDirectory, att Attacher) *Terminals {
	return &Terminals{dir: dir, att: att}
}

// Tabs returns the worktree's terminal sessions in creation order,
// creating a plain shell first if it has none.
func (t *Terminals) Tabs(ctx context.Context, wt domain.Worktree) ([]domain.TerminalSession, error) {
	sessions, err := t.dir.Sessions(ctx, wt.RepoID, wt.ID)
	if err != nil || len(sessions) > 0 {
		return sessions, err
	}
	s, err := t.dir.Create(ctx, wt.RepoID, wt.ID, domain.ShellTerminal)
	if err != nil {
		return nil, err
	}
	return []domain.TerminalSession{s}, nil
}

// Add creates a new terminal of the given kind and returns the refreshed
// tab list plus the new tab's index.
func (t *Terminals) Add(ctx context.Context, wt domain.Worktree, kind domain.TerminalKind) ([]domain.TerminalSession, int, error) {
	created, err := t.dir.Create(ctx, wt.RepoID, wt.ID, kind)
	if err != nil {
		return nil, 0, err
	}
	sessions, err := t.dir.Sessions(ctx, wt.RepoID, wt.ID)
	if err != nil {
		return nil, 0, err
	}
	for i, s := range sessions {
		if s.ID == created.ID {
			return sessions, i, nil
		}
	}
	return append(sessions, created), len(sessions), nil
}

func (t *Terminals) Attach(s domain.TerminalSession, w, h int) (Screen, error) {
	return t.att.Attach(s, w, h)
}
