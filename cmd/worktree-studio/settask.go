package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const setTaskLogPrefix = "worktree-studio__set-task: "

const setTaskUsage = "usage: worktree-studio set-task (<source>:<key> | <local-key> | --new <title> | --clear) [--task-url <url>] [path]"

// taskFlags is the `task` object both create-worktree and set-task send to
// the server (see internal/api/tasks.go's taskRef). All empty clears the
// worktree's task.
type taskFlags struct {
	Source   string `json:"source,omitempty"`
	Key      string `json:"key,omitempty"`
	URL      string `json:"url,omitempty"`
	NewTitle string `json:"new_title,omitempty"`
}

// setRef parses "jira:PROJ-1" into source+key; a bare "12" or "#12" is a
// local task key.
func (t *taskFlags) setRef(v string) {
	if src, key, ok := strings.Cut(v, ":"); ok {
		t.Source, t.Key = src, key
		return
	}
	t.Source, t.Key = "local", strings.TrimPrefix(v, "#")
}

// takeFlag matches rest[*i] against `--name value` or `--name=value`,
// advancing *i past a separate value. Exits on a missing value.
func takeFlag(rest []string, i *int, name, logPrefix string) (string, bool) {
	a := rest[*i]
	if v, ok := strings.CutPrefix(a, name+"="); ok {
		return v, true
	}
	if a != name {
		return "", false
	}
	if *i+1 >= len(rest) {
		fmt.Fprintln(os.Stderr, logPrefix+name+" requires a value")
		os.Exit(1)
	}
	*i++
	return rest[*i], true
}

// runSetTaskCommand handles `set-task`: labels the worktree containing path
// (default: cwd) with a task through the running server, the same way
// create-worktree goes through it. This is how external tools (Jira
// automations, agents) group worktrees under a task without the UI.
func runSetTaskCommand(args []string) bool {
	if len(args) == 0 || args[0] != "set-task" {
		return false
	}

	var task taskFlags
	var path string
	clear, picked := false, false
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if v, ok := takeFlag(rest, &i, "--new", setTaskLogPrefix); ok {
			task.NewTitle, picked = v, true
			continue
		}
		if v, ok := takeFlag(rest, &i, "--task-url", setTaskLogPrefix); ok {
			task.URL = v
			continue
		}
		switch {
		case a == "--clear":
			clear = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, setTaskLogPrefix+"unrecognized flag %q\n%s\n", a, setTaskUsage)
			os.Exit(1)
		case !picked && !clear:
			task.setRef(a)
			picked = true
		case path == "":
			path = a
		default:
			fmt.Fprintln(os.Stderr, setTaskLogPrefix+"too many arguments\n"+setTaskUsage)
			os.Exit(1)
		}
	}
	if picked == clear {
		fmt.Fprintln(os.Stderr, setTaskLogPrefix+setTaskUsage)
		os.Exit(1)
	}
	if clear {
		task = taskFlags{}
	}

	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, setTaskLogPrefix+"resolve cwd: %v\n", err)
			os.Exit(1)
		}
		path = cwd
	}

	body, _ := json.Marshal(struct {
		Path string `json:"path"`
		taskFlags
	}{path, task})
	os.Exit(postCLI("/api/worktree-task", body, path, setTaskLogPrefix))
	return true
}

// postCLI POSTs body to the running server and prints the response — the
// shared tail of create-worktree and set-task.
func postCLI(endpoint string, body []byte, path, logPrefix string) int {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(serverBaseURL()+endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, logPrefix+"is worktree-studio running? request failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		fmt.Fprintf(os.Stderr, logPrefix+"server returned %d: %s\n", resp.StatusCode, respBody)
		return 1
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && result.Status == "no matching worktree" {
		fmt.Fprintf(os.Stderr, logPrefix+"%q isn't inside any worktree-studio-tracked repo\n", path)
		return 1
	}

	fmt.Fprintln(os.Stdout, string(respBody))
	return 0
}
