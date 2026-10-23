//go:build !desktop

package main

import (
	"log/slog"
	"net"
	"net/http"
)

// serve is the web entry point: just the HTTP server, opened in a browser.
// The Wails desktop entry point lives in desktop.go (build tag `desktop`).
func serve(ln net.Listener, h http.Handler, _ *slog.Logger) error {
	return http.Serve(ln, h)
}
