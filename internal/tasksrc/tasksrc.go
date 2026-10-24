// Package tasksrc runs external task-source adapters: user-configured,
// read-only executables that answer `list [--query <text>]` and
// `get <key>...` with a JSON array of tasks (see docs/tasks-plan.md and the
// example install/task-sources/jira). Like internal/gh, it shells out to a
// real tool instead of speaking any tracker's API itself.
package tasksrc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Item is one task as an adapter reports it. There's no status: status is
// worktree-studio's own.
type Item struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Timeout bounds one adapter call; a picker search shouldn't hang on a slow
// tracker.
var Timeout = 15 * time.Second

// List runs `<command> list [--query <q>]`.
func List(command, q string) ([]Item, error) {
	args := []string{"list"}
	if q != "" {
		args = append(args, "--query", q)
	}
	return run(command, args...)
}

// Get runs `<command> get <keys...>`.
func Get(command string, keys []string) ([]Item, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	return run(command, append([]string{"get"}, keys...)...)
}

func run(command string, args ...string) ([]Item, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%s %s: %s", command, args[0], msg)
		}
		return nil, fmt.Errorf("%s %s: %w", command, args[0], err)
	}
	var items []Item
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		return nil, fmt.Errorf("%s %s: output is not a JSON array of tasks: %w", command, args[0], err)
	}
	out := items[:0]
	for _, it := range items {
		if it.Key != "" {
			out = append(out, it)
		}
	}
	return out, nil
}
