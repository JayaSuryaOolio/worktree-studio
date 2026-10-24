package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"worktree-studio/tui"
)

// The `wts` launcher: this same binary (installed once, symlinked as `wts`)
// starts and stops its own pieces as detached background processes.
//
//	wts start|stop|restart     backend (API + embedded UI); `stop` stops everything
//	wts web start|stop         standalone web server proxying to the backend
//	wts desktop start|stop     native window on the backend (macOS build)
//	wts tui                    terminal UI, in the foreground
//	wts attach <terminal-id>   this terminal becomes that terminal tab
//	wts ptyd                   PTY daemon (normally auto-started)
//	wts ptyd restart           restart it on this build, tabs restored
//	wts status
//
// Pids live in ~/.worktree-studio/run/<name>.pid, logs next to the data.

const usageText = `usage: wts <command>
  start | stop | restart     backend (stop also stops web and desktop)
  web start | stop           web server (needs the backend)
  desktop start | stop       desktop window (needs the backend)
  tui                        terminal UI (needs the backend)
  attach <terminal-id>       attach this terminal to a terminal tab (Ctrl+\ detaches)
  ptyd                       PTY daemon for the ptyd terminal backend (auto-started)
  ptyd restart               restart the PTY daemon on this build (tabs come back)
  status                     what is running, where, and where the logs are`

const defaultWebAddr = ":8700"

func wtsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".worktree-studio")
}

func logPathFor(name string) string { return filepath.Join(wtsDir(), name+".log") }

func pidPath(name string) string { return filepath.Join(wtsDir(), "run", name+".pid") }

// pidOf returns the live pid recorded for name, or 0.
func pidOf(name string) int {
	b, err := os.ReadFile(pidPath(name))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid > 0 && syscall.Kill(pid, 0) == nil {
		return pid
	}
	return 0
}

// spawn re-executes this binary with args as a detached background process
// (own session, so it outlives the terminal) and records its pid. logName ""
// discards output (the backend already mirrors its own log to server.log).
func spawn(name, logName string, args ...string) (int, error) {
	self, err := os.Executable()
	if err != nil {
		return 0, err
	}
	out := os.DevNull
	if logName != "" {
		out = logPathFor(logName)
	}
	f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	cmd := exec.Command(self, args...)
	cmd.Stdout, cmd.Stderr = f, f
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(pidPath(name)), 0o755); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, os.WriteFile(pidPath(name), []byte(strconv.Itoa(pid)), 0o644)
}

// stopProc terminates name's process (SIGTERM, then SIGKILL after 3s).
func stopProc(name string) bool {
	pid := pidOf(name)
	if pid == 0 {
		os.Remove(pidPath(name))
		return false
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	for i := 0; i < 30 && syscall.Kill(pid, 0) == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	os.Remove(pidPath(name))
	return true
}

// waitFor polls cond for up to 10s.
func waitFor(cond func() bool) bool {
	for i := 0; i < 50; i++ {
		if cond() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// probe returns the address recorded in file if something answers there.
func probe(file, path string) string {
	b, err := os.ReadFile(filepath.Join(wtsDir(), file))
	if err != nil {
		return ""
	}
	addr := strings.TrimSpace(string(b))
	resp, err := (&http.Client{Timeout: time.Second}).Get("http://" + addr + path)
	if err != nil {
		return ""
	}
	resp.Body.Close()
	return addr
}

func runningServer() string { return probe("server.addr", "/api/repos") }
func runningWeb() string    { return probe("web.addr", "/") }

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func needBackend() string {
	addr := runningServer()
	if addr == "" {
		die("backend is not running — run `wts start` first")
	}
	return addr
}

func runWtsCommand(args []string) bool {
	if len(args) == 0 {
		// Bare `wts` shouldn't start a foreground server; bare
		// `worktree-studio` still does (see docs/running-locally.md).
		if filepath.Base(os.Args[0]) == "wts" {
			fmt.Println(usageText)
			return true
		}
		return false
	}
	switch args[0] {
	case "start":
		backendStart()
	case "stop":
		for _, n := range []string{"desktop", "web", "backend"} {
			stopOne(n)
		}
	case "restart":
		stopOne("backend")
		backendStart()
	case "status":
		status()
	case "tui":
		os.Setenv("WORKTREE_STUDIO_ADDR", needBackend())
		if err := tui.Run(); err != nil {
			die("tui: %v", err)
		}
	case "web", "desktop":
		if len(args) != 2 || (args[1] != "start" && args[1] != "stop") {
			die("usage: wts %s start|stop", args[0])
		}
		if args[1] == "stop" {
			stopOne(args[0])
		} else if args[0] == "web" {
			webStart()
		} else {
			desktopStart()
		}
	case "attach":
		if len(args) != 2 {
			die("usage: wts attach <terminal-id>")
		}
		attachTerminal(args[1])
	case "ptyd":
		if len(args) == 2 && args[1] == "restart" {
			ptydRestart()
		} else {
			ptydServe()
		}
	case "web-serve": // internal: the web process
		webServe()
	case "desktop-run": // internal: the desktop process
		if err := desktopRun(needBackend()); err != nil {
			die("desktop: %v", err)
		}
	default:
		return false // incl. "backend": falls through to the foreground server
	}
	return true
}

func stopOne(name string) {
	if stopProc(name) {
		fmt.Println(name, "stopped")
	} else if name == "backend" && runningServer() != "" {
		fmt.Println("backend is running but wasn't started by `wts start`; stop it where it runs")
	} else {
		fmt.Println(name, "not running")
	}
	if name == "backend" {
		os.Remove(filepath.Join(wtsDir(), "server.addr"))
	}
	if name == "web" {
		os.Remove(filepath.Join(wtsDir(), "web.addr"))
	}
}

func backendStart() {
	if addr := runningServer(); addr != "" {
		fmt.Printf("backend already running: http://%s\nlogs: %s\n", addr, logPathFor("server"))
		return
	}
	pid, err := spawn("backend", "", "backend")
	if err != nil {
		die("start backend: %v", err)
	}
	if !waitFor(func() bool { return runningServer() != "" }) {
		die("backend did not come up; see %s", logPathFor("server"))
	}
	fmt.Printf("backend started (pid %d): http://%s\nlogs: %s\n", pid, runningServer(), logPathFor("server"))
}

func webStart() {
	needBackend()
	if addr := runningWeb(); addr != "" {
		fmt.Printf("web already running: http://%s\nlogs: %s\n", addr, logPathFor("web"))
		return
	}
	pid, err := spawn("web", "web", "web-serve")
	if err != nil {
		die("start web: %v", err)
	}
	if !waitFor(func() bool { return runningWeb() != "" }) {
		die("web did not come up; see %s", logPathFor("web"))
	}
	fmt.Printf("web started (pid %d): http://%s\nlogs: %s\n", pid, runningWeb(), logPathFor("web"))
}

func desktopStart() {
	needBackend()
	if pidOf("desktop") != 0 {
		fmt.Printf("desktop already running\nlogs: %s\n", logPathFor("desktop"))
		return
	}
	pid, err := spawn("desktop", "desktop", "desktop-run")
	if err != nil {
		die("start desktop: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
	if pidOf("desktop") == 0 {
		die("desktop exited immediately; see %s", logPathFor("desktop"))
	}
	fmt.Printf("desktop started (pid %d)\nlogs: %s\n", pid, logPathFor("desktop"))
}

func status() {
	row := func(name, addr string, pid int) {
		switch {
		case addr != "":
			fmt.Printf("%-8s running  http://%s  pid %d\n", name, addr, pid)
		case pid != 0:
			fmt.Printf("%-8s running  pid %d\n", name, pid)
		default:
			fmt.Printf("%-8s stopped\n", name)
		}
	}
	row("backend", runningServer(), pidOf("backend"))
	row("web", runningWeb(), pidOf("web"))
	row("desktop", "", pidOf("desktop"))
	fmt.Printf("logs: %s (server.log, web.log, desktop.log)\n", wtsDir())
}

// webServe is the web process: the embedded UI, with /api and /ws proxied
// to the backend. The target is re-read per request so a backend restart
// onto another port doesn't strand it.
func webServe() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	addr := defaultWebAddr
	if v := os.Getenv("WORKTREE_STUDIO_WEB_ADDR"); v != "" {
		addr = v
	}
	ln, addr, err := listenFirstFree(addr, logger)
	if err != nil {
		die("no free port: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtsDir(), "web.addr"), []byte("localhost"+addrPort(addr)), 0o644); err != nil {
		die("write web.addr: %v", err)
	}
	proxy := &httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			b, _ := os.ReadFile(filepath.Join(wtsDir(), "server.addr"))
			pr.SetURL(&url.URL{Scheme: "http", Host: strings.TrimSpace(string(b))})
		},
	}
	r := chi.NewRouter()
	r.Handle("/api/*", proxy)
	r.Handle("/ws/*", proxy)
	mountFrontend(r, logger)
	logger.Info("web listening", "addr", addr)
	if err := http.Serve(ln, r); err != nil {
		die("web exited: %v", err)
	}
}
