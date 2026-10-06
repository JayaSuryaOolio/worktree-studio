package domain

import (
	"strings"
	"testing"
)

func sessions(ids ...string) []TerminalSession {
	var out []TerminalSession
	for _, id := range ids {
		out = append(out, TerminalSession{ID: id})
	}
	return out
}

func leaves(t Tab) string { return strings.Join(t.Root.Leaves(), ",") }

// a | b over c:  split a right (b), then b down (c).
func TestSplitRectsAndNeighbors(t *testing.T) {
	root := Leaf("a").Split("a", Right, "b").Split("b", Down, "c")
	rects := root.Rects(Rect{0, 0, 81, 20})
	if (rects["a"] != Rect{0, 0, 40, 20}) || (rects["b"] != Rect{41, 0, 40, 10}) || (rects["c"] != Rect{41, 10, 40, 10}) {
		t.Fatalf("rects %v", rects)
	}
	for _, c := range []struct {
		from string
		d    Dir
		want string
	}{{"a", Right, "b"}, {"c", Left, "a"}, {"b", Down, "c"}, {"c", Up, "b"}, {"a", Left, ""}, {"b", Right, ""}} {
		if got, _ := Neighbor(rects, c.from, c.d); got != c.want {
			t.Errorf("%s %v: got %q want %q", c.from, c.d, got, c.want)
		}
	}
	if got := strings.Join(root.Close("b").Leaves(), ","); got != "a,c" {
		t.Fatalf("close b: %s", got)
	}
	if root.Close("a").Close("b").Close("c") != nil {
		t.Fatal("closing every leaf empties the tree")
	}
}

func TestWorkbenchReconcileSplitPlaceClose(t *testing.T) {
	w := Workbench{}.Reconcile(sessions("s1", "s2"))
	if len(w.Tabs) != 2 || leaves(w.Tabs[1]) != "s2" {
		t.Fatalf("one tab per new session: %+v", w.Tabs)
	}

	// split, then fill the empty pane with a newly created session
	w = w.Split(Right)
	if w.Tab().Focus != "" || leaves(w.Tab()) != "s1," {
		t.Fatalf("split focuses an empty pane: %s", leaves(w.Tab()))
	}
	if w.Split(Down).Tab().Root != w.Tab().Root {
		t.Fatal("only one empty pane at a time")
	}
	w = w.Place("s3").Reconcile(sessions("s1", "s2", "s3"))
	if len(w.Tabs) != 2 || leaves(w.Tab()) != "s1,s3" || w.Tab().Focus != "s3" {
		t.Fatalf("placed session must not also get its own tab: %+v", w.Tabs)
	}

	// closing a pane hides its session; it stays hidden across reconciles
	w = w.ClosePane().Reconcile(sessions("s1", "s2", "s3"))
	if leaves(w.Tab()) != "s1" || len(w.Hidden(sessions("s1", "s2", "s3"))) != 1 {
		t.Fatalf("after close: %s", leaves(w.Tab()))
	}

	// a session that ended disappears, and its tab with it
	w = w.Reconcile(sessions("s1", "s3"))
	if len(w.Tabs) != 1 || leaves(w.Tab()) != "s1" {
		t.Fatalf("ended session's tab: %+v", w.Tabs)
	}

	// closing the last pane leaves an empty pane to pick into
	w = w.ClosePane()
	if len(w.Tabs) != 1 || leaves(w.Tab()) != "" {
		t.Fatalf("last close: %+v", w.Tabs)
	}
}

func TestResizeAndDrag(t *testing.T) {
	area := Rect{0, 0, 81, 20}
	root := Leaf("a").Split("a", Right, "b").Split("b", Right, "c") // a | (b | c)
	width := func(p *Pane, id string) int { return p.Rects(area)[id].W }

	// b grows left by moving the outer divider (b is the inner split's First,
	// so only the outer one can grow it leftward)
	if got := root.Resize("b", Left, 8, area); width(got, "b") <= width(root, "b") || width(got, "a") >= width(root, "a") {
		t.Fatalf("b left: %v", got.Rects(area))
	}
	// c is at the right edge: right moves its nearest divider, shrinking it
	if got := root.Resize("c", Right, 8, area); width(got, "c") >= width(root, "c") {
		t.Fatalf("c right: %v", got.Rects(area))
	}
	// no stacked split: up/down do nothing
	if root.Resize("a", Down, 8, area) != root {
		t.Fatal("no divider on that axis")
	}

	div := root.Rects(area)["a"].W // the outer divider's column
	path, ok := root.DividerAt(area, div, 5)
	if !ok || len(path) != 0 {
		t.Fatalf("outer divider at x=%d: %v %v", div, path, ok)
	}
	if got := root.DragTo(path, area, 20, 5); got.Rects(area)["a"].W != 20 {
		t.Fatalf("drag: %v", got.Rects(area))
	}
	if _, ok := root.DividerAt(area, 5, 5); ok {
		t.Fatal("inside a pane is not a divider")
	}
}
