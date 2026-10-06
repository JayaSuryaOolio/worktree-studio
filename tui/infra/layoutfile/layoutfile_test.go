package layoutfile

import (
	"path/filepath"
	"strings"
	"testing"

	"worktree-studio/tui/domain"
)

func TestRoundTrip(t *testing.T) {
	s := Store{filepath.Join(t.TempDir(), "wts-tui", "layouts.json")}
	if got, err := s.Load(); err != nil || len(got) != 0 {
		t.Fatalf("missing file loads empty: %v %v", got, err)
	}
	w := domain.Workbench{}.Reconcile([]domain.TerminalSession{{ID: "a"}, {ID: "b"}}).Split(domain.Down)
	if err := s.Save(map[domain.WorktreeID]domain.Workbench{"wt": w}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if l := strings.Join(got["wt"].Tab().Root.Leaves(), ","); l != "a," || !got["wt"].Known["b"] || len(got["wt"].Tabs) != 2 {
		t.Fatalf("round trip: %+v", got["wt"])
	}
}
