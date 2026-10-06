package domain

// Dir is a direction on screen. Splits use Right (side by side) and Down
// (stacked); focus moves use all four.
type Dir int

const (
	Left Dir = iota
	Right
	Up
	Down
)

type Rect struct{ X, Y, W, H int }

// Pane is a node of a tab's split tree. A leaf shows one terminal session
// by ID; Session "" is an empty pane waiting for the user to pick what runs
// in it. An inner node divides its area between First and Second. Trees
// are never mutated: every operation returns a new one.
type Pane struct {
	Session       string
	Dir           Dir     // Right or Down
	Ratio         float64 // First's share of the area
	First, Second *Pane
}

func Leaf(session string) *Pane { return &Pane{Session: session} }

func (p *Pane) IsLeaf() bool { return p.First == nil }

// Split puts a new leaf beside target: to its right, or below it.
func (p *Pane) Split(target string, dir Dir, session string) *Pane {
	if p.IsLeaf() {
		if p.Session != target {
			return p
		}
		return &Pane{Dir: dir, Ratio: 0.5, First: p, Second: Leaf(session)}
	}
	q := *p
	q.First, q.Second = p.First.Split(target, dir, session), p.Second.Split(target, dir, session)
	return &q
}

// Close removes target's leaf; its sibling takes the parent's place. Nil
// means the tree is now empty.
func (p *Pane) Close(target string) *Pane {
	if p.IsLeaf() {
		if p.Session == target {
			return nil
		}
		return p
	}
	a, b := p.First.Close(target), p.Second.Close(target)
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	q := *p
	q.First, q.Second = a, b
	return &q
}

// Replace swaps the session shown in one leaf.
func (p *Pane) Replace(old, session string) *Pane {
	if p.IsLeaf() {
		if p.Session == old {
			return Leaf(session)
		}
		return p
	}
	q := *p
	q.First, q.Second = p.First.Replace(old, session), p.Second.Replace(old, session)
	return &q
}

// Leaves lists sessions left-to-right, top-to-bottom.
func (p *Pane) Leaves() []string {
	if p.IsLeaf() {
		return []string{p.Session}
	}
	return append(p.First.Leaves(), p.Second.Leaves()...)
}

// Halves divides r between First and Second. Side-by-side panes get a
// one-column divider between them; stacked panes don't need one, because
// every pane starts with its own header line.
func (p *Pane) Halves(r Rect) (Rect, Rect) {
	if p.Dir == Right {
		aw := min(max(int(float64(r.W-1)*p.Ratio+0.5), 1), r.W-2)
		return Rect{r.X, r.Y, aw, r.H}, Rect{r.X + aw + 1, r.Y, r.W - aw - 1, r.H}
	}
	ah := min(max(int(float64(r.H)*p.Ratio+0.5), 1), r.H-1)
	return Rect{r.X, r.Y, r.W, ah}, Rect{r.X, r.Y + ah, r.W, r.H - ah}
}

// Rects places every leaf inside r.
func (p *Pane) Rects(r Rect) map[string]Rect {
	out := map[string]Rect{}
	var walk func(*Pane, Rect)
	walk = func(n *Pane, r Rect) {
		if n.IsLeaf() {
			out[n.Session] = r
			return
		}
		a, b := n.Halves(r)
		walk(n.First, a)
		walk(n.Second, b)
	}
	walk(p, r)
	return out
}

// Neighbor is the pane next to from in direction d: the nearest one that
// shares some of from's edge, preferring the one sharing the most.
func Neighbor(rects map[string]Rect, from string, d Dir) (string, bool) {
	f, ok := rects[from]
	if !ok {
		return "", false
	}
	overlap := func(a0, a1, b0, b1 int) int { return min(a1, b1) - max(a0, b0) }
	best, bestDist, bestOver := "", 0, 0
	for id, c := range rects {
		var dist, over int
		switch d {
		case Left:
			dist, over = f.X-(c.X+c.W), overlap(f.Y, f.Y+f.H, c.Y, c.Y+c.H)
		case Right:
			dist, over = c.X-(f.X+f.W), overlap(f.Y, f.Y+f.H, c.Y, c.Y+c.H)
		case Up:
			dist, over = f.Y-(c.Y+c.H), overlap(f.X, f.X+f.W, c.X, c.X+c.W)
		case Down:
			dist, over = c.Y-(f.Y+f.H), overlap(f.X, f.X+f.W, c.X, c.X+c.W)
		}
		if id == from || dist < 0 || over <= 0 {
			continue
		}
		if best == "" || dist < bestDist || (dist == bestDist && over > bestOver) {
			best, bestDist, bestOver = id, dist, over
		}
	}
	return best, best != ""
}

// Tab is one split layout and the pane that has focus within it.
type Tab struct {
	Root  *Pane
	Focus string
}

// Workbench is one worktree's tabs. Each session the server reports gets
// its own tab the first time it's seen; after that, where it lives (or
// whether it's hidden because its pane was closed) is the user's choice.
type Workbench struct {
	Tabs   []Tab
	Active int
	Zoom   bool
	Known  map[string]bool // sessions already given a place
}

// Tab is the active tab; before the first Reconcile, an empty pane.
func (w Workbench) Tab() Tab {
	if len(w.Tabs) == 0 {
		return Tab{Root: Leaf("")}
	}
	return w.Tabs[w.Active]
}

func (w Workbench) withTab(t Tab) Workbench {
	tabs := append([]Tab(nil), w.Tabs...)
	tabs[w.Active] = t
	w.Tabs = tabs
	return w
}

// Reconcile drops panes whose session is gone and opens a tab for each
// session not seen before. A worktree always has at least one tab; if
// nothing is left to show, it's an empty pane.
func (w Workbench) Reconcile(sessions []TerminalSession) Workbench {
	live := map[string]bool{}
	for _, s := range sessions {
		live[s.ID] = true
	}
	known := map[string]bool{}
	for id := range w.Known {
		if live[id] {
			known[id] = true
		}
	}
	var tabs []Tab
	for i, t := range w.Tabs {
		root := t.Root
		for _, s := range t.Root.Leaves() {
			if root != nil && s != "" && !live[s] {
				root = root.Close(s)
			}
		}
		if root == nil {
			if i < w.Active {
				w.Active--
			}
			continue
		}
		if !contains(root.Leaves(), t.Focus) {
			t.Focus = root.Leaves()[0]
		}
		tabs = append(tabs, Tab{root, t.Focus})
	}
	for _, s := range sessions {
		if !known[s.ID] {
			known[s.ID] = true
			tabs = append(tabs, Tab{Leaf(s.ID), s.ID})
		}
	}
	if len(tabs) == 0 {
		tabs = []Tab{{Leaf(""), ""}}
	}
	w.Tabs, w.Known = tabs, known
	w.Active = min(max(w.Active, 0), len(tabs)-1)
	return w
}

// Show makes the tab holding session active and focuses its pane.
func (w Workbench) Show(session string) Workbench {
	for i, t := range w.Tabs {
		if contains(t.Root.Leaves(), session) {
			w.Active = i
			t.Focus = session
			return w.withTab(t)
		}
	}
	return w
}

// Place fills the focused empty pane with session.
func (w Workbench) Place(session string) Workbench {
	t := w.Tab()
	if t.Focus != "" {
		return w
	}
	known := map[string]bool{session: true}
	for id := range w.Known {
		known[id] = true
	}
	w.Known = known
	return w.withTab(Tab{t.Root.Replace("", session), session})
}

// Split opens an empty pane beside the focused one and focuses it. One
// empty pane at a time: splitting from an empty pane does nothing.
func (w Workbench) Split(dir Dir) Workbench {
	t := w.Tab()
	if t.Focus == "" || contains(t.Root.Leaves(), "") {
		return w
	}
	w.Zoom = false
	return w.withTab(Tab{t.Root.Split(t.Focus, dir, ""), ""})
}

// ClosePane removes the focused pane from the layout (its session keeps
// running and can be picked into an empty pane again). A tab with no panes
// left is removed.
func (w Workbench) ClosePane() Workbench {
	t := w.Tab()
	w.Zoom = false
	if root := t.Root.Close(t.Focus); root != nil {
		return w.withTab(Tab{root, root.Leaves()[0]})
	}
	w.Tabs = append(append([]Tab(nil), w.Tabs[:w.Active]...), w.Tabs[w.Active+1:]...)
	if len(w.Tabs) == 0 {
		w.Tabs = []Tab{{Leaf(""), ""}}
	}
	w.Active = min(w.Active, len(w.Tabs)-1)
	return w
}

// Move focuses the neighbouring pane in area; false if there is none.
func (w Workbench) Move(d Dir, area Rect) (Workbench, bool) {
	t := w.Tab()
	next, ok := Neighbor(t.Root.Rects(area), t.Focus, d)
	if !ok {
		return w, false
	}
	w.Zoom = false
	return w.withTab(Tab{t.Root, next}), true
}

// Visible is each pane on screen with its area: the whole active tab, or
// just the focused pane when zoomed.
func (w Workbench) Visible(area Rect) map[string]Rect {
	t := w.Tab()
	if w.Zoom {
		return map[string]Rect{t.Focus: area}
	}
	return t.Root.Rects(area)
}

// Hidden lists sessions that aren't in any tab, in server order.
func (w Workbench) Hidden(sessions []TerminalSession) []TerminalSession {
	shown := map[string]bool{}
	for _, t := range w.Tabs {
		for _, s := range t.Root.Leaves() {
			shown[s] = true
		}
	}
	var out []TerminalSession
	for _, s := range sessions {
		if !shown[s.ID] {
			out = append(out, s)
		}
	}
	return out
}

// Path addresses an inner node from the root: false = First, true = Second.
type Path []bool

func (p *Pane) node(path Path, r Rect) (*Pane, Rect) {
	for _, second := range path {
		a, b := p.Halves(r)
		if second {
			p, r = p.Second, b
		} else {
			p, r = p.First, a
		}
	}
	return p, r
}

// withRatio sets one split's ratio, kept between 10% and 90%.
func (p *Pane) withRatio(path Path, ratio float64) *Pane {
	q := *p
	switch {
	case len(path) == 0:
		q.Ratio = min(max(ratio, 0.1), 0.9)
	case path[0]:
		q.Second = p.Second.withRatio(path[1:], ratio)
	default:
		q.First = p.First.withRatio(path[1:], ratio)
	}
	return &q
}

// DividerAt finds the split whose divider is at (x, y): the column between
// side-by-side panes, or the header line of a lower pane.
func (p *Pane) DividerAt(r Rect, x, y int) (Path, bool) {
	var path Path
	for !p.IsLeaf() {
		a, b := p.Halves(r)
		if p.Dir == Right && x == a.X+a.W && y >= r.Y && y < r.Y+r.H ||
			p.Dir == Down && y == b.Y && x >= r.X && x < r.X+r.W {
			return path, true
		}
		second := x >= b.X && y >= b.Y
		path = append(path, second)
		if second {
			p, r = p.Second, b
		} else {
			p, r = p.First, a
		}
	}
	return nil, false
}

// DragTo moves the divider at path to (x, y).
func (p *Pane) DragTo(path Path, r Rect, x, y int) *Pane {
	n, nr := p.node(path, r)
	if n.IsLeaf() {
		return p
	}
	ratio := float64(y-nr.Y) / float64(max(nr.H, 1))
	if n.Dir == Right {
		ratio = float64(x-nr.X) / float64(max(nr.W-1, 1))
	}
	return p.withRatio(path, ratio)
}

// Resize moves a divider beside target n cells toward d. It prefers the
// nearest divider that grows target that way; at the screen edge it moves
// the nearest divider on that axis instead (so target shrinks).
func (p *Pane) Resize(target string, d Dir, n int, r Rect) *Pane {
	horiz, toward := d == Left || d == Right, d == Right || d == Down
	var ancestors []Path
	var inFirst []bool
	cur, path := p, Path{}
	for !cur.IsLeaf() {
		first := contains(cur.First.Leaves(), target)
		if !first && !contains(cur.Second.Leaves(), target) {
			return p
		}
		ancestors, inFirst = append(ancestors, append(Path(nil), path...)), append(inFirst, first)
		path = append(path, !first)
		if first {
			cur = cur.First
		} else {
			cur = cur.Second
		}
	}
	pick := -1
	for i := len(ancestors) - 1; i >= 0; i-- {
		if node, _ := p.node(ancestors[i], r); (node.Dir == Right) != horiz {
			continue
		}
		if pick < 0 {
			pick = i
		}
		if inFirst[i] == toward {
			pick = i
			break
		}
	}
	if pick < 0 {
		return p
	}
	node, nr := p.node(ancestors[pick], r)
	size := nr.H
	if node.Dir == Right {
		size = nr.W - 1
	}
	delta := float64(n) / float64(max(size, 1))
	if !toward {
		delta = -delta
	}
	return p.withRatio(ancestors[pick], node.Ratio+delta)
}

// Resize nudges the focused pane's divider (see Pane.Resize).
func (w Workbench) Resize(d Dir, n int, area Rect) Workbench {
	t := w.Tab()
	if w.Zoom || len(w.Tabs) == 0 {
		return w
	}
	return w.withTab(Tab{t.Root.Resize(t.Focus, d, n, area), t.Focus})
}

// Drag moves the active tab's divider at path to (x, y).
func (w Workbench) Drag(path Path, area Rect, x, y int) Workbench {
	if len(w.Tabs) == 0 {
		return w
	}
	t := w.Tab()
	return w.withTab(Tab{t.Root.DragTo(path, area, x, y), t.Focus})
}
