import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import {
  ExternalWorktreeEntry,
  importWorktree,
  listExternalWorktrees,
  listTerminalsForRepo,
  TerminalSessionWithWorktree,
  Worktree,
} from "./api";
import { useRepoContext } from "./RepoContext";

type Tab = "worktrees" | "shells";

// The per-repo "settings" page: reached via the gear icon on the repo's
// sidebar row. Tab 1 is a bird's-eye view of every worktree git/DB knows
// about for this repo, split into three datagrids (local, imported, and
// "the rest" — real git worktrees not yet tracked here, with an attach
// action). Tab 2 lists every open shell across this repo's worktrees, with
// a deep link to jump straight to it.
export default function RepoSettings() {
  const { repoId } = useParams<{ repoId: string }>();
  const { repos } = useRepoContext();
  const [searchParams, setSearchParams] = useSearchParams();
  const tab: Tab = searchParams.get("tab") === "shells" ? "shells" : "worktrees";

  const repo = repos.find((r) => r.id === repoId);

  if (!repoId) return null;

  function setTab(next: Tab) {
    setSearchParams(next === "worktrees" ? {} : { tab: next });
  }

  return (
    <div className="container">
      <h1>{repo ? `${repo.name} — Settings` : "Repo settings"}</h1>

      <div className="settings-modal-tabs" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={tab === "worktrees"}
          className={tab === "worktrees" ? "active" : ""}
          onClick={() => setTab("worktrees")}
        >
          Worktrees
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === "shells"}
          className={tab === "shells" ? "active" : ""}
          onClick={() => setTab("shells")}
        >
          Open shells
        </button>
      </div>

      <div className="repo-settings-body">
        {tab === "worktrees" ? <WorktreesTab repoId={repoId} /> : <ShellsTab repoId={repoId} />}
      </div>
    </div>
  );
}

function WorktreesTab({ repoId }: { repoId: string }) {
  const { worktreesByRepo, worktreesLoading, refreshWorktrees } = useRepoContext();
  const [external, setExternal] = useState<ExternalWorktreeEntry[] | null>(null);
  const [externalError, setExternalError] = useState<string | null>(null);
  const [attaching, setAttaching] = useState<string | null>(null);
  const [attachError, setAttachError] = useState<string | null>(null);

  const worktrees = worktreesByRepo[repoId] ?? [];
  const local = worktrees.filter((w) => w.source !== "imported");
  const imported = worktrees.filter((w) => w.source === "imported");

  function refreshExternal() {
    listExternalWorktrees(repoId)
      .then(setExternal)
      .catch((err) => setExternalError((err as Error).message));
  }

  useEffect(() => {
    refreshExternal();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repoId]);

  async function handleAttach(entry: ExternalWorktreeEntry) {
    setAttaching(entry.path);
    setAttachError(null);
    try {
      await importWorktree(repoId, entry.path);
      refreshWorktrees();
      refreshExternal();
    } catch (err) {
      setAttachError((err as Error).message);
    } finally {
      setAttaching(null);
    }
  }

  return (
    <>
      <section className="settings-section">
        <h3>Local worktrees</h3>
        <p className="muted">Created here, through the normal "+ New worktree" flow.</p>
        <WorktreeTable worktrees={local} loading={worktreesLoading} emptyText="No local worktrees yet." />
      </section>

      <section className="settings-section">
        <h3>Imported worktrees</h3>
        <p className="muted">Existing git worktrees attached from below.</p>
        <WorktreeTable worktrees={imported} loading={worktreesLoading} emptyText="No imported worktrees yet." />
      </section>

      <section className="settings-section">
        <h3>Other git worktrees</h3>
        <p className="muted">
          Detected via <code>git worktree list</code> but not tracked here yet.
        </p>
        {externalError && <p className="error">{externalError}</p>}
        {attachError && <p className="error">{attachError}</p>}
        {external === null ? (
          <p className="muted">Loading…</p>
        ) : external.length === 0 ? (
          <p className="muted">Nothing to attach — git and worktree-studio agree.</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Branch</th>
                <th>Path</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {external.map((entry) => (
                <tr key={entry.path}>
                  <td>{entry.branch || <span className="muted">(detached)</span>}</td>
                  <td>
                    <code>{entry.path}</code>
                  </td>
                  <td>
                    <button type="button" disabled={attaching === entry.path} onClick={() => handleAttach(entry)}>
                      {attaching === entry.path ? "…" : "Attach"}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}

function WorktreeTable({
  worktrees,
  loading,
  emptyText,
}: {
  worktrees: Worktree[];
  loading: boolean;
  emptyText: string;
}) {
  if (loading) return <p className="muted">Loading…</p>;
  if (worktrees.length === 0) return <p className="muted">{emptyText}</p>;

  return (
    <table>
      <thead>
        <tr>
          <th>Branch</th>
          <th>Path</th>
          <th>Status</th>
          <th>Created</th>
        </tr>
      </thead>
      <tbody>
        {worktrees.map((wt) => (
          <tr key={wt.id}>
            <td>
              <Link to={`/repo/${wt.repo_id}/worktree/${wt.id}`}>{wt.branch}</Link>
            </td>
            <td>
              <code>{wt.path}</code>
            </td>
            <td>
              <span className={`badge badge-status-${wt.status}`}>{wt.status}</span>
            </td>
            <td>{new Date(wt.created_at).toLocaleString()}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function ShellsTab({ repoId }: { repoId: string }) {
  const [sessions, setSessions] = useState<TerminalSessionWithWorktree[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    listTerminalsForRepo(repoId)
      .then(setSessions)
      .catch((err) => setError((err as Error).message));
  }, [repoId]);

  if (error) return <p className="error">{error}</p>;
  if (sessions === null) return <p className="muted">Loading…</p>;
  if (sessions.length === 0) return <p className="muted">No open shells in this repo.</p>;

  return (
    <table>
      <thead>
        <tr>
          <th>Worktree</th>
          <th>Tab</th>
          <th>Created</th>
          <th />
        </tr>
      </thead>
      <tbody>
        {sessions.map((s) => (
          <tr key={s.id}>
            <td>{s.worktree_branch || s.worktree_name}</td>
            <td>{s.tab_label}</td>
            <td>{new Date(s.created_at).toLocaleString()}</td>
            <td>
              <Link to={`/repo/${repoId}/worktree/${s.worktree_id}?terminal=${s.id}`}>Open</Link>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
