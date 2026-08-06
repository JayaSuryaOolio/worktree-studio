// Thin client wrapper around worktree-studio's REST API.

export interface Repo {
  id: string;
  name: string;
  path: string;
}

export interface Worktree {
  id: string;
  repo_id: string;
  name: string;
  branch: string;
  path: string;
  created_at: string;
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
  tabLabel?: string
): Promise<TerminalSession> {
  return request<TerminalSession>(
    `/api/repos/${repoId}/worktrees/${worktreeId}/terminals/`,
    { method: "POST", body: JSON.stringify({ tab_label: tabLabel }) }
  );
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
