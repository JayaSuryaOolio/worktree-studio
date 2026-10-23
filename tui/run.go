// Package tui is the terminal frontend's composition root, shared by
// cmd/wts-tui and the `wts tui` subcommand of the main binary.
package tui

import (
	"context"
	"os"
	"os/signal"

	tea "github.com/charmbracelet/bubbletea"

	"worktree-studio/tui/app"
	"worktree-studio/tui/infra/httpapi"
	"worktree-studio/tui/infra/layoutfile"
	"worktree-studio/tui/infra/ptyterm"
	"worktree-studio/tui/ui"
)

// Run blocks until the user quits. The server address comes from
// WORKTREE_STUDIO_ADDR (see httpapi.FromEnv).
func Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := httpapi.FromEnv()
	sidebar := ui.NewSidebar(ctx, app.NewSidebar(client), app.NewNewWorktree(client), client)
	model := ui.NewRoot(ctx, sidebar, app.NewTerminals(client, ptyterm.Attacher{})).WithLayouts(layoutfile.Default())
	_, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
