package store

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Tasks are a label on worktrees, not a building block of their own (see
// docs/tasks-plan.md): a worktree carries (task_source, task_key), and the
// tasks table holds that pair's title, link and worktree-studio's own
// status. A task row only ever comes into existence when a worktree is
// labelled with it — there's no backlog.

// TaskSourceLocal is the built-in source: tasks created in worktree-studio
// itself, keyed by a sequential number. Any other source value is just a
// label naming where the task lives (e.g. "jira").
const TaskSourceLocal = "local"

// TaskStatus is one user-defined status. Position orders them: the first
// is where every new task starts, the last is where a task moves once no
// active worktree carries it any more.
type TaskStatus struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

// Task is the row behind a (source, key) worktree label.
type Task struct {
	Source    string `json:"source"`
	Key       string `json:"key"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	StatusID  int64  `json:"status_id"`
	UpdatedAt string `json:"updated_at"`
}

var (
	// ErrTaskStatusInUse is returned when deleting a status some task still has.
	ErrTaskStatusInUse = errors.New("status is in use by at least one task")
	// ErrLastTaskStatus is returned when deleting the only remaining status.
	ErrLastTaskStatus = errors.New("at least one status must remain")
)

var defaultTaskStatuses = []string{"In progress", "In review", "Done"}

func (s *Store) migrateTasks() error {
	const schema = `
CREATE TABLE IF NOT EXISTS task_statuses (
	id       INTEGER PRIMARY KEY,
	name     TEXT NOT NULL UNIQUE,
	position INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
	source     TEXT NOT NULL,
	key        TEXT NOT NULL,
	title      TEXT NOT NULL DEFAULT '',
	url        TEXT NOT NULL DEFAULT '',
	status_id  INTEGER NOT NULL REFERENCES task_statuses(id),
	updated_at TEXT NOT NULL,
	PRIMARY KEY (source, key)
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	for _, col := range []string{"task_source", "task_key"} {
		has, err := s.hasColumn("worktrees", col)
		if err != nil {
			return err
		}
		if !has {
			if _, err := s.db.Exec(`ALTER TABLE worktrees ADD COLUMN ` + col + ` TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		}
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_worktrees_task ON worktrees(task_source, task_key)`); err != nil {
		return err
	}

	var n int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM task_statuses`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for i, name := range defaultTaskStatuses {
		if _, err := s.db.Exec(`INSERT INTO task_statuses (name, position) VALUES (?, ?)`, name, i); err != nil {
			return err
		}
	}
	return nil
}

// ListTaskStatuses returns every status in position order.
func (s *Store) ListTaskStatuses() ([]TaskStatus, error) {
	rows, err := s.db.Query(`SELECT id, name, position FROM task_statuses ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TaskStatus{}
	for rows.Next() {
		var st TaskStatus
		if err := rows.Scan(&st.ID, &st.Name, &st.Position); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// AddTaskStatus appends a status at the end of the order.
func (s *Store) AddTaskStatus(name string) (TaskStatus, error) {
	res, err := s.db.Exec(
		`INSERT INTO task_statuses (name, position) VALUES (?, (SELECT COALESCE(MAX(position), -1) + 1 FROM task_statuses))`, name)
	if err != nil {
		return TaskStatus{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return TaskStatus{}, err
	}
	var st TaskStatus
	err = s.db.QueryRow(`SELECT id, name, position FROM task_statuses WHERE id = ?`, id).Scan(&st.ID, &st.Name, &st.Position)
	return st, err
}

// RenameTaskStatus renames a status. Tasks reference it by id, so they
// follow automatically.
func (s *Store) RenameTaskStatus(id int64, name string) error {
	return execOne(s.db, `UPDATE task_statuses SET name = ? WHERE id = ?`, name, id)
}

// ReorderTaskStatuses sets each listed status's position to its index in ids.
func (s *Store) ReorderTaskStatuses(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if err := execOne(tx, `UPDATE task_statuses SET position = ? WHERE id = ?`, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteTaskStatus removes a status no task uses, keeping at least one.
func (s *Store) DeleteTaskStatus(id int64) error {
	var inUse, total int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM tasks WHERE status_id = ?`, id).Scan(&inUse); err != nil {
		return err
	}
	if inUse > 0 {
		return ErrTaskStatusInUse
	}
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM task_statuses`).Scan(&total); err != nil {
		return err
	}
	if total <= 1 {
		return ErrLastTaskStatus
	}
	return execOne(s.db, `DELETE FROM task_statuses WHERE id = ?`, id)
}

// boundaryStatusID returns the first ("ASC") or last ("DESC") status id.
func (s *Store) boundaryStatusID(dir string) (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM task_statuses ORDER BY position ` + dir + `, id ` + dir + ` LIMIT 1`).Scan(&id)
	return id, err
}

const taskColumns = `source, key, title, url, status_id, updated_at`

func scanTask(row interface{ Scan(...any) error }, t *Task) error {
	return row.Scan(&t.Source, &t.Key, &t.Title, &t.URL, &t.StatusID, &t.UpdatedAt)
}

// GetTask fetches one task. Returns sql.ErrNoRows if it doesn't exist.
func (s *Store) GetTask(source, key string) (Task, error) {
	var t Task
	err := scanTask(s.db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE source = ? AND key = ?`, source, key), &t)
	return t, err
}

// ListTasks returns every task, most recently updated first.
func (s *Store) ListTasks() ([]Task, error) {
	rows, err := s.db.Query(`SELECT ` + taskColumns + ` FROM tasks ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Task{}
	for rows.Next() {
		var t Task
		if err := scanTask(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SearchTasks returns tasks whose key or title contains q (case-insensitive),
// most recently updated first.
func (s *Store) SearchTasks(q string, limit int) ([]Task, error) {
	like := "%" + strings.ToLower(q) + "%"
	rows, err := s.db.Query(
		`SELECT `+taskColumns+` FROM tasks WHERE LOWER(key) LIKE ? OR LOWER(title) LIKE ? ORDER BY updated_at DESC LIMIT ?`,
		like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Task{}
	for rows.Next() {
		var t Task
		if err := scanTask(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// CreateLocalTask creates a local task keyed by the next sequential number,
// at the first status.
func (s *Store) CreateLocalTask(title, url string) (Task, error) {
	first, err := s.boundaryStatusID("ASC")
	if err != nil {
		return Task{}, err
	}
	var next int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(CAST(key AS INTEGER)), 0) + 1 FROM tasks WHERE source = ?`, TaskSourceLocal).Scan(&next); err != nil {
		return Task{}, err
	}
	t := Task{Source: TaskSourceLocal, Key: strconv.Itoa(next), Title: title, URL: url, StatusID: first, UpdatedAt: now()}
	_, err = s.db.Exec(`INSERT INTO tasks (`+taskColumns+`) VALUES (?, ?, ?, ?, ?, ?)`, t.Source, t.Key, t.Title, t.URL, t.StatusID, t.UpdatedAt)
	return t, err
}

// EnsureTask returns the (source, key) task, creating it at the first status
// if it doesn't exist yet. A non-empty url is stored only when the task has
// none, so a link someone set by hand is never overwritten.
func (s *Store) EnsureTask(source, key, url string) (Task, error) {
	first, err := s.boundaryStatusID("ASC")
	if err != nil {
		return Task{}, err
	}
	if _, err := s.db.Exec(
		`INSERT INTO tasks (`+taskColumns+`) VALUES (?, ?, '', ?, ?, ?) ON CONFLICT(source, key) DO NOTHING`,
		source, key, url, first, now()); err != nil {
		return Task{}, err
	}
	if url != "" {
		if _, err := s.db.Exec(`UPDATE tasks SET url = ? WHERE source = ? AND key = ? AND url = ''`, url, source, key); err != nil {
			return Task{}, err
		}
	}
	return s.GetTask(source, key)
}

// TaskPatch is a partial update; nil fields are left unchanged.
type TaskPatch struct {
	Title    *string `json:"title"`
	URL      *string `json:"url"`
	StatusID *int64  `json:"status_id"`
}

// UpdateTask applies p to the (source, key) task. Returns sql.ErrNoRows if
// the task doesn't exist.
func (s *Store) UpdateTask(source, key string, p TaskPatch) (Task, error) {
	sets := []string{"updated_at = ?"}
	args := []any{now()}
	if p.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *p.Title)
	}
	if p.URL != nil {
		sets = append(sets, "url = ?")
		args = append(args, *p.URL)
	}
	if p.StatusID != nil {
		sets = append(sets, "status_id = ?")
		args = append(args, *p.StatusID)
	}
	args = append(args, source, key)
	if err := execOne(s.db, `UPDATE tasks SET `+strings.Join(sets, ", ")+` WHERE source = ? AND key = ?`, args...); err != nil {
		return Task{}, err
	}
	return s.GetTask(source, key)
}

// SetWorktreeTask labels a worktree with (source, key); both "" clears it.
func (s *Store) SetWorktreeTask(worktreeID, source, key string) error {
	return execOne(s.db, `UPDATE worktrees SET task_source = ?, task_key = ? WHERE id = ?`, source, key, worktreeID)
}

// ListTaskWorktrees returns every worktree that carries a task, any status.
func (s *Store) ListTaskWorktrees() ([]Worktree, error) {
	rows, err := s.db.Query(`SELECT ` + worktreeColumns + ` FROM worktrees WHERE task_key != '' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Worktree{}
	for rows.Next() {
		var w Worktree
		if err := scanWorktree(rows, &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// SettleTaskStatus keeps a task's status in step with its worktrees: once no
// active worktree carries it, it moves to the last status ("archived means
// done"); when an active worktree carries a task sitting at the last status,
// it moves back to the first ("nothing starts until a worktree starts").
// Statuses in between are the user's to set and are left alone.
func (s *Store) SettleTaskStatus(source, key string) error {
	if key == "" {
		return nil
	}
	t, err := s.GetTask(source, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	first, err := s.boundaryStatusID("ASC")
	if err != nil {
		return err
	}
	last, err := s.boundaryStatusID("DESC")
	if err != nil {
		return err
	}
	var active int
	if err := s.db.QueryRow(
		`SELECT COUNT(1) FROM worktrees WHERE task_source = ? AND task_key = ? AND status = ?`,
		source, key, WorktreeStatusActive).Scan(&active); err != nil {
		return err
	}
	switch {
	case active == 0 && t.StatusID != last:
		_, err = s.UpdateTask(source, key, TaskPatch{StatusID: &last})
	case active > 0 && t.StatusID == last && first != last:
		_, err = s.UpdateTask(source, key, TaskPatch{StatusID: &first})
	}
	return err
}

// execOne runs an UPDATE/DELETE and reports sql.ErrNoRows if nothing matched.
func execOne(db interface {
	Exec(string, ...any) (sql.Result, error)
}, query string, args ...any) error {
	res, err := db.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
