// Package ui is the Bubble Tea presentation layer. It depends on app and
// domain only; it never touches HTTP.
package ui

import (
	"context"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"worktree-studio/tui/app"
	"worktree-studio/tui/domain"
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

// sidebarRow is one visible line of the tree: a repo, or one of its
// worktrees (item != nil).
type sidebarRow struct {
	repo domain.Repo
	item *domain.SidebarItem
}

// sidebarRows flattens repos → worktrees. Without a filter, only expanded
// repos show their worktrees. With one, every repo that has a matching
// worktree is shown open with just the matches, and the rest are hidden.
func sidebarRows(repos []domain.Repo, items map[domain.RepoID][]domain.SidebarItem, expanded map[domain.RepoID]bool, filter string) []sidebarRow {
	q := strings.ToLower(filter)
	var rows []sidebarRow
	for _, r := range repos {
		var kids []sidebarRow
		for i := range items[r.ID] {
			it := &items[r.ID][i]
			hay := strings.ToLower(it.Worktree.Branch + " " + it.Worktree.Name)
			if q == "" || strings.Contains(hay, q) {
				kids = append(kids, sidebarRow{repo: r, item: it})
			}
		}
		if q != "" && len(kids) == 0 {
			continue
		}
		rows = append(rows, sidebarRow{repo: r})
		if q != "" || expanded[r.ID] {
			rows = append(rows, kids...)
		}
	}
	return rows
}

// SidebarModel is the repo → worktree tree with git/attention badges. It is
// a prototype of the browser Sidebar.tsx, minus spotlight and the
// expandable row actions. Keys arrive from Root's keymap.
type SidebarModel struct {
	ctx        context.Context
	uc         *app.Sidebar
	nw         *app.NewWorktree
	dialog     *newWorktreeDialog
	dialogRepo domain.RepoID
	feed       <-chan domain.AttentionEvent
	repos      []domain.Repo
	items      map[domain.RepoID][]domain.SidebarItem
	expanded   map[domain.RepoID]bool
	filter     string
	cursor     int
	focused    bool
	attention  domain.Attention
	err        error
	width      int
	height     int
}

func NewSidebar(ctx context.Context, uc *app.Sidebar, nw *app.NewWorktree, feed app.AttentionFeed) SidebarModel {
	return SidebarModel{
		ctx: ctx, uc: uc, nw: nw, feed: feed.Subscribe(ctx), attention: domain.Attention{},
		items: map[domain.RepoID][]domain.SidebarItem{}, expanded: map[domain.RepoID]bool{},
	}
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

// loadItems fetches a repo's worktrees; withGit adds the per-row git status,
// which is only worth paying for once the repo is expanded.
func (m SidebarModel) loadItems(repo domain.RepoID, withGit bool) tea.Cmd {
	return func() tea.Msg {
		load := m.uc.Active
		if withGit {
			load = m.uc.Load
		}
		items, err := load(m.ctx, repo)
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
				cmd = tea.Batch(cmd, m.loadItems(m.dialogRepo, true))
			}
			return m, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case reposMsg:
		m.repos, m.err = msg, nil
		cmds := make([]tea.Cmd, len(m.repos))
		for i, r := range m.repos {
			cmds[i] = m.loadItems(r.ID, false)
		}
		return m, tea.Batch(cmds...)
	case itemsMsg:
		m.items[msg.repo], m.err = msg.items, nil
		m.cursor = min(m.cursor, max(len(m.rows())-1, 0))
	case attentionMsg:
		m.attention = m.attention.Apply(domain.AttentionEvent(msg))
		return m, m.waitAttention()
	case errMsg:
		m.err = msg.err
	}
	return m, nil
}

func (m SidebarModel) rows() []sidebarRow {
	return sidebarRows(m.repos, m.items, m.expanded, m.filter)
}

func (m SidebarModel) current() (sidebarRow, bool) {
	rows := m.rows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return sidebarRow{}, false
	}
	return rows[m.cursor], true
}

func (m SidebarModel) Move(d int) SidebarModel {
	m.cursor = min(max(m.cursor+d, 0), max(len(m.rows())-1, 0))
	return m
}

// Expand opens the repo under the cursor (loading its git status), or steps
// into its first worktree if it's already open. It reports false when the
// cursor is on a worktree, so Root can treat → as "go to the terminal".
func (m SidebarModel) Expand() (SidebarModel, tea.Cmd, bool) {
	row, ok := m.current()
	if !ok || row.item != nil {
		return m, nil, false
	}
	if !m.expanded[row.repo.ID] {
		m.expanded[row.repo.ID] = true
		return m, m.loadItems(row.repo.ID, true), true
	}
	return m.Move(1), nil, true
}

// Collapse closes the repo under the cursor, or jumps from a worktree to
// its repo row.
func (m SidebarModel) Collapse() SidebarModel {
	row, ok := m.current()
	if !ok {
		return m
	}
	if row.item == nil {
		delete(m.expanded, row.repo.ID)
		return m
	}
	for i, r := range m.rows() {
		if r.item == nil && r.repo.ID == row.repo.ID {
			m.cursor = i
		}
	}
	return m
}

// Type appends to the filter; "" with backspace=true drops the last rune.
func (m SidebarModel) Type(s string, backspace bool) SidebarModel {
	if backspace {
		r := []rune(m.filter)
		if len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
	} else {
		m.filter += s
	}
	m.cursor = 0
	for i, r := range m.rows() {
		if r.item != nil { // land on the first match, not its repo header
			m.cursor = i
			break
		}
	}
	return m
}

func (m SidebarModel) ClearFilter() SidebarModel {
	m.filter, m.cursor = "", 0
	return m
}

// Refresh reloads git status for every expanded repo.
func (m SidebarModel) Refresh() tea.Cmd {
	var cmds []tea.Cmd
	for id := range m.expanded {
		cmds = append(cmds, m.loadItems(id, true))
	}
	return tea.Batch(cmds...)
}

// Open marks the selected worktree seen and asks Root to show its tabs; on
// a repo row it toggles the repo instead.
func (m SidebarModel) Open() (SidebarModel, tea.Cmd) {
	row, ok := m.current()
	if !ok {
		return m, nil
	}
	if row.item == nil {
		if m.expanded[row.repo.ID] {
			return m.Collapse(), nil
		}
		m, cmd, _ := m.Expand()
		return m, cmd
	}
	wt := row.item.Worktree
	markSeen := func() tea.Msg {
		if err := m.uc.MarkSeen(m.ctx, wt); err != nil {
			return errMsg{err}
		}
		return nil
	}
	return m, tea.Batch(markSeen, func() tea.Msg { return openMsg{wt} })
}

func (m SidebarModel) NewWorktree() (SidebarModel, tea.Cmd) {
	row, ok := m.current()
	if !ok {
		return m, nil
	}
	var cmd tea.Cmd
	m.dialogRepo = row.repo.ID
	m.dialog, cmd = openNewWorktree(m.ctx, m.nw, row.repo.ID)
	return m, cmd
}

// Click puts the cursor on the row drawn at screen line y (below the
// filter line and its gap); false if no row is there.
func (m SidebarModel) Click(y int) (SidebarModel, bool) {
	start, end := m.window(len(m.rows()))
	if i := start + y - 2; y >= 2 && i < end {
		m.cursor = i
		return m, true
	}
	return m, false
}

// Selected is the worktree under the cursor.
func (m SidebarModel) Selected() (domain.Worktree, bool) {
	row, ok := m.current()
	if !ok || row.item == nil {
		return domain.Worktree{}, false
	}
	return row.item.Worktree, true
}

// NeedYou counts worktrees whose Claude is waiting on the user.
func (m SidebarModel) NeedYou() int { return len(m.attention) }

// DialogView is the open modal's box, or "" — the root centres it over the
// whole screen, since it is wider than the sidebar column.
func (m SidebarModel) DialogView() string {
	if m.dialog == nil {
		return ""
	}
	return m.dialog.view()
}

func (m SidebarModel) View() string {
	switch {
	case len(m.repos) == 0 && m.err == nil:
		return dimStyle.Render(" loading…")
	case len(m.repos) == 0:
		return errStyle.Render(m.err.Error()) + "\n"
	}
	w := m.width
	if w <= 0 {
		w = 40
	}
	var b strings.Builder
	if m.filter != "" {
		b.WriteString(" " + mutedStyle.Render("filter ") + textStyle.Render(m.filter) + accentStyle.Render("▏") + "\n\n")
	} else {
		b.WriteString(dimStyle.Render(" type to filter") + "\n\n")
	}
	rows := m.rows()
	start, end := m.window(len(rows))
	for i := start; i < end; i++ {
		line, bar := m.row(rows[i], w-1), " "
		if i == m.cursor {
			line = selectedStyle.Width(w - 1).Render(line)
			if m.focused {
				bar = accentStyle.Render("▌")
			}
		}
		b.WriteString(bar + line + "\n")
	}
	if len(rows) == 0 {
		b.WriteString(dimStyle.Render(" no worktree matches") + "\n")
	}
	if m.err != nil {
		b.WriteString("\n" + errStyle.Render(m.err.Error()) + "\n")
	}
	return b.String()
}

func (m SidebarModel) row(r sidebarRow, w int) string {
	if r.item == nil {
		return m.repoRow(r.repo, w)
	}
	return m.worktreeRow(*r.item, w)
}

// repoRow renders "▾ name   3 ●": the count of active worktrees, and a dot
// if any of them needs you (so collapsed repos still surface attention).
func (m SidebarModel) repoRow(r domain.Repo, w int) string {
	arrow := "▸ "
	if m.expanded[r.ID] || m.filter != "" {
		arrow = "▾ "
	}
	items := m.items[r.ID]
	meta := dimStyle.Render(strconv.Itoa(len(items)))
	for _, it := range items {
		if _, ok := m.attention[it.Worktree.ID]; ok {
			meta += " " + attnStyle.Render("●")
			break
		}
	}
	room := w - 2 - lipgloss.Width(meta) - 1
	return mutedStyle.Render(arrow) + titleStyle.Render(padRight(midTruncate(r.Name, room), room)) + " " + meta
}

// worktreeRow renders "  <pin><branch…>   ↑1↓2 * ●", with the branch
// middle-truncated so the distinguishing tail survives (as branchLabel.ts).
func (m SidebarModel) worktreeRow(it domain.SidebarItem, w int) string {
	var badges []string
	if it.Git != nil {
		if t := it.Git.Ticks(); t != "" {
			badges = append(badges, dimStyle.Render(t))
		}
		if it.Git.Dirty {
			badges = append(badges, mutedStyle.Render("*"))
		}
	}
	if _, ok := m.attention[it.Worktree.ID]; ok {
		badges = append(badges, attnStyle.Render("●"))
	}
	meta := strings.Join(badges, " ")
	prefix := "  "
	if it.Worktree.Pinned {
		prefix = " ⚑"
	}
	room := w - 4 - lipgloss.Width(meta)
	return prefix + " " + padRight(midTruncate(it.Worktree.Branch, room), room) + " " + meta
}

// window returns the slice of n rows that fits under the filter line,
// scrolled to keep the cursor visible.
func (m SidebarModel) window(n int) (int, int) {
	room := m.height - 2
	if m.height <= 0 || room >= n {
		return 0, n
	}
	room = max(room, 1)
	start := min(max(m.cursor-room/2, 0), n-room)
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
