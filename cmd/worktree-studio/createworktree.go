package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// createWorktreeLogPrefix namespaces this subcommand's own error output the
// same way spotlightLogPrefix/orphansLogPrefix/openFileLogPrefix do for
// theirs.
const createWorktreeLogPrefix = "worktree-studio__create-worktree: "

const createWorktreeUsage = "usage: worktree-studio create-worktree <name> [--branch <source-branch>] [--task <source>:<key> | --new-task <title>] [--task-url <url>] [path]"

// runCreateWorktreeCommand handles the `create-worktree <name> [--branch
// <source-branch>] [path]` subcommand: tells the running worktree-studio
// server to create a new worktree for whichever repo `path` (or, if
// omitted, the current working directory) belongs to — through the server
// itself (see internal/api/api.go's handleCreateWorktreeCLI), the same "go
// through the server, don't reimplement its job" idiom
// runSpotlightCommand/runOpenFileCommand already use, so a worktree created
// this way shows up in the audit log and the UI's worktree list exactly
// like one created from the dashboard. This is what lets Claude (or any
// other tool without access to this app's UI) create a worktree just by
// being somewhere inside the target repo — its root checkout or any of its
// existing worktrees — without ever needing that repo's id.
//
// path is optional: if omitted, it defaults to the current working
// directory, same as open-file/spotlight's implicit cwd resolution.
//
// Returns whether args[0] was this subcommand at all — false means main()
// should fall through to running the server, same convention as the other
// one-shot subcommands.
func runCreateWorktreeCommand(args []string) bool {
	if len(args) == 0 || args[0] != "create-worktree" {
		return false
	}

	var name, branch, path string
	var task taskFlags
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if v, ok := takeFlag(rest, &i, "--branch", createWorktreeLogPrefix); ok {
			branch = v
			continue
		}
		if v, ok := takeFlag(rest, &i, "--task", createWorktreeLogPrefix); ok {
			task.setRef(v)
			continue
		}
		if v, ok := takeFlag(rest, &i, "--new-task", createWorktreeLogPrefix); ok {
			task.NewTitle = v
			continue
		}
		if v, ok := takeFlag(rest, &i, "--task-url", createWorktreeLogPrefix); ok {
			task.URL = v
			continue
		}
		switch {
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, createWorktreeLogPrefix+"unrecognized flag %q\n%s\n", a, createWorktreeUsage)
			os.Exit(1)
		case name == "":
			name = a
		case path == "":
			path = a
		default:
			fmt.Fprintln(os.Stderr, createWorktreeLogPrefix+"too many arguments\n"+createWorktreeUsage)
			os.Exit(1)
		}
	}
	if name == "" {
		fmt.Fprintln(os.Stderr, createWorktreeLogPrefix+createWorktreeUsage)
		os.Exit(1)
	}

	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, createWorktreeLogPrefix+"resolve cwd: %v\n", err)
			os.Exit(1)
		}
		path = cwd
	}

	os.Exit(createWorktreeAction(path, name, branch, task))
	return true
}

func createWorktreeAction(path, name, branch string, task taskFlags) int {
	body, err := json.Marshal(map[string]any{"path": path, "name": name, "source_branch": branch, "task": task})
	if err != nil {
		fmt.Fprintf(os.Stderr, createWorktreeLogPrefix+"encode request: %v\n", err)
		return 1
	}
	return postCLI("/api/worktrees", body, path, createWorktreeLogPrefix)
}
