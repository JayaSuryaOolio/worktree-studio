// Package ui is the Bubble Tea presentation layer. It depends on app and
// domain only; it never touches HTTP.
package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"worktree-studio/tui/app"
	"worktree-studio/tui/domain"
)

var (
	activeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("238"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	attnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	dirtyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type (
	// openMsg asks the root to attach this worktree's terminal.
	openMsg  struct{ wt domain.Worktree }
	reposMsg []domain.Repo
	itemsMsg struct {
		repo  domain.RepoID
		items []domain.SidebarItem
	}
	attentionMsg domain.AttentionEvent
	errMsg       struct{ err error }
)

// SidebarModel is the worktree sidebar: repo header, worktree rows with
// git/attention badges, j/k navigation. It is a prototype of the browser
// Sidebar.tsx, minus spotlight and the expandable row actions.
type SidebarModel struct {
	ctx       context.Context
	uc        *app.Sidebar
	nw        *app.NewWorktree
	dialog    *newWorktreeDialog
	feed      <-chan domain.AttentionEvent
	repos     []domain.Repo
	repoIdx   int
	items     []domain.SidebarItem
	cursor    int
	attention domain.Attention
	err       error
	width     int
	height    int
}

func NewSidebar(ctx context.Context, uc *app.Sidebar, nw *app.NewWorktree, feed app.AttentionFeed) SidebarModel {
	return SidebarModel{ctx: ctx, uc: uc, nw: nw, feed: feed.Subscribe(ctx), attention: domain.Attention{}}
}

func (m SidebarModel) Init() tea.Cmd {
	return tea.Batch(m.loadRepos(), m.waitAttention())
}

func (m SidebarModel) loadRepos() tea.Cmd {
	return func() tea.Msg {
		repos, err := m.uc.Repos(m.ctx)
		if err != nil {
			return errMsg{err}
		}
		return reposMsg(repos)
	}
}

func (m SidebarModel) loadItems() tea.Cmd {
	if len(m.repos) == 0 {
		return nil
	}
	repo := m.repos[m.repoIdx].ID
	return func() tea.Msg {
		items, err := m.uc.Load(m.ctx, repo)
		if err != nil {
			return errMsg{err}
		}
		return itemsMsg{repo, items}
	}
}

func (m SidebarModel) waitAttention() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.feed
		if !ok {
			return nil
		}
		return attentionMsg(ev)
	}
}

func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	if m.dialog != nil {
		switch msg.(type) {
		case tea.KeyMsg, draftMsg, createdMsg:
			if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+c" {
				return m, tea.Quit
			}
			cmd, done, created := m.dialog.update(msg)
			if done {
				m.dialog = nil
			}
			if created {
				cmd = tea.Batch(cmd, m.loadItems())
			}
			return m, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case reposMsg:
		m.repos, m.err = msg, nil
		return m, m.loadItems()
	case itemsMsg:
		if len(m.repos) == 0 || msg.repo != m.repos[m.repoIdx].ID {
			return m, nil // stale response after switching repos
		}
		m.items, m.err = msg.items, nil
		m.cursor = min(m.cursor, max(len(m.items)-1, 0))
	case attentionMsg:
		m.attention = m.attention.Apply(domain.AttentionEvent(msg))
		return m, m.waitAttention()
	case errMsg:
		m.err = msg.err
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m SidebarModel) onKey(k tea.KeyMsg) (SidebarModel, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor = min(m.cursor+1, max(len(m.items)-1, 0))
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "r":
		return m, m.loadItems()
	case "n":
		if len(m.repos) > 0 {
			var cmd tea.Cmd
			m.dialog, cmd = openNewWorktree(m.ctx, m.nw, m.repos[m.repoIdx].ID)
			return m, cmd
		}
	case "]", "[":
		if n := len(m.repos); n > 1 {
			d := 1
			if k.String() == "[" {
				d = n - 1
			}
			m.repoIdx, m.cursor, m.items = (m.repoIdx+d)%n, 0, nil
			return m, m.loadItems()
		}
	case "enter":
		if len(m.items) > 0 {
			wt := m.items[m.cursor].Worktree
			markSeen := func() tea.Msg {
				if err := m.uc.MarkSeen(m.ctx, wt); err != nil {
					return errMsg{err}
				}
				return nil
			}
			return m, tea.Batch(markSeen, func() tea.Msg { return openMsg{wt} })
		}
	}
	return m, nil
}

// Selected is the worktree under the cursor.
func (m SidebarModel) Selected() (domain.Worktree, bool) {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return domain.Worktree{}, false
	}
	return m.items[m.cursor].Worktree, true
}

// DialogView is the open modal's box, or "" — the root centres it over the
// whole screen, since it is wider than the sidebar column.
func (m SidebarModel) DialogView() string {
	if m.dialog == nil {
		return ""
	}
	return m.dialog.view()
}

func (m SidebarModel) View() string {
	var b strings.Builder
	switch {
	case len(m.repos) == 0 && m.err == nil:
		return dimStyle.Render("loading…")
	case len(m.repos) == 0:
		return errStyle.Render(m.err.Error()) + "\n"
	}
	repo := m.repos[m.repoIdx]
	b.WriteString(titleStyle.Render(fmt.Sprintf("%s (%d/%d)", repo.Name, m.repoIdx+1, len(m.repos))) + "\n\n")
	w := m.width
	if w <= 0 {
		w = 40
	}
	start, end := m.window()
	for i := start; i < end; i++ {
		line := m.row(m.items[i], w)
		if i == m.cursor {
			line = activeStyle.Width(w).Render(line)
		}
		b.WriteString(line + "\n")
	}
	if len(m.items) == 0 {
		hint := "no active worktrees"
		if len(m.repos) > 1 {
			hint += " — press ] for the next repo"
		}
		b.WriteString(dimStyle.Render(hint) + "\n")
	}
	if m.err != nil {
		b.WriteString("\n" + errStyle.Render(m.err.Error()) + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("j/k move  [/] repo  n new  enter open  c claude  s shell  tab next-tab  r refresh  q quit"))
	return b.String()
}

// row renders "<pin><branch…>   ↑1↓2 ● *", with the branch middle-truncated
// so the distinguishing tail survives (same idea as branchLabel.ts).
func (m SidebarModel) row(it domain.SidebarItem, w int) string {
	var badges []string
	if it.Git != nil {
		if t := it.Git.Ticks(); t != "" {
			badges = append(badges, dimStyle.Render(t))
		}
		if it.Git.Dirty {
			badges = append(badges, dirtyStyle.Render("*"))
		}
	}
	if _, ok := m.attention[it.Worktree.ID]; ok {
		badges = append(badges, attnStyle.Render("●"))
	}
	meta := strings.Join(badges, " ")
	prefix := "  "
	if it.Worktree.Pinned {
		prefix = "⚑ "
	}
	room := w - lipgloss.Width(prefix) - lipgloss.Width(meta) - 1
	return prefix + padRight(midTruncate(it.Worktree.Branch, room), room) + " " + meta
}

// window returns the slice of rows that fits the terminal (header and
// footer take 5 lines), scrolled to keep the cursor visible.
func (m SidebarModel) window() (int, int) {
	room := m.height - 5
	if m.height <= 0 || room >= len(m.items) {
		return 0, len(m.items)
	}
	room = max(room, 1)
	start := min(max(m.cursor-room/2, 0), len(m.items)-room)
	return start, start + room
}

func midTruncate(s string, n int) string {
	r := []rune(s)
	if n <= 1 || len(r) <= n {
		return s
	}
	head := (n - 1) / 3
	return string(r[:head]) + "…" + string(r[len(r)-(n-1-head):])
}

func padRight(s string, n int) string {
	if d := n - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
