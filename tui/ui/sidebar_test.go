package ui

import (
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
	m := SidebarModel{height: 15, items: make([]domain.SidebarItem, 30), cursor: 25}
	s, e := m.window()
	if e-s != 10 || m.cursor < s || m.cursor >= e || e > 30 {
		t.Fatalf("window %d-%d", s, e)
	}
	m.cursor = 0
	if s, _ := m.window(); s != 0 {
		t.Fatal("cursor at top must start at 0")
	}
}
