// Package web exists purely to hold the go:embed directive for the built
// frontend. go:embed patterns are resolved relative to the directory of
// the source file containing the directive, so this file has to live
// alongside web/dist (not in cmd/worktree-studio, which is a different
// directory) — cmd/worktree-studio/main.go imports this package instead.
//
// Before the frontend has ever been built, web/dist contains only a
// checked-in .gitkeep placeholder (see .gitignore) so this directive
// always has a directory to embed and `go build ./...` keeps working even
// on a fresh checkout; main.go's mountFrontend falls back to a friendly
// placeholder page at runtime if no real index.html is present.
package web

import "embed"

//go:embed all:dist
var DistFS embed.FS
