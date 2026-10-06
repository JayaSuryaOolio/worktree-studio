// Package layoutfile implements app.LayoutStore as one JSON file, by
// default ~/.config/wts-tui/layouts.json (or under $XDG_CONFIG_HOME).
package layoutfile

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"worktree-studio/tui/domain"
)

type Store struct{ Path string }

func Default() Store {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return Store{filepath.Join(dir, "wts-tui", "layouts.json")}
}

func (s Store) Load() (map[domain.WorktreeID]domain.Workbench, error) {
	out := map[domain.WorktreeID]domain.Workbench{}
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(b, &out)
}

// Save writes via a temp file and rename, so a crash never leaves half a file.
func (s Store) Save(layouts map[domain.WorktreeID]domain.Workbench) error {
	b, err := json.Marshal(layouts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}
