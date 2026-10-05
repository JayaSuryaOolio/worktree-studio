package ui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"worktree-studio/tui/app"
	"worktree-studio/tui/domain"
	"worktree-studio/tui/infra/ptyterm"
)

type oneSession struct{ name string }

func (o oneSession) Sessions(context.Context, domain.RepoID, domain.WorktreeID) ([]domain.TerminalSession, error) {
	return []domain.TerminalSession{{TmuxName: o.name}}, nil
}
func (o oneSession) Create(context.Context, domain.RepoID, domain.WorktreeID, string) (domain.TerminalSession, error) {
	panic("unexpected create")
}

// Drives Root end to end: open a worktree, see its terminal beside the
// sidebar, type into it, and get back to the sidebar with the escape key.
func TestRootOpensTerminalBesideSidebar(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	const name = "wts-tui-root-test"
	exec.Command("tmux", "kill-session", "-t", name).Run()
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-x", "80", "-y", "12", "cat").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	t.Cleanup(func() { exec.Command("tmux", "kill-session", "-t", name).Run() })

	var m tea.Model = NewRoot(context.Background(), SidebarModel{}, app.NewTerminals(oneSession{name}, ptyterm.Attacher{}))
	step := func(msg tea.Msg) tea.Cmd {
		var c tea.Cmd
		m, c = m.Update(msg)
		return c
	}
	step(tea.WindowSizeMsg{Width: 100, Height: 12})
	cmd := step(openMsg{domain.Worktree{ID: "w", RepoID: "r"}})
	sm := cmd().(screenMsg)
	if sm.err != nil {
		t.Fatal(sm.err)
	}
	step(sm)

	for _, r := range "hi-claude\r" {
		step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if r == '\r' {
			step(tea.KeyMsg{Type: tea.KeyEnter})
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(m.View(), "hi-claude") {
		if time.Now().After(deadline) {
			t.Fatalf("typed text never reached the pane:\n%s", m.View())
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Log("\n" + m.View())

	step(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if m.(Root).focus != focusSidebar {
		t.Fatal("escape key must return focus to the sidebar")
	}
	m.(Root).screen.Close()
}
