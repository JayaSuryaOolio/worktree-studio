package store

// TaskSource is a configured external task source: an adapter command that
// answers the picker search and title refresh (see internal/tasksrc). ID is
// the label worktrees carry in task_source, e.g. "jira". The built-in
// "local" source is never stored here.
type TaskSource struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
}

func (s *Store) migrateTaskSources() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS task_sources (
	id      TEXT PRIMARY KEY,
	name    TEXT NOT NULL,
	command TEXT NOT NULL
)`)
	return err
}

// ListTaskSources returns every configured source, by id.
func (s *Store) ListTaskSources() ([]TaskSource, error) {
	rows, err := s.db.Query(`SELECT id, name, command FROM task_sources ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TaskSource{}
	for rows.Next() {
		var src TaskSource
		if err := rows.Scan(&src.ID, &src.Name, &src.Command); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// AddTaskSource stores a new source; a duplicate id is a constraint error.
func (s *Store) AddTaskSource(src TaskSource) error {
	_, err := s.db.Exec(`INSERT INTO task_sources (id, name, command) VALUES (?, ?, ?)`, src.ID, src.Name, src.Command)
	return err
}

// DeleteTaskSource removes a source. Worktrees keep their labels; the tasks
// just stop refreshing. Returns sql.ErrNoRows if it doesn't exist.
func (s *Store) DeleteTaskSource(id string) error {
	return execOne(s.db, `DELETE FROM task_sources WHERE id = ?`, id)
}

// ActiveTaskKeys returns the keys of source's tasks that an active worktree
// carries — what a title refresh asks the adapter about.
func (s *Store) ActiveTaskKeys(source string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT task_key FROM worktrees WHERE task_source = ? AND task_key != '' AND status = ? ORDER BY task_key`,
		source, WorktreeStatusActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RefreshTask stores what the source says about a task: the title always,
// the url only if the task has none (a hand-set link wins). It doesn't touch
// updated_at, so a refresh doesn't reorder the board.
func (s *Store) RefreshTask(source, key, title, url string) error {
	_, err := s.db.Exec(
		`UPDATE tasks SET title = ?, url = CASE WHEN url = '' THEN ? ELSE url END WHERE source = ? AND key = ?`,
		title, url, source, key)
	return err
}
