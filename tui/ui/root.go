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
	// tabsMsg carries a worktree's terminal sessions. created is a session
	// the user just made: into puts it in the focused empty pane instead of
	// a tab of its own. quiet reloads leave focus where it is.
	tabsMsg struct {
		wt       domain.Worktree
		sessions []domain.TerminalSession
		created  string
		into     bool
		quiet    bool
		err      error
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

// Root composes sidebar, tab bar, panes and status bar, and owns the keymap
// (tui/DESIGN.md): alt combos and ctrl+space are the app's; every other key
// goes to whatever has focus. Each visible pane is its own tmux attach.
type Root struct {
	ctx       context.Context
	sidebar   SidebarModel
	terminals *app.Terminals
	wt        *domain.Worktree // worktree whose tabs are shown
	sessions  []domain.TerminalSession
	bench     domain.Workbench
	benches   map[domain.WorktreeID]domain.Workbench // other worktrees' layouts, restored on return
	screens   map[string]app.Screen                  // by session ID, visible panes only
	attaching map[string]bool
	pick      int // cursor in the empty pane's "what runs here?" list
	focus     focus
	leader    bool     // ctrl+space menu is open
	palette   *palette // ctrl+space space command palette is open
	store     app.LayoutStore
	resizing  bool // resize mode: arrows move the focused pane's divider
	drag      domain.Path
	dragging  bool // a divider is being dragged with the mouse
	width     int
	height    int
	status    string
}

func NewRoot(ctx context.Context, sidebar SidebarModel, terminals *app.Terminals) Root {
	sidebar.focused = true
	return Root{
		ctx: ctx, sidebar: sidebar, terminals: terminals,
		benches: map[domain.WorktreeID]domain.Workbench{}, screens: map[string]app.Screen{}, attaching: map[string]bool{},
	}
}

// WithLayouts restores saved pane layouts from s and saves them back as
// they change.
func (r Root) WithLayouts(s app.LayoutStore) Root {
	r.store = s
	saved, err := s.Load()
	if err != nil {
		r.status = "saved layouts: " + err.Error()
		return r
	}
	for id, b := range saved {
		r.benches[id] = b
	}
	return r
}

// save writes every worktree's layout, this one included.
func (r Root) save() Root {
	if r.store == nil {
		return r
	}
	all := map[domain.WorktreeID]domain.Workbench{}
	for id, b := range r.benches {
		if len(b.Tabs) > 0 {
			all[id] = b
		}
	}
	if r.ready() {
		all[r.wt.ID] = r.bench
	}
	if err := r.store.Save(all); err != nil {
		r.status = "saving layouts: " + err.Error()
	}
	return r
}

// modal is true while a menu holds the accent and the keys.
func (r Root) modal() bool { return r.leader || r.palette != nil }

func (r Root) Init() tea.Cmd { return r.sidebar.Init() }

// area is where panes go: the main column minus the tab bar and status bar.
// Every pane spends its first line on a header.
func (r Root) area() domain.Rect {
	return domain.Rect{W: max(r.width-sidebarWidth-1, 10), H: max(r.height-2, 2)}
}

func (r Root) setFocus(f focus) Root {
	if f == focusTerminal && r.wt == nil {
		return r
	}
	r.focus = f
	r.sidebar.focused = f == focusSidebar
	return r
}

// ready is true once the shown worktree's tabs have loaded.
func (r Root) ready() bool { return r.wt != nil && len(r.bench.Tabs) > 0 }

func (r Root) visible() map[string]domain.Rect {
	if !r.ready() {
		return nil
	}
	return r.bench.Visible(r.area())
}

// sync makes the live screens match the visible panes: detach the hidden,
// resize the moved, attach the new.
func (r Root) sync() (Root, tea.Cmd) {
	r = r.save()
	vis := r.visible()
	for id, s := range r.screens {
		if rect, ok := vis[id]; ok {
			s.Resize(rect.W, rect.H-1)
		} else {
			s.Close()
			delete(r.screens, id)
		}
	}
	var cmds []tea.Cmd
	for id, rect := range vis {
		sess, ok := r.session(id)
		if !ok || r.screens[id] != nil || r.attaching[id] {
			continue
		}
		r.attaching[id] = true
		cmds = append(cmds, func() tea.Msg {
			s, err := r.terminals.Attach(sess, rect.W, rect.H-1)
			return screenMsg{sess, s, err}
		})
	}
	return r, tea.Batch(cmds...)
}

func (r Root) session(id string) (domain.TerminalSession, bool) {
	for _, s := range r.sessions {
		if s.ID == id {
			return s, true
		}
	}
	return domain.TerminalSession{}, false
}

// switchTo shows wt's tabs, parking the current worktree's layout.
func (r Root) switchTo(wt domain.Worktree) Root {
	if r.wt != nil {
		r.benches[r.wt.ID] = r.bench
	}
	r.wt, r.bench, r.sessions = &wt, r.benches[wt.ID], nil
	r, _ = r.sync() // nothing is attachable yet: this only detaches
	return r
}

func (r Root) loadTabs(wt domain.Worktree, kind *domain.TerminalKind, into bool) tea.Cmd {
	return func() tea.Msg {
		if kind == nil {
			s, err := r.terminals.Tabs(r.ctx, wt)
			return tabsMsg{wt: wt, sessions: s, err: err}
		}
		s, i, err := r.terminals.Add(r.ctx, wt, *kind)
		if err != nil {
			return tabsMsg{wt: wt, err: err}
		}
		return tabsMsg{wt: wt, sessions: s, created: s[i].ID, into: into}
	}
}

// reload re-reads the session list without moving focus, e.g. after a
// session ended.
func (r Root) reload() tea.Cmd {
	if r.wt == nil {
		return nil
	}
	wt := *r.wt
	return func() tea.Msg {
		s, err := r.terminals.Tabs(r.ctx, wt)
		return tabsMsg{wt: wt, sessions: s, quiet: true, err: err}
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

// choice is one line of the empty pane's "what runs here?" list: a new
// session of some kind, or an existing one that isn't shown anywhere.
type choice struct {
	kind    *domain.TerminalKind
	session domain.TerminalSession
}

func (r Root) choices() []choice {
	cs := []choice{{kind: &domain.ClaudeTerminal}, {kind: &domain.ShellTerminal}}
	for _, s := range r.bench.Hidden(r.sessions) {
		cs = append(cs, choice{session: s})
	}
	return cs
}

func (r Root) do(act action, key string) (Root, tea.Cmd) {
	switch act {
	case actMove:
		d := arrow(key)
		if r.focus == focusSidebar {
			if d == domain.Right {
				return r.setFocus(focusTerminal), nil
			}
			return r, nil
		}
		if b, ok := r.bench.Move(d, r.area()); ok {
			r.bench = b
			return r.sync()
		}
		if d == domain.Left {
			r = r.setFocus(focusSidebar)
			return r, r.sidebar.Refresh()
		}
	case actTab:
		return r.selectTab(int(key[len(key)-1] - '1'))
	case actResize:
		if !r.ready() {
			return r, nil
		}
		n := 2
		if strings.HasPrefix(key, "shift+") {
			n = 8
		}
		r.bench = r.bench.Resize(arrow(key), n, r.area())
		return r.sync()
	case actResizeMode:
		r.resizing = r.ready()
	case actDone:
		r.resizing = false
	case actLeader:
		r.leader = true
	case actPalette:
		r.palette = &palette{}
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
		return r, r.loadTabs(wt, &kind, false)
	case actSplit, actClosePane, actZoom:
		if !r.ready() {
			return r, nil
		}
		switch act {
		case actSplit:
			r.bench, r.pick = r.bench.Split(arrow(key)), 0
		case actClosePane:
			r.bench = r.bench.ClosePane()
		case actZoom:
			r.bench.Zoom = !r.bench.Zoom
		}
		r = r.setFocus(focusTerminal)
		return r.sync()
	case actPick:
		cs := r.choices()
		if !r.ready() || r.pick >= len(cs) {
			return r, nil
		}
		c := cs[r.pick]
		if c.kind != nil {
			r.status = "creating…"
			return r, r.loadTabs(*r.wt, c.kind, true)
		}
		r.bench = r.bench.Place(c.session.ID)
		return r.sync()
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
		if r.focus == focusTerminal {
			r.pick = min(max(r.pick+d, 0), len(r.choices())-1)
			return r, nil
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
		if wt, ok := r.sidebar.Selected(); ok && r.wt != nil && r.wt.ID == wt.ID {
			return r.setFocus(focusTerminal), nil // already on screen
		}
		var cmd tea.Cmd
		r.sidebar, cmd = r.sidebar.Open()
		return r, cmd
	}
	return r, nil
}

func (r Root) selectTab(i int) (Root, tea.Cmd) {
	if !r.ready() || i < 0 || i >= len(r.bench.Tabs) {
		return r, nil
	}
	r.bench.Active, r.bench.Zoom, r.pick = i, false, 0
	r = r.setFocus(focusTerminal)
	return r.sync()
}

func (r Root) onKey(k tea.KeyMsg) (Root, tea.Cmd) {
	key := k.String()
	if r.palette != nil {
		return r.onPaletteKey(k)
	}
	if r.leader {
		r.leader = false
		if act, ok := lookup(leaderKeys, key); ok {
			return r.do(act, key)
		}
		return r, nil
	}
	if r.resizing {
		if act, ok := lookup(resizeKeys, key); ok {
			return r.do(act, key)
		}
		return r, nil
	}
	if act, ok := lookup(globalKeys, key); ok {
		return r.do(act, key)
	}
	if r.focus == focusTerminal {
		id := r.bench.Tab().Focus
		if id == "" {
			if act, ok := lookup(pickKeys, key); ok {
				return r.do(act, key)
			}
			return r, nil
		}
		if s := r.screens[id]; s != nil {
			if b := keyBytes(k); b != nil {
				_ = s.Write(b)
			}
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
		var cmd, sync tea.Cmd
		r.sidebar, cmd = r.sidebar.Update(tea.WindowSizeMsg{Width: sidebarWidth, Height: msg.Height - 1})
		r, sync = r.sync()
		return r, tea.Batch(cmd, sync)
	case openMsg:
		r = r.switchTo(msg.wt)
		r.status = "attaching…"
		return r, r.loadTabs(msg.wt, nil, false)
	case tabsMsg:
		if msg.err != nil {
			r.status = msg.err.Error()
			return r, nil
		}
		if r.wt == nil || msg.wt.ID != r.wt.ID {
			if msg.created == "" {
				return r, nil // stale: another worktree has been opened since
			}
			r = r.switchTo(msg.wt) // a tab was made for the sidebar's selection
		}
		r.sessions, r.status = msg.sessions, ""
		if msg.into {
			r.bench = r.bench.Place(msg.created)
		}
		r.bench = r.bench.Reconcile(msg.sessions)
		if msg.created != "" {
			r.bench = r.bench.Show(msg.created)
		}
		if !msg.quiet {
			r = r.setFocus(focusTerminal)
		}
		return r.sync()
	case screenMsg:
		delete(r.attaching, msg.session.ID)
		if msg.err != nil {
			r.status = msg.err.Error()
			return r, nil
		}
		if _, ok := r.visible()[msg.session.ID]; !ok || r.screens[msg.session.ID] != nil {
			msg.screen.Close() // stale: the pane went away while attaching
			return r, nil
		}
		r.screens[msg.session.ID] = msg.screen
		r, cmd := r.sync() // the layout may have changed size meanwhile
		return r, tea.Batch(cmd, waitFrame(msg.screen))
	case frameMsg:
		for _, s := range r.screens {
			if s == msg.screen {
				return r, waitFrame(s)
			}
		}
		return r, nil // stale: detached since
	case closedMsg:
		for id, s := range r.screens {
			if s == msg.screen {
				delete(r.screens, id)
				return r, r.reload() // the session ended: reconcile drops its pane
			}
		}
		return r, nil
	case tea.KeyMsg:
		if r.sidebar.dialog == nil {
			return r.onKey(msg)
		}
	case tea.MouseMsg:
		if r.sidebar.dialog == nil {
			return r.onMouse(msg)
		}
		return r, nil
	}
	var cmd tea.Cmd
	r.sidebar, cmd = r.sidebar.Update(msg)
	return r, cmd
}

// onMouse: click focuses (sidebar row, tab, pane), dragging a divider
// resizes, and the wheel goes to the pane under the pointer.
func (r Root) onMouse(m tea.MouseMsg) (Root, tea.Cmd) {
	r.leader, r.palette = false, nil
	area := r.area()
	px, py := m.X-sidebarWidth-1, m.Y-1 // in the pane area
	switch {
	case m.Action == tea.MouseActionRelease:
		if r.dragging {
			r.dragging = false
			return r.sync() // resize the terminals once, at the end
		}
	case m.Action == tea.MouseActionMotion:
		if r.dragging {
			r.bench = r.bench.Drag(r.drag, area, px, py)
		}
	case m.Button == tea.MouseButtonWheelUp || m.Button == tea.MouseButtonWheelDown:
		if id, rect, ok := r.paneAt(px, py); ok && r.screens[id] != nil {
			r.screens[id].Scroll(px-rect.X, max(py-rect.Y-1, 0), m.Button == tea.MouseButtonWheelUp)
		}
	case m.Button != tea.MouseButtonLeft || m.Action != tea.MouseActionPress:
	case m.X < sidebarWidth:
		var ok bool
		if r.sidebar, ok = r.sidebar.Click(m.Y); ok {
			return r.setFocus(focusSidebar).do(actOpen, "")
		}
	case m.Y == 0:
		x := 0
		for i, l := range r.tabLabels() {
			if x += lipgloss.Width(l); px < x {
				return r.selectTab(i)
			}
		}
	case r.ready():
		if path, ok := r.bench.Tab().Root.DividerAt(area, px, py); ok && !r.bench.Zoom {
			r.drag, r.dragging = path, true
		}
		if id, _, ok := r.paneAt(px, py); ok {
			r.bench = r.bench.Show(id)
			return r.setFocus(focusTerminal), nil
		}
	}
	return r, nil
}

func (r Root) paneAt(x, y int) (string, domain.Rect, bool) {
	for id, rect := range r.visible() {
		if x >= rect.X && x < rect.X+rect.W && y >= rect.Y && y < rect.Y+rect.H {
			return id, rect, true
		}
	}
	return "", domain.Rect{}, false
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
	if r.modal() {
		r.sidebar.focused = false // the menu holds the accent while open
	}
	area := r.area()
	bodyH := area.H + 1 // tab bar + panes
	side := lipgloss.NewStyle().Width(sidebarWidth).Height(bodyH).MaxHeight(bodyH).Render(r.sidebar.View())
	sep := ruleStyle.Render(repeatLine("│", bodyH))

	var panes string
	switch {
	case r.wt == nil:
		panes = box(dimStyle.Render("enter on a worktree opens its terminals"), area.W, area.H)
	case !r.ready():
		panes = box(dimStyle.Render(r.status), area.W, area.H)
	case r.bench.Zoom:
		panes = r.pane(r.bench.Tab().Focus, area)
	default:
		panes = r.render(r.bench.Tab().Root, area)
	}
	if r.leader {
		box := r.leaderMenu()
		panes = overlay(panes, box, area.W-lipgloss.Width(box)-1, area.H-lipgloss.Height(box)-1)
	}
	if r.palette != nil {
		w := min(area.W-4, 72)
		panes = overlay(panes, r.paletteView(w), (area.W-w)/2, 1)
	}
	column := lipgloss.NewStyle().MaxWidth(area.W).Render(r.tabBar()) + "\n" + panes
	body := lipgloss.JoinHorizontal(lipgloss.Top, side, sep, column)
	return body + "\n" + r.statusBar()
}

// render draws a split tree into rect, mirroring Pane.Halves.
func (r Root) render(p *domain.Pane, rect domain.Rect) string {
	if p.IsLeaf() {
		return r.pane(p.Session, rect)
	}
	a, b := p.Halves(rect)
	if p.Dir == domain.Right {
		return lipgloss.JoinHorizontal(lipgloss.Top, r.render(p.First, a), ruleStyle.Render(repeatLine("│", rect.H)), r.render(p.Second, b))
	}
	return r.render(p.First, a) + "\n" + r.render(p.Second, b)
}

// pane is one pane: a header line, then the terminal or the picker.
func (r Root) pane(id string, rect domain.Rect) string {
	var body string
	switch s := r.screens[id]; {
	case id == "":
		body = r.picker()
	case s != nil:
		body = s.Render()
	case r.status != "":
		body = dimStyle.Render(r.status)
	default:
		body = dimStyle.Render("attaching…")
	}
	return r.paneHeader(id, rect.W) + "\n" + box(body, rect.W, rect.H-1)
}

func box(s string, w, h int) string {
	return lipgloss.NewStyle().Width(w).Height(h).MaxWidth(w).MaxHeight(h).Render(s)
}

func (r Root) label(id string) string {
	if s, ok := r.session(id); ok {
		return s.Label
	}
	return "new pane"
}

// picker is the empty pane's "what runs here?" list.
func (r Root) picker() string {
	lines := []string{mutedStyle.Render(" what runs here?"), ""}
	focused := r.focus == focusTerminal && !r.modal()
	for i, c := range r.choices() {
		name, note := c.session.Label, "running, not shown"
		if c.kind != nil {
			name, note = c.kind.Label, "new session"
		}
		bar, line := " ", " "+padRight(name, 10)+dimStyle.Render(note)
		if i == r.pick {
			line = selectedStyle.Render(" " + padRight(name, 10) + note + " ")
			if focused {
				bar = accentStyle.Render("▌")
			}
		}
		lines = append(lines, bar+line)
	}
	return strings.Join(lines, "\n")
}

func (r Root) tabLabels() []string {
	var out []string
	for i, t := range r.bench.Tabs {
		var names []string
		for _, id := range t.Root.Leaves() {
			names = append(names, r.label(id))
		}
		label := " " + strconv.Itoa(i+1) + " " + strings.Join(names, "+") + " "
		if i == r.bench.Active && r.bench.Zoom {
			label += "[zoom] "
		}
		out = append(out, label)
	}
	return out
}

func (r Root) tabBar() string {
	if !r.ready() {
		return ""
	}
	var b strings.Builder
	for i, l := range r.tabLabels() {
		if i == r.bench.Active {
			b.WriteString(selectedStyle.Render(l))
		} else {
			b.WriteString(mutedStyle.Render(l))
		}
	}
	return b.String()
}

// paneHeader is a pane's title rule; it turns accent on the pane keys go
// to, so it's the one place on screen that says "you're typing here".
func (r Root) paneHeader(id string, w int) string {
	title := " " + r.label(id) + " "
	fill := max(w-lipgloss.Width(title)-1, 0)
	if r.focus == focusTerminal && !r.modal() && id == r.bench.Tab().Focus {
		return ansi.Truncate(accentStyle.Render("━"+title+strings.Repeat("━", fill)), w, "")
	}
	return ansi.Truncate(ruleStyle.Render("─")+mutedStyle.Render(title)+ruleStyle.Render(strings.Repeat("─", fill)), w, "")
}

func (r Root) mode() (string, string) {
	switch {
	case r.palette != nil:
		return "PALETTE", dimStyle.Render("type to search  ↑↓ choose  enter run  esc close")
	case r.leader:
		return "MENU", dimStyle.Render("press a key from the menu")
	case r.resizing:
		return "RESIZE", hints(resizeKeys)
	case r.focus == focusTerminal && r.bench.Tab().Focus == "":
		return "NEW PANE", hints(pickKeys) + "  " + hints(globalKeys)
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
		b.WriteString(accentStyle.Render(padRight(k.label, 5)) + textStyle.Render(k.help))
	}
	return menuStyle.Render(dimStyle.Render("ctrl+space") + "\n" + b.String())
}

// overlay draws box over base with its top-left corner at x, y, keeping
// base's styling on both sides of it.
func overlay(base, box string, x, y int) string {
	lines := strings.Split(base, "\n")
	boxLines := strings.Split(box, "\n")
	bw := lipgloss.Width(box)
	x, y = max(x, 0), max(y, 0)
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
