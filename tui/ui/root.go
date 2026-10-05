package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"worktree-studio/tui/app"
)

const (
	sidebarWidth  = 38
	frameInterval = 16 * time.Millisecond
	// escapeKey leaves the terminal pane; every other key goes to the shell.
	escapeKey = "ctrl+]"
)

type (
	screenMsg struct {
		screen app.Screen
		err    error
	}
	frameMsg  struct{ screen app.Screen }
	closedMsg struct{ screen app.Screen }
)

type focus int

const (
	focusSidebar focus = iota
	focusTerminal
)

// Root composes the sidebar and the main terminal pane and routes keys.
type Root struct {
	ctx       context.Context
	sidebar   SidebarModel
	terminals *app.Terminals
	screen    app.Screen
	focus     focus
	width     int
	height    int
	status    string
}

func NewRoot(ctx context.Context, sidebar SidebarModel, terminals *app.Terminals) Root {
	return Root{ctx: ctx, sidebar: sidebar, terminals: terminals}
}

func (r Root) Init() tea.Cmd { return r.sidebar.Init() }

func (r Root) paneSize() (int, int) {
	return max(r.width-sidebarWidth-1, 10), max(r.height, 1)
}

func (r Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = msg.Width, msg.Height
		var cmd tea.Cmd
		r.sidebar, cmd = r.sidebar.Update(tea.WindowSizeMsg{Width: sidebarWidth, Height: msg.Height})
		if r.screen != nil {
			r.screen.Resize(r.paneSize())
		}
		return r, cmd
	case openMsg:
		if r.screen != nil {
			r.screen.Close()
			r.screen = nil
		}
		r.status = "attaching…"
		w, h := r.paneSize()
		wt := msg.wt
		return r, func() tea.Msg {
			s, err := r.terminals.Open(r.ctx, wt, w, h)
			return screenMsg{s, err}
		}
	case screenMsg:
		if msg.err != nil {
			r.status = msg.err.Error()
			return r, nil
		}
		r.screen, r.status, r.focus = msg.screen, "", focusTerminal
		return r, waitFrame(msg.screen)
	case frameMsg:
		if msg.screen != r.screen {
			return r, nil // stale: we've since switched worktrees
		}
		return r, waitFrame(msg.screen)
	case closedMsg:
		if msg.screen == r.screen {
			r.screen, r.focus, r.status = nil, focusSidebar, "terminal session ended"
		}
		return r, nil
	case tea.KeyMsg:
		if r.focus == focusTerminal && r.screen != nil && r.sidebar.dialog == nil {
			if msg.String() == escapeKey {
				r.focus = focusSidebar
			} else if b := keyBytes(msg); b != nil {
				_ = r.screen.Write(b)
			}
			return r, nil
		}
	}
	var cmd tea.Cmd
	r.sidebar, cmd = r.sidebar.Update(msg)
	return r, cmd
}

// waitFrame blocks until the screen has new output, then waits one frame so
// bursts coalesce into a single redraw.
func waitFrame(s app.Screen) tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-s.Updates(); !ok {
			return closedMsg{s}
		}
		time.Sleep(frameInterval)
		return frameMsg{s}
	}
}

func (r Root) View() string {
	if r.width == 0 {
		return ""
	}
	if d := r.sidebar.DialogView(); d != "" {
		return lipgloss.Place(r.width, r.height, lipgloss.Center, lipgloss.Center, d)
	}
	pw, ph := r.paneSize()
	side := lipgloss.NewStyle().Width(sidebarWidth).Height(ph).MaxHeight(ph).Render(r.sidebar.View())

	sepColor := lipgloss.Color("238")
	if r.focus == focusTerminal {
		sepColor = lipgloss.Color("111")
	}
	sep := lipgloss.NewStyle().Foreground(sepColor).Render(repeatLine("│", ph))

	var main string
	switch {
	case r.screen != nil:
		main = r.screen.Render()
	case r.status != "":
		main = dimStyle.Render(r.status)
	default:
		main = dimStyle.Render("enter on a worktree opens its terminal  ·  " + escapeKey + " returns here")
	}
	pane := lipgloss.NewStyle().Width(pw).Height(ph).MaxWidth(pw).MaxHeight(ph).Render(main)
	return lipgloss.JoinHorizontal(lipgloss.Top, side, sep, pane)
}

func repeatLine(s string, n int) string {
	out := s
	for i := 1; i < n; i++ {
		out += "\n" + s
	}
	return out
}
