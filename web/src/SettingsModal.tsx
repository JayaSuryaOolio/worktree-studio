import { useEffect, useState } from "react";
import {
  DependencyName,
  DependencyStatus,
  DependencyStatusMap,
  getDependencyStatus,
  getHooks,
  getServerLogs,
  HookStatus,
  installHook,
  installSkill,
  ServerLogs,
  uninstallHook,
} from "./api";
import { getStoredTheme, setTheme, Theme } from "./theme";

interface Props {
  onClose: () => void;
}

type Tab = "installation" | "appearance" | "logs";

// Three tabs: an installation/dependency-status page (tmux/spotlight
// detection, plus actionable install/uninstall for the claude hook and
// the skill), appearance (the dark/light theme switch — see theme.ts),
// and logs (this server's own recent error output, plus the log file's
// path — see internal/api/logs.go). The cross-repo worktree list that
// used to live here was removed per direct request — the per-repo
// settings page's own Worktrees tab already covers that.
export default function SettingsModal({ onClose }: Props) {
  const [tab, setTab] = useState<Tab>("installation");

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
            aria-selected={tab === "installation"}
            className={tab === "installation" ? "active" : ""}
            onClick={() => setTab("installation")}
          >
            Installation
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === "appearance"}
            className={tab === "appearance" ? "active" : ""}
            onClick={() => setTab("appearance")}
          >
            Appearance
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === "logs"}
            className={tab === "logs" ? "active" : ""}
            onClick={() => setTab("logs")}
          >
            Logs
          </button>
        </div>
        <div className="settings-modal-body">
          {tab === "installation" ? <InstallationTab /> : tab === "appearance" ? <AppearanceTab /> : <LogsTab />}
        </div>
      </div>
    </div>
  );
}

const DEPENDENCY_LABELS: Record<DependencyName, string> = {
  tmux: "tmux",
  spotlight: "Spotlight CLI",
  skill: "worktree-studio skill (global)",
  vscode_cli: "VS Code CLI (`code`)",
};

// tmux/spotlight are detection-only — this tool won't run `brew install`
// on someone's behalf, just show status + a hint. skill is actionable: a
// file write only worktree-studio itself owns
// (~/.claude/skills/worktree-studio/), so an install button here is safe
// to offer directly. Claude Code hooks are a separate, dynamic list below
// (see HooksSection) since there can be any number of them.
const ACTIONABLE: Partial<Record<DependencyName, boolean>> = {
  skill: true,
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

  return (
    <>
      {error && <p className="error">{error}</p>}
      {status === null ? (
        <p className="muted">Loading…</p>
      ) : (
        <ul className="dependency-list">
          {(Object.keys(DEPENDENCY_LABELS) as DependencyName[]).map((name) => (
            <DependencyRow
              key={name}
              name={name}
              label={DEPENDENCY_LABELS[name]}
              status={status[name]}
              busy={busy === name}
              onInstall={name === "skill" ? () => act(name, installSkill) : undefined}
            />
          ))}
        </ul>
      )}

      <section className="settings-section">
        <h3>Claude Code hooks</h3>
        <HooksSection />
      </section>
    </>
  );
}

function DependencyRow({
  name,
  label,
  status,
  busy,
  onInstall,
}: {
  name: DependencyName;
  label: string;
  status: DependencyStatus;
  busy: boolean;
  onInstall?: () => void;
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
      {ACTIONABLE[name] && !status.installed && (
        <span className="dependency-actions">
          <button type="button" disabled={busy} onClick={onInstall}>
            {busy ? "…" : "Install"}
          </button>
        </span>
      )}
    </li>
  );
}

// Renders one row per hook GET /api/settings/hooks returns — entirely
// driven by internal/claudehook's registry (see HookStatus), so adding a
// new hook there is the only change needed for a new row to appear here.
// Each hook installs/uninstalls independently of the others.
function HooksSection() {
  const [hooks, setHooks] = useState<HookStatus[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  function refresh() {
    return getHooks()
      .then(setHooks)
      .catch((err) => setError((err as Error).message));
  }

  useEffect(() => {
    refresh();
  }, []);

  async function act(id: string, fn: (id: string) => Promise<void>) {
    setBusy(id);
    setError(null);
    try {
      await fn(id);
      await refresh();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(null);
    }
  }

  if (error) return <p className="error">{error}</p>;
  if (hooks === null) return <p className="muted">Loading…</p>;

  return (
    <ul className="dependency-list">
      {hooks.map((hook) => (
        <li className="dependency-row" key={hook.id}>
          <span className={hook.installed ? "badge badge-clean" : "badge badge-dirty"}>
            {hook.installed ? "✓" : "✗"}
          </span>
          <span className="dependency-label">{hook.name}</span>
          <span className="dependency-detail muted">{hook.installed ? "" : hook.hint}</span>
          <span className="dependency-actions">
            {hook.installed ? (
              <button type="button" disabled={busy === hook.id} onClick={() => act(hook.id, uninstallHook)}>
                {busy === hook.id ? "…" : "Uninstall"}
              </button>
            ) : (
              <button type="button" disabled={busy === hook.id} onClick={() => act(hook.id, installHook)}>
                {busy === hook.id ? "…" : "Install"}
              </button>
            )}
          </span>
        </li>
      ))}
    </ul>
  );
}

const THEME_OPTIONS: { value: Theme; label: string; hint: string }[] = [
  { value: "dark", label: "Dark", hint: "Command Deck's default look." },
  { value: "light", label: "Light", hint: "Same layout, a light palette." },
];

// The theme itself lives outside React state — applyTheme/setTheme (see
// theme.ts) toggle a data-theme attribute on <html> that styles/tokens.css
// reads directly, so every already-mounted component re-themes for free
// without needing to re-render. This component's own state is just for
// which radio button shows as selected.
function AppearanceTab() {
  const [theme, setThemeState] = useState<Theme>(getStoredTheme);

  function handleChange(next: Theme) {
    setTheme(next);
    setThemeState(next);
  }

  return (
    <section className="settings-section">
      <h3>Theme</h3>
      <div className="theme-option-list" role="radiogroup" aria-label="Theme">
        {THEME_OPTIONS.map((opt) => (
          <label key={opt.value} className="theme-option">
            <input
              type="radio"
              name="theme"
              value={opt.value}
              checked={theme === opt.value}
              onChange={() => handleChange(opt.value)}
            />
            <span className="theme-option-label">{opt.label}</span>
            <span className="theme-option-hint">{opt.hint}</span>
          </label>
        ))}
      </div>
    </section>
  );
}

// This server's own recent ERROR-level output (internal/api/logs.go),
// plus the real file path — shown so a person can open/tail/grep it
// directly for anything this bounded, errors-only view leaves out.
function LogsTab() {
  const [logs, setLogs] = useState<ServerLogs | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  function refresh() {
    setRefreshing(true);
    setError(null);
    return getServerLogs()
      .then(setLogs)
      .catch((err) => setError((err as Error).message))
      .finally(() => setRefreshing(false));
  }

  useEffect(() => {
    refresh();
  }, []);

  return (
    <section className="settings-section">
      <div className="logs-tab-header">
        <h3>Application logs</h3>
        <button type="button" disabled={refreshing} onClick={refresh}>
          {refreshing ? "…" : "Refresh"}
        </button>
      </div>

      {error && <p className="error">{error}</p>}

      {logs === null ? (
        <p className="muted">Loading…</p>
      ) : (
        <>
          {logs.path ? (
            <p className="muted">
              Full log: <code>{logs.path}</code>
            </p>
          ) : (
            <p className="muted">No log file — this server has no durable log destination configured.</p>
          )}
          {logs.lines.length === 0 ? (
            <p className="muted">No errors logged yet.</p>
          ) : (
            <pre className="logs-tab-lines">{logs.lines.join("\n")}</pre>
          )}
        </>
      )}
    </section>
  );
}
