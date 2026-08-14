package claudehook

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	sessionTrackingID = "session-tracking"
	sessionContextID  = "session-context"
	// noServerURL is used by tests that execute the real generated script
	// but don't care about its best-effort server POST — port 1 is a
	// reserved port nothing binds to, so the connection is refused near-
	// instantly (swallowed by the script's own `|| true`) rather than
	// risking a real POST landing on whatever this machine actually has
	// listening on the default worktree-studio port.
	noServerURL = "http://127.0.0.1:1"
)

func withFakeClaudeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	// Resolve symlinks up front (macOS's TMPDIR lives under /var, itself a
	// symlink to /private/var) so HOME matches what a subprocess's own
	// `pwd`/getcwd() reports after chdir — otherwise a real machine never
	// sees this mismatch, but a test comparing "$HOME/..." prefixes against
	// a script's own pwd output would.
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", resolved)
	return resolved
}

func writeRealSettings(t *testing.T, home string, content string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readSettingsRaw(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v\n%s", err, data)
	}
	return m
}

// installAll installs every registered hook — the common case for tests
// that just need "both hooks present" set up rather than testing per-hook
// behavior directly.
func installAll(t *testing.T, serverBaseURL string) {
	t.Helper()
	for _, spec := range hookRegistry {
		if err := InstallHookByID(spec.id, serverBaseURL); err != nil {
			t.Fatalf("InstallHookByID(%q): %v", spec.id, err)
		}
	}
}

func TestHooksListsBothRegisteredHooks(t *testing.T) {
	infos := Hooks()
	if len(infos) != 2 {
		t.Fatalf("Hooks() returned %d entries, want 2", len(infos))
	}
	ids := map[string]bool{}
	for _, info := range infos {
		ids[info.ID] = true
		if info.Name == "" {
			t.Errorf("hook %q has an empty Name", info.ID)
		}
	}
	if !ids[sessionTrackingID] || !ids[sessionContextID] {
		t.Errorf("Hooks() = %+v, want both %q and %q", infos, sessionTrackingID, sessionContextID)
	}
}

func TestInstallHookByIDOnMissingSettingsFile(t *testing.T) {
	home := withFakeClaudeHome(t)

	installed, err := IsHookInstalledByID(sessionTrackingID)
	if err != nil {
		t.Fatalf("IsHookInstalledByID (no settings file): %v", err)
	}
	if installed {
		t.Fatal("expected not installed before InstallHookByID runs")
	}

	if err := InstallHookByID(sessionTrackingID, "http://localhost:8787"); err != nil {
		t.Fatalf("InstallHookByID: %v", err)
	}

	installed, err = IsHookInstalledByID(sessionTrackingID)
	if err != nil {
		t.Fatalf("IsHookInstalledByID: %v", err)
	}
	if !installed {
		t.Fatal("expected installed after InstallHookByID")
	}
	// The other hook must be unaffected — installing one is not installing all.
	if installed, err := IsHookInstalledByID(sessionContextID); err != nil || installed {
		t.Fatalf("session-context installed=%v err=%v, want false (untouched)", installed, err)
	}

	scriptPath, _ := scriptPath("session-start.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Errorf("audit hook script not written: %v", err)
	}

	settings := readSettingsRaw(t, home)
	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Fatalf("SessionStart entries = %d, want 1 (only the installed hook)", len(sessionStart))
	}
}

func TestInstallHookByIDUnknownID(t *testing.T) {
	withFakeClaudeHome(t)
	err := InstallHookByID("does-not-exist", "http://localhost:8787")
	if !errors.Is(err, ErrUnknownHook) {
		t.Fatalf("err = %v, want ErrUnknownHook", err)
	}
}

func TestInstallHookPreservesExistingUnrelatedSettings(t *testing.T) {
	home := withFakeClaudeHome(t)
	writeRealSettings(t, home, `{
		"model": "sonnet",
		"env": {"FOO": "bar"},
		"hooks": {
			"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "some-other-tool"}]}]
		}
	}`)

	installAll(t, "http://localhost:8787")

	settings := readSettingsRaw(t, home)
	if settings["model"] != "sonnet" {
		t.Errorf("model field was clobbered: %+v", settings)
	}
	env := settings["env"].(map[string]any)
	if env["FOO"] != "bar" {
		t.Errorf("env field was clobbered: %+v", settings)
	}
	hooks := settings["hooks"].(map[string]any)
	preToolUse := hooks["PreToolUse"].([]any)
	if len(preToolUse) != 1 {
		t.Fatalf("PreToolUse entries = %d, want 1 (must not be touched)", len(preToolUse))
	}
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 2 {
		t.Fatalf("SessionStart entries = %d, want 2", len(sessionStart))
	}
}

func TestInstallHookByIDIsIdempotent(t *testing.T) {
	withFakeClaudeHome(t)
	if err := InstallHookByID(sessionTrackingID, "http://localhost:8787"); err != nil {
		t.Fatalf("InstallHookByID (1st): %v", err)
	}
	if err := InstallHookByID(sessionTrackingID, "http://localhost:8787"); err != nil {
		t.Fatalf("InstallHookByID (2nd): %v", err)
	}

	installed, err := IsHookInstalledByID(sessionTrackingID)
	if err != nil || !installed {
		t.Fatalf("IsHookInstalledByID after double-install: installed=%v err=%v", installed, err)
	}

	// Re-reading the raw file to count entries directly (not just the
	// boolean) is the part that actually proves no duplicate entry was
	// appended on the second call.
	home := os.Getenv("HOME")
	settings := readSettingsRaw(t, home)
	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Fatalf("SessionStart entries after double-install = %d, want exactly 1 (idempotency broken)", len(sessionStart))
	}
}

func TestInstallHookByIDBacksUpExistingFile(t *testing.T) {
	home := withFakeClaudeHome(t)
	writeRealSettings(t, home, `{"model": "sonnet"}`)

	if err := InstallHookByID(sessionTrackingID, "http://localhost:8787"); err != nil {
		t.Fatalf("InstallHookByID: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(home, ".worktree-studio", "backups", "claude-settings-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("backup files = %d, want 1", len(matches))
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var backedUp map[string]any
	if err := json.Unmarshal(data, &backedUp); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}
	if backedUp["model"] != "sonnet" {
		t.Errorf("backup content = %+v, want the pre-install settings", backedUp)
	}
}

func TestUninstallHookByIDRemovesOnlyItsOwnEntry(t *testing.T) {
	home := withFakeClaudeHome(t)
	writeRealSettings(t, home, `{
		"hooks": {
			"SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "someone-elses-hook"}]}]
		}
	}`)

	installAll(t, "http://localhost:8787")
	settings := readSettingsRaw(t, home)
	hooks := settings["hooks"].(map[string]any)
	if len(hooks["SessionStart"].([]any)) != 3 {
		t.Fatalf("expected the pre-existing entry plus our 2, got %+v", hooks["SessionStart"])
	}

	if err := UninstallHookByID(sessionTrackingID); err != nil {
		t.Fatalf("UninstallHookByID: %v", err)
	}

	settings = readSettingsRaw(t, home)
	hooks = settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 2 {
		t.Fatalf("SessionStart entries after uninstalling one hook = %d, want 2 (the other tool's plus session-context)", len(sessionStart))
	}

	if installed, err := IsHookInstalledByID(sessionTrackingID); err != nil || installed {
		t.Fatalf("session-tracking installed=%v err=%v after its own uninstall, want false", installed, err)
	}
	if installed, err := IsHookInstalledByID(sessionContextID); err != nil || !installed {
		t.Fatalf("session-context installed=%v err=%v, want still true (untouched)", installed, err)
	}
}

func TestUninstallHookByIDNoOpWhenNotInstalled(t *testing.T) {
	withFakeClaudeHome(t)
	if err := UninstallHookByID(sessionTrackingID); err != nil {
		t.Fatalf("UninstallHookByID on a machine where it was never installed: %v", err)
	}
}

func TestUninstallHookByIDUnknownID(t *testing.T) {
	withFakeClaudeHome(t)
	err := UninstallHookByID("does-not-exist")
	if !errors.Is(err, ErrUnknownHook) {
		t.Fatalf("err = %v, want ErrUnknownHook", err)
	}
}

func TestAuditScriptHasNoContextOutput(t *testing.T) {
	withFakeClaudeHome(t)
	if err := InstallHookByID(sessionTrackingID, "http://localhost:8787"); err != nil {
		t.Fatalf("InstallHookByID: %v", err)
	}

	path, _ := scriptPath("session-start.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit hook script: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "pwd") || strings.Contains(content, "printf") {
		t.Errorf("audit script should only POST to the server, not print context (that's the context script's job):\n%s", content)
	}
	if !strings.Contains(content, "curl") || !strings.Contains(content, "exit 0") {
		t.Errorf("audit script missing expected curl POST / exit 0:\n%s", content)
	}
}

func TestContextScriptContentHasExpectedShape(t *testing.T) {
	withFakeClaudeHome(t)
	if err := InstallHookByID(sessionContextID, "http://localhost:8787"); err != nil {
		t.Fatalf("InstallHookByID: %v", err)
	}

	path, _ := scriptPath("session-context.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read context hook script: %v", err)
	}
	content := string(data)
	// The context script prints unconditionally, then best-effort logs
	// the same text to the server — but only when `jq` is present, so it
	// never hand-builds JSON for arbitrary PR-title text itself.
	if !strings.Contains(content, "command -v jq") {
		t.Errorf("context script's server POST must be gated on jq being available:\n%s", content)
	}
	for _, want := range []string{
		`context="Ooga. Claude wake up in cave (folder): $cwd"`,
		`.worktree-studio/*`,
		"git rev-parse --abbrev-ref HEAD",
		"gh pr list",
		`printf '%s\n' "$context"`,
		"curl",
		"http://localhost:8787/api/claude-hook-context",
		"exit 0",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("context script missing expected fragment %q:\n%s", want, content)
		}
	}
}

// runContextScript executes the installed context script with cwd set to
// dir and returns its stdout — the actual regression check for the
// git-branch detection logic, independent of whether `gh` happens to be
// installed on the machine running the test (that part is best-effort and
// silently skips itself when `gh` is absent, same as it would on a real
// machine without the GitHub CLI).
func runContextScript(t *testing.T, dir string) string {
	t.Helper()
	path, err := scriptPath("session-context.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", path)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+os.Getenv("HOME"))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run context script: %v", err)
	}
	return string(out)
}

// setUpWorktreeStudioGitRepo creates and commits a one-file git repo at
// filepath.Join(home, ".worktree-studio", "worktrees", "abc123", branch),
// checked out on branch — the shared fixture for tests that need the
// context script to detect a real current branch.
func setUpWorktreeStudioGitRepo(t *testing.T, home, branch string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := filepath.Join(home, ".worktree-studio", "worktrees", "abc123", branch)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"HOME="+home,
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q", "-b", branch)
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "f.txt")
	runGit("commit", "-q", "-m", "initial")
	return repo
}

func TestContextScriptReportsBranchInsideWorktreeStudioDir(t *testing.T) {
	home := withFakeClaudeHome(t)
	if err := InstallHookByID(sessionContextID, noServerURL); err != nil {
		t.Fatalf("InstallHookByID: %v", err)
	}
	repo := setUpWorktreeStudioGitRepo(t, home, "some-feature")

	out := runContextScript(t, repo)
	if !strings.Contains(out, "Ooga. Claude wake up in cave (folder): "+repo) {
		t.Errorf("output missing pwd line: %q", out)
	}
	if !strings.Contains(out, "Branch-mark say: some-feature") {
		t.Errorf("output missing branch line for a worktree-studio-managed dir: %q", out)
	}
}

func TestContextScriptSkipsBranchOutsideWorktreeStudioDir(t *testing.T) {
	home := withFakeClaudeHome(t)
	if err := InstallHookByID(sessionContextID, noServerURL); err != nil {
		t.Fatalf("InstallHookByID: %v", err)
	}

	elsewhere := filepath.Join(home, "just-a-normal-dir")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}

	out := runContextScript(t, elsewhere)
	if !strings.Contains(out, "Ooga. Claude wake up in cave (folder): "+elsewhere) {
		t.Errorf("output missing pwd line: %q", out)
	}
	if strings.Contains(out, "Branch-mark say:") {
		t.Errorf("should not report a branch outside a worktree-studio-managed dir: %q", out)
	}
}

// TestContextScriptPostsInjectedContextForLogging is the real regression
// test for "can I get logs into what context is injected... in the
// worktree logs?" — it runs the actual generated script (not just asserts
// on its source text) against a real local HTTP server and checks the
// POST body is exactly what a person would need to reconstruct what
// Claude saw.
func TestContextScriptPostsInjectedContextForLogging(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}

	var mu sync.Mutex
	var gotPath string
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	home := withFakeClaudeHome(t)
	if err := InstallHookByID(sessionContextID, ts.URL); err != nil {
		t.Fatalf("InstallHookByID: %v", err)
	}
	repo := setUpWorktreeStudioGitRepo(t, home, "some-feature")

	// The script's own curl call is synchronous and bounded (-m 2), so by
	// the time this returns the POST (if any) has already landed.
	runContextScript(t, repo)

	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/api/claude-hook-context" {
		t.Fatalf("posted path = %q, want /api/claude-hook-context (server never received a POST)", gotPath)
	}
	var payload struct {
		Cwd     string `json:"cwd"`
		Context string `json:"context"`
	}
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("posted body isn't valid JSON: %v\n%s", err, gotBody)
	}
	if payload.Cwd != repo {
		t.Errorf("posted cwd = %q, want %q", payload.Cwd, repo)
	}
	if !strings.Contains(payload.Context, "Ooga. Claude wake up in cave (folder): "+repo) {
		t.Errorf("posted context missing pwd line: %q", payload.Context)
	}
	if !strings.Contains(payload.Context, "Branch-mark say: some-feature") {
		t.Errorf("posted context missing branch line: %q", payload.Context)
	}
}

func TestReadSettingsRejectsInvalidJSONRatherThanClobbering(t *testing.T) {
	home := withFakeClaudeHome(t)
	writeRealSettings(t, home, `this is not json`)

	err := InstallHookByID(sessionTrackingID, "http://localhost:8787")
	if err == nil {
		t.Fatal("expected InstallHookByID to refuse to touch a settings.json that isn't valid JSON")
	}

	// Confirm the original (invalid, but real) content is untouched.
	data, readErr := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "this is not json" {
		t.Error("settings.json was modified despite the parse failure")
	}
}
