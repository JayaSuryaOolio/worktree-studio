import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import {
  ExternalWorktreeEntry,
  importWorktree,
  listArchivedWorktrees,
  listExternalWorktrees,
  listTerminalsForRepo,
  Repo,
  TerminalSessionWithWorktree,
  updateRepoBaseBranch,
  Worktree,
} from "./api";
import { useRepoContext } from "./RepoContext";
import { daysUntilAutoDelete, unarchiveWorktreeSafe } from "./worktreeActions";

type Tab = "general" | "worktrees" | "shells";

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
  const tabParam = searchParams.get("tab");
  const tab: Tab = tabParam === "shells" ? "shells" : tabParam === "worktrees" ? "worktrees" : "general";

  const repo = repos.find((r) => r.id === repoId);

  if (!repoId) return null;

  function setTab(next: Tab) {
    setSearchParams(next === "general" ? {} : { tab: next });
  }

  return (
    <div className="container">
      <h1>{repo ? `${repo.name} — Settings` : "Repo settings"}</h1>

      <div className="settings-modal-tabs" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={tab === "general"}
          className={tab === "general" ? "active" : ""}
          onClick={() => setTab("general")}
        >
          General
        </button>
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
        {tab === "worktrees" ? (
          <WorktreesTab repoId={repoId} />
        ) : tab === "shells" ? (
          <ShellsTab repoId={repoId} />
        ) : repo ? (
          <GeneralTab repo={repo} />
        ) : (
          <p className="muted">Loading…</p>
        )}
      </div>
    </div>
  );
}

// Base-branch override for "+ New worktree": git's own default for
// `git worktree add -b <branch> <path>` with no explicit start point is
// whatever the main checkout's HEAD happens to be at that moment — not
// necessarily the repo's actual main/default branch (e.g. if the main
// checkout is itself left on a feature branch). worktree-studio
// auto-detects a sensible default (origin's default branch, else local
// main/master — see internal/gitops.DetectDefaultBranch) when this is left
// blank, but a repo with an unconventional setup (no origin, an unusual
// default-branch name) may need this set explicitly.
function GeneralTab({ repo }: { repo: Repo }) {
  const { refreshRepos } = useRepoContext();
  const [baseBranch, setBaseBranch] = useState(repo.base_branch);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    setBaseBranch(repo.base_branch);
  }, [repo.base_branch]);

  async function handleSave() {
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      await updateRepoBaseBranch(repo.id, baseBranch.trim());
      refreshRepos();
      setSaved(true);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="settings-section">
      <h3>New worktree base branch</h3>
      <p className="muted">
        New worktrees branch off this. Leave blank to auto-detect (origin's default branch, else local
        main/master). This is only as fresh as the last time this repo's local branch was fetched/pulled — for a
        worktree that's always guaranteed up to date with the remote, prefix it with <code>origin/</code> (e.g.{" "}
        <code>origin/main</code>) instead of just <code>main</code>, or pick it per-worktree from the "Create from"
        dropdown when creating one.
      </p>
      <div className="button-with-icon" style={{ gap: "0.5rem" }}>
        <input
          type="text"
          placeholder="auto-detect"
          value={baseBranch}
          onChange={(e) => setBaseBranch(e.target.value)}
        />
        <button type="button" disabled={saving} onClick={handleSave}>
          {saving ? "Saving…" : "Save"}
        </button>
      </div>
      {error && <p className="error">{error}</p>}
      {saved && !error && <p className="muted">Saved.</p>}
    </section>
  );
}

function WorktreesTab({ repoId }: { repoId: string }) {
  const { worktreesByRepo, worktreesLoading, refreshWorktrees } = useRepoContext();
  const [external, setExternal] = useState<ExternalWorktreeEntry[] | null>(null);
  const [externalError, setExternalError] = useState<string | null>(null);
  const [attaching, setAttaching] = useState<string | null>(null);
  const [attachError, setAttachError] = useState<string | null>(null);
  const [archived, setArchived] = useState<Worktree[] | null>(null);
  const [archivedError, setArchivedError] = useState<string | null>(null);

  const worktrees = worktreesByRepo[repoId] ?? [];
  const local = worktrees.filter((w) => w.source !== "imported");
  const imported = worktrees.filter((w) => w.source === "imported");

  function refreshExternal() {
    listExternalWorktrees(repoId)
      .then(setExternal)
      .catch((err) => setExternalError((err as Error).message));
  }

  function refreshArchived() {
    listArchivedWorktrees(repoId)
      .then(setArchived)
      .catch((err) => setArchivedError((err as Error).message));
  }

  useEffect(() => {
    refreshExternal();
    refreshArchived();
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
        <h3>Archived worktrees</h3>
        <p className="muted">
          Hidden from the normal list, but the git worktree and branch are still on disk — unarchive to bring one
          back. Left archived too long, it's auto-deleted for good (git worktree removed, no DB record kept).
        </p>
        <ArchivedWorktreesTable
          worktrees={archived}
          error={archivedError}
          onUnarchived={() => {
            refreshWorktrees();
            refreshArchived();
          }}
        />
      </section>

      <section className="settings-section">
        <h3>Other git worktrees</h3>
        <p className="muted">
          Detected via <code>git worktree list</code> but not tracked here yet.
        </p>
        {attachError && <p className="error">{attachError}</p>}
        {externalError ? (
          <p className="error">{externalError}</p>
        ) : external === null ? (
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

function ArchivedWorktreesTable({
  worktrees,
  error,
  onUnarchived,
}: {
  worktrees: Worktree[] | null;
  error: string | null;
  onUnarchived: () => void;
}) {
  const [unarchiving, setUnarchiving] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  async function handleUnarchive(wt: Worktree) {
    setUnarchiving(wt.id);
    setActionError(null);
    await unarchiveWorktreeSafe(wt, { onDone: onUnarchived, onError: setActionError });
    setUnarchiving(null);
  }

  if (error) return <p className="error">{error}</p>;
  if (worktrees === null) return <p className="muted">Loading…</p>;
  if (worktrees.length === 0) return <p className="muted">No archived worktrees.</p>;

  return (
    <>
      {actionError && <p className="error">{actionError}</p>}
      <table>
        <thead>
          <tr>
            <th>Branch</th>
            <th>Path</th>
            <th>Auto-deletes in</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {worktrees.map((wt) => {
            const daysLeft = daysUntilAutoDelete(wt.archived_at);
            return (
              <tr key={wt.id}>
                <td>{wt.branch}</td>
                <td>
                  <code>{wt.path}</code>
                </td>
                <td>{daysLeft !== null && (daysLeft > 0 ? `${daysLeft} day(s)` : "any time now")}</td>
                <td>
                  <button type="button" disabled={unarchiving === wt.id} onClick={() => handleUnarchive(wt)}>
                    {unarchiving === wt.id ? "…" : "Unarchive"}
                  </button>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
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
          <th>Created from</th>
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
            <td>{wt.source_branch || <span className="muted">—</span>}</td>
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
