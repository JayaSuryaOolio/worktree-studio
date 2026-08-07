// Thin client wrapper around worktree-studio's REST API.

export interface Repo {
  id: string;
  name: string;
  path: string;
}

// Not to be confused with WorktreeStatus below, which is git dirty/ahead-
// behind info — this is the worktree's own lifecycle state.
export type WorktreeLifecycle = "active" | "archived" | "deleted";

export interface Worktree {
  id: string;
  repo_id: string;
  name: string;
  branch: string;
  path: string;
  created_at: string;
  status: WorktreeLifecycle;
}

export interface TerminalSession {
  id: string;
  worktree_id: string;
  tmux_session_name: string;
  tab_label: string;
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

export function startSpotlight(
  repoId: string,
  worktreeId: string
): Promise<{ root: string }> {
  return request<{ root: string }>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/spotlight/start`,
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
