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

export function deleteWorktree(repoId: string, worktreeId: string): Promise<void> {
  return request<void>(`/api/repos/${repoId}/worktrees/${worktreeId}`, {
    method: "DELETE",
  });
}
