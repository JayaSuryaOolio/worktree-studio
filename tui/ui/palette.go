package ui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// palette is the ctrl+space space command palette: one searchable list of
// this worktree's tabs, every worktree, and every action (the browser's
// Cmd+K).
type palette struct {
	query  string
	cursor int
}

type paletteItem struct {
	kind, label string
	run         func(Root) (Root, tea.Cmd)
}

var paletteActions = []struct {
	label string
	act   action
	key   string
}{
	{"new Claude tab", actNewClaude, ""},
	{"new shell tab", actNewShell, ""},
	{"split right", actSplit, "|"},
	{"split down", actSplit, "-"},
	{"next tab", actNextTab, ""},
	{"sidebar ↔ panes", actSidebar, ""},
	{"close pane", actClosePane, ""},
	{"close tab", actCloseTab, ""},
	{"zoom pane", actZoom, ""},
	{"resize mode", actResizeMode, ""},
	{"new worktree", actNewWorktree, ""},
	{"quit", actQuit, ""},
}

func (r Root) paletteItems() []paletteItem {
	var items []paletteItem
	if r.ready() {
		for i, l := range r.tabLabels() {
			items = append(items, paletteItem{"tab", strings.TrimSpace(l), func(r Root) (Root, tea.Cmd) { return r.selectTab(i) }})
		}
	}
	for _, repo := range r.sidebar.repos {
		for _, it := range r.sidebar.items[repo.ID] {
			wt := it.Worktree
			items = append(items, paletteItem{"worktree", repo.Name + " / " + wt.Branch, func(r Root) (Root, tea.Cmd) {
				return r, r.sidebar.OpenWorktree(wt)
			}})
		}
	}
	for _, a := range paletteActions {
		items = append(items, paletteItem{"action", a.label, func(r Root) (Root, tea.Cmd) { return r.do(a.act, a.key) }})
	}
	return items
}

// matches filters items by query: substring matches first, then ones whose
// letters appear in order ("fls" finds "feat/login-screen").
func (p palette) matches(items []paletteItem) []paletteItem {
	q := strings.ToLower(p.query)
	type scored struct {
		item  paletteItem
		score int
	}
	var hits []scored
	for _, it := range items {
		l := strings.ToLower(it.label)
		switch {
		case strings.Contains(l, q):
			hits = append(hits, scored{it, 0})
		case subsequence(l, q):
			hits = append(hits, scored{it, 1})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score < hits[j].score })
	out := make([]paletteItem, len(hits))
	for i, h := range hits {
		out[i] = h.item
	}
	return out
}

func subsequence(s, q string) bool {
	for _, c := range q {
		i := strings.IndexRune(s, c)
		if i < 0 {
			return false
		}
		s = s[i+len(string(c)):]
	}
	return true
}

func (r Root) onPaletteKey(k tea.KeyMsg) (Root, tea.Cmd) {
	p := *r.palette
	hits := p.matches(r.paletteItems())
	switch k.Type {
	case tea.KeyEsc, tea.KeyCtrlAt:
		r.palette = nil
		return r, nil
	case tea.KeyEnter:
		r.palette = nil
		if p.cursor < len(hits) {
			return hits[p.cursor].run(r)
		}
		return r, nil
	case tea.KeyUp:
		p.cursor = max(p.cursor-1, 0)
	case tea.KeyDown:
		p.cursor = min(p.cursor+1, max(len(hits)-1, 0))
	case tea.KeyBackspace:
		if q := []rune(p.query); len(q) > 0 {
			p.query, p.cursor = string(q[:len(q)-1]), 0
		}
	case tea.KeyRunes, tea.KeySpace:
		if !k.Alt {
			p.query, p.cursor = p.query+string(k.Runes), 0
		}
	case tea.KeyCtrlC:
		return r, tea.Quit
	}
	r.palette = &p
	return r, nil
}

const paletteRows = 12

func (r Root) paletteView(w int) string {
	p := *r.palette
	hits := p.matches(r.paletteItems())
	lines := []string{accentStyle.Render("› ") + textStyle.Render(p.query) + accentStyle.Render("▏"), ""}
	start := min(max(p.cursor-paletteRows/2, 0), max(len(hits)-paletteRows, 0))
	for i := start; i < min(start+paletteRows, len(hits)); i++ {
		h := hits[i]
		label := padRight(midTruncate(h.label, w-12), w-11)
		bar, line := " ", label+dimStyle.Render(padRight(h.kind, 9))
		if i == p.cursor {
			bar, line = accentStyle.Render("▌"), selectedStyle.Render(label+padRight(h.kind, 9))
		}
		lines = append(lines, bar+line)
	}
	if len(hits) == 0 {
		lines = append(lines, dimStyle.Render(" nothing matches"))
	}
	return menuStyle.Render(strings.Join(lines, "\n"))
}
