// Command worktree-studio runs the local HTTP server for worktree-studio: a
// small dashboard over git worktrees for a registry of repos, backed by a
// SQLite store and a JSONL audit log. See PLAN.md and docs/architecture.md.
package main

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"worktree-studio/internal/api"
	"worktree-studio/internal/audit"
	"worktree-studio/internal/files"
	"worktree-studio/internal/store"
	"worktree-studio/internal/term"
	webembed "worktree-studio/web"
)

const defaultAddr = ":8787"

// addrPort extracts ":<port>" from a listen address like ":8787",
// "0.0.0.0:9000", or "localhost:8787" — used to build a "http://localhost:
// <port>" URL for the installed claude hook script, regardless of what
// host `addr` itself binds to (the hook always runs on this same machine).
func addrPort(addr string) string {
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[i:]
	}
	return ":" + addr
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	st, err := store.OpenDefault()
	if err != nil {
		logger.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	al, err := audit.NewDefault()
	if err != nil {
		logger.Error("open audit log", "err", err)
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		logger.Error("resolve home dir", "err", err)
		os.Exit(1)
	}
	worktreeRoot := filepath.Join(home, ".worktree-studio", "worktrees")
	if err := os.MkdirAll(worktreeRoot, 0o755); err != nil {
		logger.Error("create worktree root", "err", err)
		os.Exit(1)
	}

	if dropped, err := term.Reconcile(st); err != nil {
		logger.Warn("reconcile terminal sessions against live tmux sessions", "err", err)
	} else if dropped > 0 {
		logger.Info("pruned stale terminal session rows (tmux session no longer live)", "count", dropped)
	}

	addr := defaultAddr
	if v := os.Getenv("WORKTREE_STUDIO_ADDR"); v != "" {
		addr = v
	}

	srv := &api.Server{
		Store:        st,
		Audit:        al,
		Term:         &term.Manager{Store: st, Audit: al},
		Files:        files.NewManager(),
		WorktreeRoot: worktreeRoot,
		Log:          logger,
		// The hook always runs on the same machine as the server (a
		// script Claude Code invokes as a local subprocess), so localhost
		// is correct regardless of what host `addr` itself binds to.
		SelfBaseURL: "http://localhost" + addrPort(addr),
	}

	// Backfills the synthetic root worktree (see api.Server.ensureRootWorktree)
	// for repos registered before it existed — new repos get theirs at
	// add-repo time instead, this only ever does real work once per repo.
	if repos, err := st.ListRepos(); err != nil {
		logger.Warn("list repos for root-worktree backfill", "err", err)
	} else {
		for _, repo := range repos {
			srv.EnsureRootWorktree(repo)
		}
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	srv.Routes(r)
	mountFrontend(r, logger)

	logger.Info("worktree-studio listening", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Error("server exited", "err", err)
		os.Exit(1)
	}
}

// mountFrontend serves the embedded web/dist build if it actually contains
// a built app (i.e. has an index.html), and otherwise serves a small
// friendly placeholder page instead of 404s/crashes — this is the "graceful
// fallback if web/dist doesn't exist yet" behavior.
func mountFrontend(r chi.Router, logger *slog.Logger) {
	sub, err := fs.Sub(webembed.DistFS, "dist")
	if err != nil {
		logger.Warn("no embedded web/dist available, serving placeholder", "err", err)
		r.NotFound(placeholderHandler)
		return
	}

	if _, err := fs.Stat(sub, "index.html"); err != nil {
		logger.Warn("web/dist has no built frontend yet (run: cd web && bun install && bun run build), serving placeholder")
		r.NotFound(placeholderHandler)
		return
	}

	fileServer := http.FileServer(http.FS(sub))
	r.Handle("/*", spaHandler(sub, fileServer))
}

// spaHandler serves static files when they exist, and falls back to
// index.html otherwise so client-side routing (React Router paths like
// /repo/:id) works on a hard refresh.
func spaHandler(fsys fs.FS, fileServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "/" {
			trimmed := path[1:]
			if _, err := fs.Stat(fsys, trimmed); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

const placeholderHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>worktree-studio</title></head>
<body style="font-family: system-ui, sans-serif; max-width: 40rem; margin: 4rem auto; line-height: 1.5;">
<h1>worktree-studio</h1>
<p>The API server is running, but the frontend hasn't been built yet.</p>
<p>Run:</p>
<pre style="background:#eee; padding: 1rem;">cd web &amp;&amp; bun install &amp;&amp; bun run build</pre>
<p>then restart this server, or run <code>bun run dev</code> in <code>web/</code> for a hot-reloading dev server proxying to this API.</p>
</body>
</html>`

func placeholderHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(placeholderHTML))
}
