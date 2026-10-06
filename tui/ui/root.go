package ui

import (
	"context"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"worktree-studio/tui/app"
	"worktree-studio/tui/domain"
)

const (
	sidebarWidth  = 38
	frameInterval = 16 * time.Millisecond
	// escapeKey leaves the terminal pane; every other key goes to the shell.
	escapeKey = "ctrl+]"
)

type (
	// tabsMsg carries a worktree's terminal sessions and which one to show.
	tabsMsg struct {
		wt     domain.Worktree
		tabs   []domain.TerminalSession
		active int
		err    error
	}
	screenMsg struct {
		session domain.TerminalSession
		screen  app.Screen
		err     error
	}
	frameMsg  struct{ screen app.Screen }
	closedMsg struct{ screen app.Screen }
)

var accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))

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
	tabs      []domain.TerminalSession
	active    int
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
	return max(r.width-sidebarWidth-1, 10), max(r.height-1, 1) // 1 row for the tab bar
}

// show attaches the active tab, dropping any live screen first.
func (r Root) show() (Root, tea.Cmd) {
	if r.screen != nil {
		r.screen.Close()
		r.screen = nil
	}
	if r.active >= len(r.tabs) {
		return r, nil
	}
	r.status = "attaching…"
	w, h := r.paneSize()
	sess := r.tabs[r.active]
	return r, func() tea.Msg {
		s, err := r.terminals.Attach(sess, w, h)
		return screenMsg{sess, s, err}
	}
}

func (r Root) loadTabs(wt domain.Worktree, kind *domain.TerminalKind) tea.Cmd {
	return func() tea.Msg {
		if kind == nil {
			tabs, err := r.terminals.Tabs(r.ctx, wt)
			return tabsMsg{wt, tabs, 0, err}
		}
		tabs, i, err := r.terminals.Add(r.ctx, wt, *kind)
		return tabsMsg{wt, tabs, i, err}
	}
}

// command handles tab keys while the sidebar has focus; ok=false means the
// key belongs to the sidebar.
func (r Root) command(key string) (Root, tea.Cmd, bool) {
	if r.sidebar.dialog != nil {
		return r, nil, false
	}
	switch key {
	case "c", "s":
		wt, ok := r.sidebar.Selected()
		if !ok {
			return r, nil, true
		}
		kind := domain.ClaudeTerminal
		if key == "s" {
			kind = domain.ShellTerminal
		}
		r.status = "creating…"
		return r, r.loadTabs(wt, &kind), true
	case "tab", "shift+tab":
		if len(r.tabs) > 1 {
			d := 1
			if key == "shift+tab" {
				d = len(r.tabs) - 1
			}
			r.active = (r.active + d) % len(r.tabs)
			r, cmd := r.show()
			return r, cmd, true
		}
		return r, nil, true
	}
	if n, err := strconv.Atoi(key); err == nil && n >= 1 && n <= len(r.tabs) {
		r.active = n - 1
		r, cmd := r.show()
		return r, cmd, true
	}
	return r, nil, false
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
		r.tabs, r.active, r.status = nil, 0, "attaching…"
		return r, r.loadTabs(msg.wt, nil)
	case tabsMsg:
		if msg.err != nil {
			r.status = msg.err.Error()
			return r, nil
		}
		r.tabs, r.active = msg.tabs, msg.active
		return r.show()
	case screenMsg:
		if msg.err != nil {
			r.status = msg.err.Error()
			return r, nil
		}
		if r.active >= len(r.tabs) || r.tabs[r.active].ID != msg.session.ID || r.screen != nil {
			msg.screen.Close() // stale: we've since switched tabs
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
		if nr, cmd, ok := r.command(msg.String()); ok {
			return nr, cmd
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
	sh := ph + 1 // sidebar and separator also span the tab-bar row
	side := lipgloss.NewStyle().Width(sidebarWidth).Height(sh).MaxHeight(sh).Render(r.sidebar.View())

	sepColor := lipgloss.Color("238")
	if r.focus == focusTerminal {
		sepColor = lipgloss.Color("111")
	}
	sep := lipgloss.NewStyle().Foreground(sepColor).Render(repeatLine("│", sh))

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
	pane = lipgloss.NewStyle().MaxWidth(pw).Render(r.tabBar()) + "\n" + pane
	return lipgloss.JoinHorizontal(lipgloss.Top, side, sep, pane)
}

func (r Root) tabBar() string {
	if len(r.tabs) == 0 {
		return dimStyle.Render("no terminal")
	}
	out := ""
	for i, t := range r.tabs {
		label := " " + strconv.Itoa(i+1) + " " + t.Label + " "
		if i == r.active {
			label = lipgloss.NewStyle().Reverse(true).Render(label)
		} else {
			label = dimStyle.Render(label)
		}
		out += label
	}
	// Say where keys go: while the pane has focus even c/s/j/k are typed
	// into the terminal, so the way out must always be on screen.
	if r.focus == focusTerminal && r.screen != nil {
		return out + accentStyle.Render("  ● typing into terminal · "+escapeKey+" for sidebar keys")
	}
	return out + dimStyle.Render("  sidebar has keys · 1-9 to type in a tab")
}

func repeatLine(s string, n int) string {
	out := s
	for i := 1; i < n; i++ {
		out += "\n" + s
	}
	return out
}
