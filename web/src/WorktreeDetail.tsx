import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  createTerminal,
  deleteTerminal,
  listTerminals,
  TerminalSession,
} from "./api";
import Terminal from "./Terminal";

export default function WorktreeDetail() {
  const { repoId, worktreeId } = useParams<{
    repoId: string;
    worktreeId: string;
  }>();
  const [terminals, setTerminals] = useState<TerminalSession[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  function refresh() {
    if (!repoId || !worktreeId) return;
    listTerminals(repoId, worktreeId)
      .then((ts) => {
        setTerminals(ts);
        setActiveId((prev) => prev ?? ts[0]?.id ?? null);
      })
      .catch((err) => setError(err.message));
  }

  useEffect(refresh, [repoId, worktreeId]);

  async function handleNewTerminal() {
    if (!repoId || !worktreeId) return;
    try {
      const ts = await createTerminal(repoId, worktreeId);
      setTerminals((prev) => [...prev, ts]);
      setActiveId(ts.id);
    } catch (err) {
      setError((err as Error).message);
    }
  }

  async function handleCloseTerminal(ts: TerminalSession) {
    if (!repoId || !worktreeId) return;
    if (!confirm(`Close terminal "${ts.tab_label}"? The shell inside it will be terminated.`))
      return;
    try {
      await deleteTerminal(repoId, worktreeId, ts.id);
      setTerminals((prev) => {
        const next = prev.filter((t) => t.id !== ts.id);
        if (activeId === ts.id) setActiveId(next[0]?.id ?? null);
        return next;
      });
    } catch (err) {
      setError((err as Error).message);
    }
  }

  if (!repoId || !worktreeId) return null;

  return (
    <div className="container worktree-detail">
      <p>
        <Link className="repo-link" to={`/repo/${repoId}`}>
          ← worktrees
        </Link>
      </p>
      <div className="terminal-toolbar">
        <div className="terminal-tabs">
          {terminals.map((ts) => (
            <button
              key={ts.id}
              className={ts.id === activeId ? "tab active" : "tab"}
              onClick={() => setActiveId(ts.id)}
            >
              {ts.tab_label}
              <span
                className="tab-close"
                onClick={(e) => {
                  e.stopPropagation();
                  handleCloseTerminal(ts);
                }}
              >
                ×
              </span>
            </button>
          ))}
        </div>
        <div className="terminal-toolbar-actions">
          <button onClick={handleNewTerminal}>+ New Terminal</button>
          <button
            title="Open this worktree in a new browser tab"
            onClick={() => window.open(window.location.href, "_blank")}
          >
            ⧉ New tab
          </button>
        </div>
      </div>
      {error && <p className="error">{error}</p>}

      <div className="terminal-area">
        {terminals.length === 0 && (
          <p>No terminals yet — click "+ New Terminal" to start a shell in this worktree.</p>
        )}
        {terminals.map((ts) => (
          <div
            key={ts.id}
            style={{ display: ts.id === activeId ? "block" : "none", height: "100%" }}
          >
            <Terminal terminalId={ts.id} />
          </div>
        ))}
      </div>
    </div>
  );
}
