// Command wts-tui is a terminal frontend for a running worktree-studio
// server — a prototype alongside the browser UI in web/.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	tea "github.com/charmbracelet/bubbletea"

	"worktree-studio/tui/app"
	"worktree-studio/tui/infra/httpapi"
	"worktree-studio/tui/infra/layoutfile"
	"worktree-studio/tui/infra/ptyterm"
	"worktree-studio/tui/ui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := httpapi.FromEnv()
	sidebar := ui.NewSidebar(ctx, app.NewSidebar(client), app.NewNewWorktree(client), client)
	model := ui.NewRoot(ctx, sidebar, app.NewTerminals(client, ptyterm.Attacher{})).WithLayouts(layoutfile.Default())
	if _, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "wts-tui:", err)
		os.Exit(1)
	}
}
