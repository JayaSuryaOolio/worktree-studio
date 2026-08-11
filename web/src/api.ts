// Thin client wrapper around worktree-studio's REST API.

export interface Repo {
  id: string;
  name: string;
  path: string;
}

// Not to be confused with WorktreeStatus below, which is git dirty/ahead-
// behind info — this is the worktree's own lifecycle state.
export type WorktreeLifecycle = "active" | "archived" | "deleted";

// "created" = made through worktree-studio's own "+ New worktree" flow;
// "imported" = an existing `git worktree` attached in via the repo settings
// page's attach flow.
export type WorktreeSource = "created" | "imported";

export interface Worktree {
  id: string;
  repo_id: string;
  name: string;
  branch: string;
  path: string;
  created_at: string;
  status: WorktreeLifecycle;
  source: WorktreeSource;
}

/** A `git worktree` that exists on disk for a repo but isn't tracked in
 * worktree-studio's DB yet — a candidate for attachWorktree(). */
export interface ExternalWorktreeEntry {
  path: string;
  branch: string;
}

export interface WorktreeWithRepo extends Worktree {
  repo_name: string;
}

export interface DependencyStatus {
  installed: boolean;
  detail?: string;
  install_hint?: string;
}

export type DependencyName = "tmux" | "spotlight" | "skill" | "claude_hook" | "vscode_cli";
export type DependencyStatusMap = Record<DependencyName, DependencyStatus>;

export interface TerminalSession {
  id: string;
  worktree_id: string;
  tmux_session_name: string;
  tab_label: string;
  created_at: string;
}

/** A TerminalSession joined with its worktree's branch/name — used by the
 * repo settings page's "Shells" tab. */
export interface TerminalSessionWithWorktree extends TerminalSession {
  worktree_branch: string;
  worktree_name: string;
}

export interface WorktreeStatus {
  branch: string;
  dirty: boolean;
  has_upstream: boolean;
  ahead: number;
  behind: number;
}

export interface AuditLogEntry {
  ts: string;
  event: string;
  [field: string]: unknown;
}

export interface SpotlightStatus {
  available: boolean;
  active: boolean;
  root?: string;
  active_worktree_path?: string;
  pid?: number;
}

export interface FileNode {
  name: string;
  path: string;
  type: "file" | "dir";
  children?: FileNode[];
}

export interface FileContent {
  path: string;
  content: string;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body?.error) message = body.error;
    } catch {
      // ignore non-JSON error bodies
    }
    if (res.status === 409) throw new ConflictError(message);
    throw new Error(message);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export function listRepos(): Promise<Repo[]> {
  return request<Repo[]>("/api/repos/");
}

export function addRepo(name: string, path: string): Promise<Repo> {
  return request<Repo>("/api/repos/", {
    method: "POST",
    body: JSON.stringify({ name, path }),
  });
}

export function listWorktrees(repoId: string): Promise<Worktree[]> {
  return request<Worktree[]>(`/api/repos/${repoId}/worktrees/`);
}

export function newNameSuggestion(repoId: string): Promise<string> {
  return request<{ name: string }>(
    `/api/repos/${repoId}/worktrees/new-name-suggestion`
  ).then((r) => r.name);
}

export function createWorktree(repoId: string, name: string): Promise<Worktree> {
  return request<Worktree>(`/api/repos/${repoId}/worktrees/`, {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

/** Registers an existing, already-on-disk git worktree of this repo — no
 * `git worktree add` is run, this is purely a registry insert for a
 * worktree created some other way (by hand, or by another tool). The
 * backend rejects a path that isn't actually one of this repo's real git
 * worktrees (a plain 400, not a ConflictError), and 409s only if that path
 * is already registered. `name` defaults server-side to `ext_<dirname>` if
 * omitted, so imported worktrees are visually distinguishable from ones
 * created through the normal flow. */
export function importWorktree(
  repoId: string,
  path: string,
  name?: string
): Promise<Worktree> {
  return request<Worktree>(`/api/repos/${repoId}/worktrees/import`, {
    method: "POST",
    body: JSON.stringify({ path, name }),
  });
}

export function deleteWorktree(
  repoId: string,
  worktreeId: string,
  force = false
): Promise<void> {
  const qs = force ? "?force=true" : "";
  return request<void>(`/api/repos/${repoId}/worktrees/${worktreeId}${qs}`, {
    method: "DELETE",
  });
}

/** Thrown by request() when the server responds 409 Conflict (used here to
 * mean: worktree has uncommitted changes, retry with force to discard them). */
export class ConflictError extends Error {}

/** Worktrees `git worktree list` reports for this repo that aren't tracked
 * in the DB yet — the repo settings page's "the rest" datagrid. */
export function listExternalWorktrees(repoId: string): Promise<ExternalWorktreeEntry[]> {
  return request<ExternalWorktreeEntry[]>(`/api/repos/${repoId}/worktrees/external`);
}

/** Imports an existing, on-disk `git worktree` into worktree-studio's DB. */
export function attachWorktree(repoId: string, path: string, branch?: string): Promise<Worktree> {
  return request<Worktree>(`/api/repos/${repoId}/worktrees/attach`, {
    method: "POST",
    body: JSON.stringify({ path, branch }),
  });
}

/** Every terminal session across every worktree under a repo, joined with
 * worktree branch/name — the repo settings page's "Shells" tab. */
export function listTerminalsForRepo(repoId: string): Promise<TerminalSessionWithWorktree[]> {
  return request<TerminalSessionWithWorktree[]>(`/api/repos/${repoId}/terminals/all`);
}

export function listTerminals(
  repoId: string,
  worktreeId: string
): Promise<TerminalSession[]> {
  return request<TerminalSession[]>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/terminals/`
  );
}

export function createTerminal(
  repoId: string,
  worktreeId: string,
  tabLabel?: string,
  initialCommand?: string,
  claudeSessionId?: string,
  claudeSessionTitle?: string
): Promise<TerminalSession> {
  return request<TerminalSession>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/terminals/`,
    {
      method: "POST",
      body: JSON.stringify({
        tab_label: tabLabel,
        initial_command: initialCommand,
        claude_session_id: claudeSessionId,
        claude_session_title: claudeSessionTitle,
      }),
    }
  );
}

/** Archives a worktree: a pure visibility flag, hides it from the normal
 * list. Does NOT touch git — the worktree/branch stay on disk, and any
 * claude session recorded against it stays resumable. See unarchiveWorktree
 * to reverse it, and deleteWorktree for the actual destructive removal. */
export function archiveWorktree(repoId: string, worktreeId: string): Promise<void> {
  return request<void>(`/api/repos/${repoId}/worktrees/${worktreeId}/archive`, {
    method: "POST",
  });
}

export function unarchiveWorktree(repoId: string, worktreeId: string): Promise<void> {
  return request<void>(`/api/repos/${repoId}/worktrees/${worktreeId}/unarchive`, {
    method: "POST",
  });
}

export function deleteTerminal(
  repoId: string,
  worktreeId: string,
  terminalId: string
): Promise<void> {
  return request<void>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/terminals/${terminalId}`,
    { method: "DELETE" }
  );
}

export function getWorktreeStatus(
  repoId: string,
  worktreeId: string
): Promise<WorktreeStatus> {
  return request<WorktreeStatus>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/status`
  );
}

export function getWorktreeAuditLog(
  repoId: string,
  worktreeId: string
): Promise<AuditLogEntry[]> {
  return request<AuditLogEntry[]>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/audit-log`
  );
}

export function getSpotlightStatus(
  repoId: string,
  worktreeId: string
): Promise<SpotlightStatus> {
  return request<SpotlightStatus>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/spotlight/`
  );
}

/** stash=true is the non-interactive equivalent of answering "yes" to the
 * spotlight CLI's own "stash and start?" prompt — this server-side call
 * has no controlling terminal to show that prompt on, so a dirty root
 * refuses (409/ConflictError) unless the caller passes stash=true, which
 * only happens after the user confirms it themselves in a browser prompt
 * (see startSpotlightWithFriendlyError in worktreeActions.ts). */
export function startSpotlight(
  repoId: string,
  worktreeId: string,
  stash = false
): Promise<{ root: string }> {
  const qs = stash ? "?stash=true" : "";
  return request<{ root: string }>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/spotlight/start${qs}`,
    { method: "POST" }
  );
}

export function stopSpotlight(repoId: string, worktreeId: string): Promise<void> {
  return request<void>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/spotlight/stop`,
    { method: "POST" }
  );
}

/** Builds the websocket URL for a terminal session, relative to the current
 * page so it works both from `vite dev` (proxied) and the production Go
 * server (served directly) without needing separate config. */
export function terminalWsUrl(terminalId: string): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws/terminals/${terminalId}`;
}

/** Builds the websocket URL for a worktree's file-change push (fsnotify-
 * driven external-change notifications) — same relative-URL trick as
 * terminalWsUrl above, so it works from both `vite dev` and the production
 * server unmodified. */
export function filesWsUrl(worktreeId: string): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws/files/${worktreeId}`;
}

/** Returns the saved dockview layout for a worktree, or `null` if nothing
 * has been saved yet (a 404, which is the normal/expected first-open
 * state here — not routed through request()'s generic error handling,
 * since a missing layout isn't an error condition for this call). */
export async function getWorktreeLayout(
  repoId: string,
  worktreeId: string
): Promise<unknown | null> {
  const res = await fetch(`/api/repos/${repoId}/worktrees/${worktreeId}/layout`);
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`);
  }
  return res.json();
}

export function saveWorktreeLayout(
  repoId: string,
  worktreeId: string,
  layout: unknown
): Promise<void> {
  return request<void>(`/api/repos/${repoId}/worktrees/${worktreeId}/layout`, {
    method: "PUT",
    body: JSON.stringify(layout),
  });
}

export function getFileTree(repoId: string, worktreeId: string): Promise<FileNode[]> {
  return request<FileNode[]>(`/api/repos/${repoId}/worktrees/${worktreeId}/files/tree`);
}

export function getFileContent(
  repoId: string,
  worktreeId: string,
  path: string
): Promise<FileContent> {
  return request<FileContent>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/files/content?path=${encodeURIComponent(path)}`
  );
}

export function openInVSCode(repoId: string, worktreeId: string): Promise<void> {
  return request<void>(`/api/repos/${repoId}/worktrees/${worktreeId}/open-in-vscode`, {
    method: "POST",
  });
}

export function saveFileContent(
  repoId: string,
  worktreeId: string,
  path: string,
  content: string
): Promise<void> {
  return request<void>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/files/content?path=${encodeURIComponent(path)}`,
    { method: "PUT", body: JSON.stringify({ content }) }
  );
}

/** Every worktree across every registered repo, any status — used by the
 * settings modal's "Worktrees" tab, not the normal per-repo views (which
 * default to active-only via listWorktrees). */
export function getAllWorktrees(): Promise<WorktreeWithRepo[]> {
  return request<WorktreeWithRepo[]>("/api/worktrees/all");
}

export function getDependencyStatus(): Promise<DependencyStatusMap> {
  return request<DependencyStatusMap>("/api/settings/dependencies");
}

export function installClaudeHook(): Promise<void> {
  return request<void>("/api/settings/dependencies/claude-hook/install", { method: "POST" });
}

export function uninstallClaudeHook(): Promise<void> {
  return request<void>("/api/settings/dependencies/claude-hook/uninstall", { method: "POST" });
}

export function installSkill(): Promise<void> {
  return request<void>("/api/settings/dependencies/skill/install", { method: "POST" });
}

/** Returns a claude session's human-readable title (its first real user
 * message, clipped), or `null` if no local transcript is found for that
 * session id — not routed through request()'s generic error handling
 * since a missing transcript is an expected, non-error outcome (e.g. the
 * session hasn't said anything yet, or was started somewhere this
 * machine's ~/.claude/projects doesn't have a record of). */
export async function getClaudeSessionTitle(sessionId: string): Promise<string | null> {
  const res = await fetch(`/api/claude-sessions/${sessionId}/title`);
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`);
  }
  const body = await res.json();
  return body.title as string;
}
