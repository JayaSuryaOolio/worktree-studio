import { useEffect, useState } from "react";
import {
  DependencyName,
  DependencyStatus,
  DependencyStatusMap,
  getAllWorktrees,
  getDependencyStatus,
  installClaudeHook,
  installSkill,
  uninstallClaudeHook,
  WorktreeWithRepo,
} from "./api";

interface Props {
  onClose: () => void;
}

type Tab = "worktrees" | "installation";

// Two tabs for now, per direct request: a cross-repo worktree list (any
// status — a bird's-eye view the per-repo pages don't give you), and an
// installation/dependency-status page (tmux/spotlight detection, plus
// actionable install/uninstall for the claude hook and the skill). A
// bulk-select/bulk-delete datagrid on the worktrees tab is a separate,
// larger PLAN.md TODO — this is the read-only list it'll eventually grow
// into, not that feature itself.
export default function SettingsModal({ onClose }: Props) {
  const [tab, setTab] = useState<Tab>("worktrees");

  return (
    <div className="dialog-backdrop" onClick={onClose}>
      <div className="dialog settings-modal" onClick={(e) => e.stopPropagation()}>
        <div className="settings-modal-header">
          <h2 style={{ margin: 0 }}>Settings</h2>
          <button type="button" onClick={onClose} aria-label="Close settings">
            ✕
          </button>
        </div>
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
            aria-selected={tab === "installation"}
            className={tab === "installation" ? "active" : ""}
            onClick={() => setTab("installation")}
          >
            Installation
          </button>
        </div>
        <div className="settings-modal-body">
          {tab === "worktrees" ? <WorktreesTab /> : <InstallationTab />}
        </div>
      </div>
    </div>
  );
}

function WorktreesTab() {
  const [worktrees, setWorktrees] = useState<WorktreeWithRepo[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getAllWorktrees()
      .then(setWorktrees)
      .catch((err) => setError((err as Error).message));
  }, []);

  if (error) return <p className="error">{error}</p>;
  if (worktrees === null) return <p className="muted">Loading…</p>;
  if (worktrees.length === 0) return <p className="muted">No worktrees created yet, in any repo.</p>;

  return (
    <table>
      <thead>
        <tr>
          <th>Repo</th>
          <th>Branch</th>
          <th>Path</th>
          <th>Status</th>
          <th>Created</th>
        </tr>
      </thead>
      <tbody>
        {worktrees.map((wt) => (
          <tr key={wt.id}>
            <td>{wt.repo_name}</td>
            <td>{wt.branch}</td>
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

const DEPENDENCY_LABELS: Record<DependencyName, string> = {
  tmux: "tmux",
  spotlight: "Spotlight CLI",
  skill: "worktree-studio skill (global)",
  claude_hook: "Claude session-tracking hook",
  vscode_cli: "VS Code CLI (`code`)",
};

// tmux/spotlight are detection-only — this tool won't run `brew install`
// on someone's behalf, just show status + a hint. skill/claude_hook are
// actionable: both are file writes only worktree-studio itself owns
// (~/.claude/skills/worktree-studio/, and one marked entry in
// ~/.claude/settings.json's hooks.SessionStart — see
// internal/claudehook/install.go for the safety rationale), so an
// install/uninstall button here is safe to offer directly.
const ACTIONABLE: Partial<Record<DependencyName, boolean>> = {
  skill: true,
  claude_hook: true,
};

function InstallationTab() {
  const [status, setStatus] = useState<DependencyStatusMap | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<DependencyName | null>(null);

  function refresh() {
    return getDependencyStatus()
      .then(setStatus)
      .catch((err) => setError((err as Error).message));
  }

  useEffect(() => {
    refresh();
  }, []);

  async function act(name: DependencyName, fn: () => Promise<void>) {
    setBusy(name);
    setError(null);
    try {
      await fn();
      await refresh();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(null);
    }
  }

  if (error) return <p className="error">{error}</p>;
  if (status === null) return <p className="muted">Loading…</p>;

  return (
    <ul className="dependency-list">
      {(Object.keys(DEPENDENCY_LABELS) as DependencyName[]).map((name) => (
        <DependencyRow
          key={name}
          name={name}
          label={DEPENDENCY_LABELS[name]}
          status={status[name]}
          busy={busy === name}
          onInstall={
            name === "claude_hook"
              ? () => act(name, installClaudeHook)
              : name === "skill"
                ? () => act(name, installSkill)
                : undefined
          }
          onUninstall={name === "claude_hook" ? () => act(name, uninstallClaudeHook) : undefined}
        />
      ))}
    </ul>
  );
}

function DependencyRow({
  name,
  label,
  status,
  busy,
  onInstall,
  onUninstall,
}: {
  name: DependencyName;
  label: string;
  status: DependencyStatus;
  busy: boolean;
  onInstall?: () => void;
  onUninstall?: () => void;
}) {
  return (
    <li className="dependency-row">
      <span className={status.installed ? "badge badge-clean" : "badge badge-dirty"}>
        {status.installed ? "✓" : "✗"}
      </span>
      <span className="dependency-label">{label}</span>
      <span className="dependency-detail muted">
        {status.installed ? status.detail : status.install_hint}
      </span>
      {ACTIONABLE[name] && (
        <span className="dependency-actions">
          {status.installed ? (
            onUninstall && (
              <button type="button" disabled={busy} onClick={onUninstall}>
                {busy ? "…" : "Uninstall"}
              </button>
            )
          ) : (
            <button type="button" disabled={busy} onClick={onInstall}>
              {busy ? "…" : "Install"}
            </button>
          )}
        </span>
      )}
    </li>
  );
}
