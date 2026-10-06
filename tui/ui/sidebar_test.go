package ui

import (
	"strings"
	"testing"

	"worktree-studio/tui/domain"
)

func TestMidTruncateKeepsTail(t *testing.T) {
	got := midTruncate("feature/ENG-1234-add-login-button", 16)
	if len([]rune(got)) != 16 || got[len(got)-6:] != "button" {
		t.Fatalf("got %q", got)
	}
	if midTruncate("short", 16) != "short" {
		t.Fatal("short strings must pass through")
	}
}

func TestWindowKeepsCursorVisible(t *testing.T) {
	m := SidebarModel{height: 12, cursor: 25}
	s, e := m.window(30)
	if e-s != 10 || m.cursor < s || m.cursor >= e || e > 30 {
		t.Fatalf("window %d-%d", s, e)
	}
	m.cursor = 0
	if s, _ := m.window(30); s != 0 {
		t.Fatal("cursor at top must start at 0")
	}
}

func TestSidebarRowsTreeAndFilter(t *testing.T) {
	repos := []domain.Repo{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}
	wt := func(r domain.RepoID, branch string) domain.SidebarItem {
		return domain.SidebarItem{Worktree: domain.Worktree{RepoID: r, Branch: branch}}
	}
	items := map[domain.RepoID][]domain.SidebarItem{
		"a": {wt("a", "feat/login"), wt("a", "fix/crash")},
		"b": {wt("b", "feat/LOGOUT")},
	}
	show := func(rows []sidebarRow) (out []string) {
		for _, r := range rows {
			if r.item == nil {
				out = append(out, string(r.repo.ID))
			} else {
				out = append(out, "  "+r.item.Worktree.Branch)
			}
		}
		return out
	}
	eq := func(got []string, want ...string) {
		t.Helper()
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	eq(show(sidebarRows(repos, items, nil, "")), "a", "b")
	eq(show(sidebarRows(repos, items, map[domain.RepoID]bool{"a": true}, "")), "a", "  feat/login", "  fix/crash", "b")
	// filter ignores case and expansion, and hides repos with no match
	eq(show(sidebarRows(repos, items, nil, "LOG")), "a", "  feat/login", "b", "  feat/LOGOUT")
	eq(show(sidebarRows(repos, items, nil, "crash")), "a", "  fix/crash")
}
