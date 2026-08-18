package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// openFileLogPrefix namespaces this subcommand's own error output the same
// way hooksLogPrefix does for install-hooks/uninstall-hooks.
const openFileLogPrefix = "worktree-studio__open-file: "

// runOpenFileCommand handles the `open-file <path>` subcommand: run from
// inside one of this app's tmux-backed terminal panes (e.g. as $EDITOR, or
// typed directly), it tells the running worktree-studio server to open
// <path> in whichever browser tab has the calling shell's worktree open
// (see internal/openfile and internal/api/hooks.go's handleOpenFile).
// Returns whether args[0] was this subcommand at all — false means main()
// should fall through to running the server, same convention as
// runHooksCommand.
func runOpenFileCommand(args []string) bool {
	if len(args) == 0 || args[0] != "open-file" {
		return false
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, openFileLogPrefix+"usage: worktree-studio open-file <path>")
		os.Exit(1)
	}
	os.Exit(openFile(args[1]))
	return true
}

func openFile(path string) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, openFileLogPrefix+"resolve cwd: %v\n", err)
		return 1
	}

	addr := defaultAddr
	if v := os.Getenv("WORKTREE_STUDIO_ADDR"); v != "" {
		addr = v
	}
	selfBaseURL := "http://localhost" + addrPort(addr)

	body, err := json.Marshal(map[string]string{"cwd": cwd, "path": path})
	if err != nil {
		fmt.Fprintf(os.Stderr, openFileLogPrefix+"encode request: %v\n", err)
		return 1
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(selfBaseURL+"/api/open-file", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, openFileLogPrefix+"is worktree-studio running? request failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, openFileLogPrefix+"server returned %d: %s\n", resp.StatusCode, respBody)
		return 1
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && result.Status == "no matching worktree" {
		fmt.Fprintln(os.Stderr, openFileLogPrefix+"current directory isn't inside any worktree-studio-tracked worktree")
		return 1
	}

	return 0
}
