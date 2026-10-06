package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"worktree-studio/tui/app"
	"worktree-studio/tui/domain"
)

const (
	sidebarWidth  = 38
	frameInterval = 16 * time.Millisecond
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

type focus int

const (
	focusSidebar focus = iota
	focusTerminal
)

// Root composes sidebar, tab bar, pane and status bar, and owns the keymap
// (tui/DESIGN.md): alt combos and ctrl+space are the app's; every other key
// goes to whatever has focus.
type Root struct {
	ctx       context.Context
	sidebar   SidebarModel
	terminals *app.Terminals
	screen    app.Screen
	wt        *domain.Worktree // worktree whose tabs are shown
	tabs      []domain.TerminalSession
	active    int
	focus     focus
	leader    bool // ctrl+space menu is open
	width     int
	height    int
	status    string
}

func NewRoot(ctx context.Context, sidebar SidebarModel, terminals *app.Terminals) Root {
	sidebar.focused = true
	return Root{ctx: ctx, sidebar: sidebar, terminals: terminals}
}

func (r Root) Init() tea.Cmd { return r.sidebar.Init() }

// paneSize is the terminal's size: the main column minus the tab bar, the
// pane header and the status bar.
func (r Root) paneSize() (int, int) {
	return max(r.width-sidebarWidth-1, 10), max(r.height-3, 1)
}

func (r Root) setFocus(f focus) Root {
	if f == focusTerminal && r.screen == nil {
		return r
	}
	r.focus = f
	r.sidebar.focused = f == focusSidebar
	return r
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

// target is the worktree a "new tab" applies to: the sidebar's selection
// while it has focus, otherwise the one on screen.
func (r Root) target() (domain.Worktree, bool) {
	if r.focus == focusTerminal && r.wt != nil {
		return *r.wt, true
	}
	return r.sidebar.Selected()
}

func (r Root) do(act action, key string) (Root, tea.Cmd) {
	switch act {
	case actFocusSidebar:
		r = r.setFocus(focusSidebar)
		return r, r.sidebar.Refresh()
	case actFocusPanes:
		return r.setFocus(focusTerminal), nil
	case actTab:
		n := int(key[len(key)-1] - '0')
		if n > len(r.tabs) {
			return r, nil
		}
		if n-1 == r.active && r.screen != nil {
			return r.setFocus(focusTerminal), nil
		}
		r.active = n - 1
		return r.show()
	case actLeader:
		r.leader = true
	case actNewClaude, actNewShell:
		wt, ok := r.target()
		if !ok {
			return r, nil
		}
		kind := domain.ClaudeTerminal
		if act == actNewShell {
			kind = domain.ShellTerminal
		}
		r.status = "creating…"
		return r, r.loadTabs(wt, &kind)
	case actNewWorktree:
		var cmd tea.Cmd
		r.sidebar, cmd = r.sidebar.NewWorktree()
		return r, cmd
	case actQuit:
		return r, tea.Quit
	case actUp, actDown:
		d := 1
		if act == actUp {
			d = -1
		}
		r.sidebar = r.sidebar.Move(d)
	case actExpand:
		var cmd tea.Cmd
		var handled bool
		if r.sidebar, cmd, handled = r.sidebar.Expand(); handled {
			return r, cmd
		}
		return r.do(actOpen, key) // → on a worktree goes to its terminal
	case actCollapse:
		r.sidebar = r.sidebar.Collapse()
	case actClearFilter:
		r.sidebar = r.sidebar.ClearFilter()
	case actBackspace:
		r.sidebar = r.sidebar.Type("", true)
	case actOpen:
		if wt, ok := r.sidebar.Selected(); ok && r.wt != nil && r.wt.ID == wt.ID && r.screen != nil {
			return r.setFocus(focusTerminal), nil // already on screen
		}
		var cmd tea.Cmd
		r.sidebar, cmd = r.sidebar.Open()
		return r, cmd
	}
	return r, nil
}

func (r Root) onKey(k tea.KeyMsg) (Root, tea.Cmd) {
	key := k.String()
	if r.leader {
		r.leader = false
		if act, ok := lookup(leaderKeys, key); ok {
			return r.do(act, key)
		}
		return r, nil
	}
	if act, ok := lookup(globalKeys, key); ok {
		return r.do(act, key)
	}
	if r.focus == focusTerminal {
		if b := keyBytes(k); b != nil {
			_ = r.screen.Write(b)
		}
		return r, nil
	}
	if act, ok := lookup(sidebarKeys, key); ok {
		return r.do(act, key)
	}
	if (k.Type == tea.KeyRunes || k.Type == tea.KeySpace) && !k.Alt {
		r.sidebar = r.sidebar.Type(string(k.Runes), false)
	}
	return r, nil
}

func (r Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = msg.Width, msg.Height
		var cmd tea.Cmd
		r.sidebar, cmd = r.sidebar.Update(tea.WindowSizeMsg{Width: sidebarWidth, Height: msg.Height - 1})
		if r.screen != nil {
			r.screen.Resize(r.paneSize())
		}
		return r, cmd
	case openMsg:
		if r.screen != nil {
			r.screen.Close()
			r.screen = nil
		}
		wt := msg.wt
		r.wt, r.tabs, r.active, r.status = &wt, nil, 0, "attaching…"
		return r, r.loadTabs(wt, nil)
	case tabsMsg:
		if msg.err != nil {
			r.status = msg.err.Error()
			return r, nil
		}
		wt := msg.wt
		r.wt, r.tabs, r.active = &wt, msg.tabs, msg.active
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
		r.screen, r.status = msg.screen, ""
		return r.setFocus(focusTerminal), waitFrame(msg.screen)
	case frameMsg:
		if msg.screen != r.screen {
			return r, nil // stale: we've since switched tabs
		}
		return r, waitFrame(msg.screen)
	case closedMsg:
		if msg.screen == r.screen {
			r.screen, r.status = nil, "terminal session ended"
			r = r.setFocus(focusSidebar)
		}
		return r, nil
	case tea.KeyMsg:
		if r.sidebar.dialog == nil {
			return r.onKey(msg)
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
	if r.leader {
		r.sidebar.focused = false // the menu holds the accent while open
	}
	pw, ph := r.paneSize()
	bodyH := ph + 2 // tab bar + pane header + terminal
	side := lipgloss.NewStyle().Width(sidebarWidth).Height(bodyH).MaxHeight(bodyH).Render(r.sidebar.View())
	sep := ruleStyle.Render(repeatLine("│", bodyH))

	var main string
	switch {
	case r.screen != nil:
		main = r.screen.Render()
	case r.status != "":
		main = dimStyle.Render(r.status)
	default:
		main = dimStyle.Render("enter on a worktree opens its terminals")
	}
	pane := lipgloss.NewStyle().Width(pw).Height(ph).MaxWidth(pw).MaxHeight(ph).Render(main)
	if r.leader {
		pane = overlayBottomRight(pane, r.leaderMenu(), pw, ph)
	}
	cut := lipgloss.NewStyle().MaxWidth(pw)
	column := cut.Render(r.tabBar()) + "\n" + cut.Render(r.paneHeader(pw)) + "\n" + pane
	body := lipgloss.JoinHorizontal(lipgloss.Top, side, sep, column)
	return body + "\n" + r.statusBar()
}

func (r Root) tabBar() string {
	if len(r.tabs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, t := range r.tabs {
		label := " " + strconv.Itoa(i+1) + " " + t.Label + " "
		if i == r.active {
			b.WriteString(selectedStyle.Render(label))
		} else {
			b.WriteString(mutedStyle.Render(label))
		}
	}
	return b.String()
}

// paneHeader is the pane's title rule; it turns accent when keys go to the
// pane, so it's the one place on screen that says "you're typing here".
func (r Root) paneHeader(w int) string {
	if r.active >= len(r.tabs) {
		return ruleStyle.Render(strings.Repeat("─", w))
	}
	title := " " + r.tabs[r.active].Label + " "
	fill := max(w-lipgloss.Width(title)-1, 0)
	if r.focus == focusTerminal && !r.leader {
		return accentStyle.Render("━" + title + strings.Repeat("━", fill))
	}
	return ruleStyle.Render("─") + mutedStyle.Render(title) + ruleStyle.Render(strings.Repeat("─", fill))
}

func (r Root) mode() (string, string) {
	switch {
	case r.leader:
		return "MENU", dimStyle.Render("press a key from the menu")
	case r.focus == focusTerminal:
		return "TERMINAL", hints(globalKeys)
	default:
		return "SIDEBAR", hints(sidebarKeys) + "  " + hints(globalKeys)
	}
}

func (r Root) statusBar() string {
	name, keys := r.mode()
	left := chipStyle.Render(name) + " " + keys
	right := ""
	if n := r.sidebar.NeedYou(); n > 0 {
		right = attnStyle.Render("● ") + textStyle.Render(strconv.Itoa(n)+" need you") + " "
	}
	gap := r.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left, r.width, "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

func (r Root) leaderMenu() string {
	var b strings.Builder
	for i, k := range leaderKeys {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(accentStyle.Render(padRight(k.label, 4)) + textStyle.Render(k.help))
	}
	return menuStyle.Render(dimStyle.Render("ctrl+space") + "\n" + b.String())
}

// overlayBottomRight draws box over the bottom-right corner of base (w×h),
// keeping base's styling on both sides of it.
func overlayBottomRight(base, box string, w, h int) string {
	lines := strings.Split(base, "\n")
	boxLines := strings.Split(box, "\n")
	bw := lipgloss.Width(box)
	x, y := max(w-bw-1, 0), max(h-len(boxLines)-1, 0)
	for i, bl := range boxLines {
		if y+i >= len(lines) {
			break
		}
		l := lines[y+i]
		left := padRight(ansi.Truncate(l, x, ""), x)
		right := ansi.TruncateLeft(l, x+bw, "")
		lines[y+i] = left + "\x1b[0m" + bl + right
	}
	return strings.Join(lines, "\n")
}

func repeatLine(s string, n int) string {
	out := s
	for i := 1; i < n; i++ {
		out += "\n" + s
	}
	return out
}
