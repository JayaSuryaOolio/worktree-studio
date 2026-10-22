//go:build desktop

package main

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// Launched from Finder/Dock the app inherits launchd's minimal PATH, which
// lacks tmux/git/gh/claude. Prepend the usual install locations.
// ponytail: fixed list, not the user's login-shell PATH; read it from `$SHELL -lc` if a tool is still missing.
func init() {
	home, _ := os.UserHomeDir()
	extra := []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".local", "bin")}
	os.Setenv("PATH", strings.Join(extra, ":")+":"+os.Getenv("PATH"))
}

// serve is the Wails desktop entry point: the same HTTP server as the web
// build keeps running on its loopback listener, and a native window renders
// it. Wails' asset handler can't carry websockets, so HTTP goes through a
// reverse proxy and the page is told the real address to open its websockets
// against (web/src/desktop.ts wsOrigin). Clipboard and link opening go
// through Wails' runtime (web/src/desktop.ts), which is what avoids the
// browser clipboard-permission problems in docs/terminal-clipboard.md.
// Build: go build -tags desktop,production ./cmd/worktree-studio
func serve(ln net.Listener, h http.Handler, logger *slog.Logger) error {
	go func() {
		if err := http.Serve(ln, h); err != nil {
			logger.Error("server exited", "err", err)
		}
	}()

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("listener address %q has no port", ln.Addr())
	}
	target := &url.URL{Scheme: "http", Host: "127.0.0.1:" + port}
	inject := []byte(`<script>window.__WTS_WS_BASE__="ws://127.0.0.1:` + port + `"</script></head>`)

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	proxy.ModifyResponse = func(resp *http.Response) error {
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
			return nil
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		body = bytes.Replace(body, []byte("</head>"), inject, 1)
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return nil
	}

	return wails.Run(&options.App{
		Title:       "worktree-studio",
		Width:       1440,
		Height:      900,
		AssetServer: &assetserver.Options{Handler: proxy},
	})
}
