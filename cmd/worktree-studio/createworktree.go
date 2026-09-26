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

// createWorktreeLogPrefix namespaces this subcommand's own error output the
// same way spotlightLogPrefix/orphansLogPrefix/openFileLogPrefix do for
// theirs.
const createWorktreeLogPrefix = "worktree-studio__create-worktree: "

const createWorktreeUsage = "usage: worktree-studio create-worktree <name> [--branch <source-branch>] [path]"

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
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--branch":
			if i+1 >= len(rest) {
				fmt.Fprintln(os.Stderr, createWorktreeLogPrefix+"--branch requires a value")
				os.Exit(1)
			}
			i++
			branch = rest[i]
		case strings.HasPrefix(a, "--branch="):
			branch = strings.TrimPrefix(a, "--branch=")
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

	os.Exit(createWorktreeAction(path, name, branch))
	return true
}

func createWorktreeAction(path, name, branch string) int {
	addr := defaultAddr
	if v := os.Getenv("WORKTREE_STUDIO_ADDR"); v != "" {
		addr = v
	}
	selfBaseURL := "http://localhost" + addrPort(addr)

	body, err := json.Marshal(map[string]string{"path": path, "name": name, "source_branch": branch})
	if err != nil {
		fmt.Fprintf(os.Stderr, createWorktreeLogPrefix+"encode request: %v\n", err)
		return 1
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(selfBaseURL+"/api/worktrees", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, createWorktreeLogPrefix+"is worktree-studio running? request failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		fmt.Fprintf(os.Stderr, createWorktreeLogPrefix+"server returned %d: %s\n", resp.StatusCode, respBody)
		return 1
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && result.Status == "no matching worktree" {
		fmt.Fprintf(os.Stderr, createWorktreeLogPrefix+"%q isn't inside any worktree-studio-tracked repo\n", path)
		return 1
	}

	fmt.Fprintln(os.Stdout, string(respBody))
	return 0
}
