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
import {
  getStoredTheme,
  resolveMode,
  setTheme,
  ThemeChoice,
  ThemeFamily,
  ThemeMode,
  THEME_FAMILIES,
  THEME_FAMILY_BLURBS,
  THEME_FAMILY_LABELS,
} from "./theme";
import { notificationPermission, requestNotificationPermission } from "./attentionNotify";
import { getNotificationsEnabled, setNotificationsEnabled } from "./notificationPreference";

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

// The theme itself lives outside React state — applyTheme/setTheme (see
// theme.ts) stamp data-theme/data-mode on <html> that styles/tokens.css
// reads directly, so every already-mounted component re-themes for free
// without needing to re-render. This component's own state only tracks
// which option shows as chosen.
//
// This replaced a sun/moon pill toggle labelled "Dark", which never said
// whether "Dark" was the current state or the thing the switch would do
// to you. Two labelled radio groups say it outright — and the family
// options preview themselves, since a theme name means nothing until you
// can see it.
function AppearanceTab() {
  const [choice, setChoice] = useState<ThemeChoice>(getStoredTheme);

  function pick(next: ThemeChoice) {
    setTheme(next);
    setChoice(next);
  }

  const modes: { value: ThemeMode; label: string }[] = [
    { value: "dark", label: "Dark" },
    { value: "light", label: "Light" },
    { value: "system", label: "System" },
  ];

  return (
    <section className="settings-section">
      <h3>Theme</h3>
      <div className="theme-picker" role="radiogroup" aria-label="Theme">
        {THEME_FAMILIES.map((family) => (
          <button
            key={family}
            type="button"
            role="radio"
            aria-checked={choice.family === family}
            className={`theme-option${choice.family === family ? " selected" : ""}`}
            onClick={() => pick({ ...choice, family })}
          >
            <ThemeSwatch family={family} mode={choice.mode} />
            <span className="theme-option-text">
              <span className="theme-option-name">{THEME_FAMILY_LABELS[family]}</span>
              <span className="theme-option-blurb">{THEME_FAMILY_BLURBS[family]}</span>
            </span>
          </button>
        ))}
      </div>

      <h3>Mode</h3>
      <div className="segmented" role="radiogroup" aria-label="Mode">
        {modes.map((m) => (
          <button
            key={m.value}
            type="button"
            role="radio"
            aria-checked={choice.mode === m.value}
            className={choice.mode === m.value ? "selected" : ""}
            onClick={() => pick({ ...choice, mode: m.value })}
          >
            {m.label}
          </button>
        ))}
      </div>
      <p className="settings-hint">
        {choice.mode === "system"
          ? "Following the operating system, and switching live when it does."
          : `Always ${choice.mode}, regardless of the operating system.`}
      </p>

      <NotificationsSection />
    </section>
  );
}

// A miniature of the sidebar, rendered with the candidate palette's own
// tokens rather than hand-copied hex values: tokens.css declares each
// palette on a plain [data-theme][data-mode] selector (not :root), so
// stamping those two attributes on this element gives it the real thing.
// A palette can't drift out of sync with its own preview.
function ThemeSwatch({ family, mode }: { family: ThemeFamily; mode: ThemeMode }) {
  return (
    <span
      className="theme-swatch"
      data-theme={family}
      data-mode={resolveMode(mode)}
      aria-hidden="true"
    >
      <span className="theme-swatch-row selected">
        <span className="theme-swatch-bar" />
      </span>
      <span className="theme-swatch-row">
        <span className="theme-swatch-bar short" />
        <span className="theme-swatch-dot" />
      </span>
      <span className="theme-swatch-row">
        <span className="theme-swatch-bar" />
      </span>
    </span>
  );
}

// A "claude session needs your input" badge always shows in the sidebar
// (see Sidebar.tsx's attention dot) regardless of this setting — this
// toggle is only about the extra desktop-level nudge (sound + a real OS
// notification) on top of that. On by default (notificationPreference.ts)
// — RepoContext.tsx already requests browser permission proactively on
// load, so most people never need to touch this switch at all; it's here
// for turning the feature off, or for a browser (e.g. Safari) that
// declined to grant permission without a real click, in which case this
// button doubles as that click.
function NotificationsSection() {
  const [enabled, setEnabled] = useState(getNotificationsEnabled);
  const [permission, setPermission] = useState(notificationPermission);

  async function handleToggle() {
    const next = !enabled;
    setNotificationsEnabled(next);
    setEnabled(next);
    if (next && permission === "default") {
      setPermission(await requestNotificationPermission());
    }
  }

  return (
    <>
      <h3>Notifications</h3>
      {permission === "unsupported" ? (
        <p className="muted">Desktop notifications aren't supported in this browser.</p>
      ) : (
        <>
          <div className="theme-switch-row">
            <button
              type="button"
              role="switch"
              aria-checked={enabled}
              aria-label="Desktop notifications"
              className="theme-switch"
              onClick={handleToggle}
            >
              <span className="theme-switch-thumb" />
            </button>
            <span className="theme-switch-label">
              {enabled ? "On" : "Off"} — desktop notification (+ sound) when a claude session needs
              your input in a worktree you're not viewing
            </span>
          </div>
          {enabled && permission === "denied" && (
            <p className="muted">
              Notifications are blocked at the browser level — the sidebar's own badge and sound
              still work, but check your browser's site settings to also get a real desktop
              notification.
            </p>
          )}
        </>
      )}
    </>
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
