package app

import (
	"context"

	"worktree-studio/tui/domain"
)

// TerminalDirectory is the port to the server's terminal-session registry.
type TerminalDirectory interface {
	Sessions(ctx context.Context, repo domain.RepoID, wt domain.WorktreeID) ([]domain.TerminalSession, error)
	Create(ctx context.Context, repo domain.RepoID, wt domain.WorktreeID, label string) (domain.TerminalSession, error)
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

// Open attaches to the worktree's first terminal session, creating one if
// the worktree has none yet.
func (t *Terminals) Open(ctx context.Context, wt domain.Worktree, w, h int) (Screen, error) {
	sessions, err := t.dir.Sessions(ctx, wt.RepoID, wt.ID)
	if err != nil {
		return nil, err
	}
	var s domain.TerminalSession
	if len(sessions) > 0 {
		s = sessions[0]
	} else if s, err = t.dir.Create(ctx, wt.RepoID, wt.ID, "shell"); err != nil {
		return nil, err
	}
	return t.att.Attach(s, w, h)
}
