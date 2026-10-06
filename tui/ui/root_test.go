package ui

import (
	"context"
	"os/exec"
	"strconv"
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
	return []domain.TerminalSession{{ID: "1", Label: "shell", TmuxName: o.name}}, nil
}
func (o oneSession) Create(context.Context, domain.RepoID, domain.WorktreeID, domain.TerminalKind) (domain.TerminalSession, error) {
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

	r := NewRoot(context.Background(), SidebarModel{}, app.NewTerminals(oneSession{name}, ptyterm.Attacher{}))
	r = drive(r, tea.WindowSizeMsg{Width: 100, Height: 12})
	r = drive(r, openMsg{domain.Worktree{ID: "w", RepoID: "r"}})
	if r.screens["1"] == nil {
		t.Fatalf("not attached: %s", r.status)
	}

	for _, c := range "hi-claude" {
		r = drive(r, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
	}
	r = drive(r, tea.KeyMsg{Type: tea.KeyEnter})
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(r.View(), "hi-claude") {
		if time.Now().After(deadline) {
			t.Fatalf("typed text never reached the pane:\n%s", r.View())
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Log("\n" + r.View())

	r = drive(r, tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
	if r.focus != focusSidebar {
		t.Fatal("alt+left from the leftmost pane must return focus to the sidebar")
	}
	r.screens["1"].Close()
}

// drive applies msg, then feeds back whatever its commands produce within a
// moment (screens' frame waits never return, so they're dropped).
func drive(r Root, msg tea.Msg) Root {
	m, cmd := r.Update(msg)
	r = m.(Root)
	for _, next := range run(cmd) {
		r = drive(r, next)
	}
	return r
}

func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if b, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, c := range b {
				out = append(out, run(c)...)
			}
			return out
		}
		if msg == nil {
			return nil
		}
		return []tea.Msg{msg}
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

type fakeScreen struct {
	typed  []byte
	closed bool
}

func (f *fakeScreen) Write(p []byte) error     { f.typed = append(f.typed, p...); return nil }
func (f *fakeScreen) Render() string           { return "" }
func (f *fakeScreen) Resize(int, int)          {}
func (f *fakeScreen) Scroll(int, int, bool)    {}
func (f *fakeScreen) Updates() <-chan struct{} { return nil }
func (f *fakeScreen) Close()                   { f.closed = true }

type fakeAttacher map[string]*fakeScreen

func (f fakeAttacher) Attach(s domain.TerminalSession, _, _ int) (app.Screen, error) {
	f[s.ID] = &fakeScreen{}
	return f[s.ID], nil
}

type fakeDir struct{ sessions []domain.TerminalSession }

func (d *fakeDir) Sessions(context.Context, domain.RepoID, domain.WorktreeID) ([]domain.TerminalSession, error) {
	return d.sessions, nil
}
func (d *fakeDir) Create(_ context.Context, _ domain.RepoID, _ domain.WorktreeID, k domain.TerminalKind) (domain.TerminalSession, error) {
	s := domain.TerminalSession{ID: "s" + strconv.Itoa(len(d.sessions)+1), Label: k.Label}
	d.sessions = append(d.sessions, s)
	return s, nil
}

func key(s string) tea.KeyMsg {
	switch s {
	case "ctrl+space":
		return tea.KeyMsg{Type: tea.KeyCtrlAt}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "shift+right":
		return tea.KeyMsg{Type: tea.KeyShiftRight}
	case "alt+shift+left":
		return tea.KeyMsg{Type: tea.KeyShiftLeft, Alt: true}
	case "alt+left":
		return tea.KeyMsg{Type: tea.KeyLeft, Alt: true}
	case "alt+right":
		return tea.KeyMsg{Type: tea.KeyRight, Alt: true}
	}
	if strings.HasPrefix(s, "alt+") {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s[4:]), Alt: true}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// The design's core rule, end to end through Root: bare keys go to the
// focused pane; only alt combos and ctrl+space are the app's. Also covers
// split → pick → focus moves → zoom → close.
func TestPanesKeysSplitsAndFocus(t *testing.T) {
	dir := &fakeDir{sessions: []domain.TerminalSession{{ID: "s1", Label: "shell"}, {ID: "s2", Label: "claude"}}}
	att := fakeAttacher{}
	r := NewRoot(context.Background(), SidebarModel{}, app.NewTerminals(dir, att))
	press := func(keys ...string) {
		for _, k := range keys {
			r = drive(r, key(k))
		}
	}
	r = drive(r, tea.WindowSizeMsg{Width: 120, Height: 30})
	r = drive(r, openMsg{domain.Worktree{ID: "w", RepoID: "r"}})
	if r.focus != focusTerminal || att["s1"] == nil {
		t.Fatal("opening a worktree shows and focuses its first tab")
	}

	press("c", "t", "q", "1", "ctrl+space", "y") // y isn't in the menu: closes it
	if string(att["s1"].typed) != "ctq1" || r.leader {
		t.Fatalf("bare keys must reach the terminal and nothing after ctrl+space may leak, got %q", att["s1"].typed)
	}

	press("alt+2")
	if r.bench.Active != 1 || att["s2"] == nil || !att["s1"].closed {
		t.Fatal("alt+2 shows tab 2 and detaches tab 1")
	}

	press("ctrl+space", "right")
	if r.bench.Tab().Focus != "" || !strings.Contains(r.View(), "what runs here?") {
		t.Fatalf("split focuses an empty pane with a picker:\n%s", r.View())
	}
	press("down", "enter") // shell
	if !strings.Contains(r.tabBar(), "2 claude+shell") || att["s3"] == nil || r.bench.Tab().Focus != "s3" {
		t.Fatalf("picked shell goes into the split: %q", r.tabBar())
	}

	press("alt+left", "a")
	if r.bench.Tab().Focus != "s2" || string(att["s2"].typed) != "a" {
		t.Fatal("alt+left moves to the left pane, and keys follow")
	}
	press("alt+left")
	if r.focus != focusSidebar {
		t.Fatal("alt+left from the leftmost pane goes to the sidebar")
	}
	press("alt+right", "ctrl+space", "z")
	if !r.bench.Zoom || !att["s3"].closed {
		t.Fatal("zoom shows only the focused pane")
	}
	press("ctrl+space", "x")
	if r.bench.Zoom || !strings.Contains(r.tabBar(), "2 shell") {
		t.Fatalf("closing a pane leaves its sibling: %q", r.tabBar())
	}
	if r.screens["s3"] == nil {
		t.Fatal("the remaining pane is attached again after unzoom")
	}
}

func TestResizeByKeyAndMouse(t *testing.T) {
	dir := &fakeDir{sessions: []domain.TerminalSession{{ID: "s1", Label: "shell"}}}
	r := NewRoot(context.Background(), SidebarModel{}, app.NewTerminals(dir, fakeAttacher{}))
	press := func(keys ...string) {
		for _, k := range keys {
			r = drive(r, key(k))
		}
	}
	click := func(x, y int, a tea.MouseAction) {
		r = drive(r, tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: a})
	}
	r = drive(r, tea.WindowSizeMsg{Width: 120, Height: 30})
	r = drive(r, openMsg{domain.Worktree{ID: "w", RepoID: "r"}})
	press("ctrl+space", "right", "down", "enter") // s1 | new shell s2
	w0 := r.visible()["s2"].W

	press("alt+shift+left")
	if r.visible()["s2"].W <= w0 {
		t.Fatal("alt+shift+← grows the right pane leftward")
	}
	press("ctrl+space", "r", "shift+right", "esc")
	if r.resizing || r.visible()["s2"].W >= w0 {
		t.Fatalf("resize mode: shift+→ shrinks it by a big step (w=%d, was %d)", r.visible()["s2"].W, w0)
	}

	x0 := sidebarWidth + 1 // where the pane area starts
	click(x0+2, 5, tea.MouseActionPress)
	if r.bench.Tab().Focus != "s1" {
		t.Fatal("clicking a pane focuses it")
	}
	div := x0 + r.visible()["s1"].W
	click(div, 5, tea.MouseActionPress)
	click(div-10, 5, tea.MouseActionMotion)
	click(div-10, 5, tea.MouseActionRelease)
	if got := x0 + r.visible()["s1"].W; got != div-10 || r.dragging {
		t.Fatalf("dragging the divider moves it: %d, want %d", got, div-10)
	}
}

type memStore struct {
	saved map[domain.WorktreeID]domain.Workbench
}

func (m *memStore) Load() (map[domain.WorktreeID]domain.Workbench, error) { return m.saved, nil }
func (m *memStore) Save(b map[domain.WorktreeID]domain.Workbench) error {
	m.saved = b
	return nil
}

// The palette finds tabs and actions by typing; a split layout survives a
// restart through the layout store.
func TestPaletteAndSavedLayouts(t *testing.T) {
	sessions := []domain.TerminalSession{{ID: "s1", Label: "claude"}, {ID: "s2", Label: "shell"}}
	store := &memStore{}
	start := func() Root {
		dir := &fakeDir{sessions: sessions}
		r := NewRoot(context.Background(), SidebarModel{}, app.NewTerminals(dir, fakeAttacher{})).WithLayouts(store)
		r = drive(r, tea.WindowSizeMsg{Width: 120, Height: 30})
		return drive(r, openMsg{domain.Worktree{ID: "w", RepoID: "r"}})
	}
	r := start()
	press := func(keys ...string) {
		for _, k := range keys {
			r = drive(r, key(k))
		}
	}

	press("ctrl+space", "space", "s", "h", "e", "l")
	if hits := r.palette.matches(r.paletteItems()); len(hits) != 2 || hits[0].label != "2 shell" || hits[1].label != "new shell tab" {
		t.Fatalf("substring hits, tabs first: %+v", hits)
	}
	if !strings.Contains(r.View(), "PALETTE") {
		t.Fatal("status bar says the palette has the keys")
	}
	press("enter")
	if r.palette != nil || r.bench.Active != 1 {
		t.Fatalf("enter runs the tab item: active=%d", r.bench.Active)
	}
	if got := (palette{query: "spd"}).matches(r.paletteItems()); len(got) != 1 || got[0].label != "split down" {
		t.Fatalf("letters in order match: %+v", got)
	}

	press("ctrl+space", "space", "s", "p", "l", "i", "t", " ", "r", "enter", "enter") // split right, then a shell in it
	if got := strings.Join(r.bench.Tab().Root.Leaves(), ","); got != "s2,s3" {
		t.Fatalf("split from the palette: %s", got)
	}
	sessions = append(sessions, domain.TerminalSession{ID: "s3", Label: "shell"})
	if r = start(); strings.Join(r.bench.Tab().Root.Leaves(), ",") != "s2,s3" {
		t.Fatalf("layout restored after restart: %v", r.bench.Tabs)
	}
}

// A pane whose attach ends (the shell exited, or tmux failed) must not be
// reattached in a loop: it waits for enter. ctrl+space w closes the tab.
func TestDetachedPaneWaitsAndCloseTab(t *testing.T) {
	dir := &fakeDir{sessions: []domain.TerminalSession{{ID: "s1", Label: "shell"}, {ID: "s2", Label: "shell"}}}
	attaches := 0
	att := countingAttacher{n: &attaches}
	r := NewRoot(context.Background(), SidebarModel{}, app.NewTerminals(dir, att))
	r = drive(r, tea.WindowSizeMsg{Width: 120, Height: 30})
	r = drive(r, openMsg{domain.Worktree{ID: "w", RepoID: "r"}})
	if attaches != 1 {
		t.Fatalf("attaches: %d", attaches)
	}
	r = drive(r, closedMsg{r.screens["s1"]}) // reload still lists s1
	if attaches != 1 || !strings.Contains(r.View(), "enter reattaches") {
		t.Fatalf("detached pane was reattached on its own: %d", attaches)
	}
	r = drive(r, key("enter"))
	if attaches != 2 {
		t.Fatalf("enter reattaches: %d", attaches)
	}

	r = drive(r, key("ctrl+space"))
	r = drive(r, key("w"))
	if len(r.bench.Tabs) != 1 || r.bench.Tab().Focus != "s2" {
		t.Fatalf("close tab: %+v", r.bench.Tabs)
	}
}

type countingAttacher struct{ n *int }

func (c countingAttacher) Attach(domain.TerminalSession, int, int) (app.Screen, error) {
	*c.n++
	return &fakeScreen{}, nil
}
