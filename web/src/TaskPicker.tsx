import { useEffect, useState } from "react";
import { searchTasks, Task, TaskRef } from "./api";
import { taskKeyLabel } from "./tasks";

/** What the picker hands back: a TaskRef to send to the server, plus the
 * key/title the caller can show or pre-fill a worktree name from. */
export interface PickedTask {
  ref: TaskRef;
  key: string;
  title: string;
}

interface Props {
  value: PickedTask | null;
  onChange: (picked: PickedTask | null) => void;
  autoFocus?: boolean;
}

// "jira:PROJ-123" — an external key typed in directly, for when no adapter
// is configured for that source (any label works) or search misses it.
const EXTERNAL_KEY = /^([a-z][\w-]*):(\S+)$/i;

// The task field in the new-worktree dialog and the task popup. Searches
// tasks worktrees already carry (the common case: another worktree on a
// running task), then each configured source's adapter, and offers "+ New task" and "Use jira:PROJ-1" for the rest.
export default function TaskPicker({ value, onChange, autoFocus }: Props) {
  const [q, setQ] = useState("");
  const [results, setResults] = useState<Task[]>([]);

  useEffect(() => {
    if (value) return;
    let cancelled = false;
    const id = setTimeout(() => {
      searchTasks(q.trim())
        .then((r) => !cancelled && setResults(r))
        .catch(() => {});
    }, 120);
    return () => {
      cancelled = true;
      clearTimeout(id);
    };
  }, [q, value]);

  if (value) {
    return (
      <div className="task-picker-chosen">
        <span className="task-key">{value.key || "new"}</span>
        <span className="task-picker-title">{value.title}</span>
        {value.ref.source && value.ref.source !== "local" && (
          <span className="task-source">{value.ref.source}</span>
        )}
        <button type="button" aria-label="Clear task" onClick={() => onChange(null)}>
          ✕
        </button>
      </div>
    );
  }

  const text = q.trim();
  const external = EXTERNAL_KEY.exec(text);

  return (
    <div className="task-picker">
      <input
        type="text"
        value={q}
        placeholder="None (ad-hoc) — search, or type a title or jira:PROJ-1"
        aria-label="Task"
        autoFocus={autoFocus}
        onChange={(e) => setQ(e.target.value)}
        onKeyDown={(e) => {
          // Enter would otherwise submit the surrounding form with no task.
          if (e.key === "Enter") e.preventDefault();
        }}
      />
      <ul className="task-picker-results" role="listbox">
        {results.map((t) => (
          <li key={`${t.source}:${t.key}`}>
            <button
              type="button"
              role="option"
              aria-selected={false}
              onClick={() =>
                onChange({
                  ref: { source: t.source, key: t.key, title: t.title, url: t.url },
                  key: taskKeyLabel(t),
                  title: t.title,
                })
              }
            >
              <span className="task-key">{taskKeyLabel(t)}</span>
              <span className="task-picker-title">{t.title}</span>
              {t.source !== "local" && <span className="task-source">{t.source}</span>}
            </button>
          </li>
        ))}
        {external && (
          <li>
            <button
              type="button"
              role="option"
              aria-selected={false}
              onClick={() =>
                onChange({ ref: { source: external[1], key: external[2] }, key: external[2], title: "" })
              }
            >
              Use <span className="task-key">{external[2]}</span>
              <span className="task-source">{external[1]}</span>
            </button>
          </li>
        )}
        {text !== "" && !external && (
          <li>
            <button
              type="button"
              role="option"
              aria-selected={false}
              onClick={() => onChange({ ref: { new_title: text }, key: "", title: text })}
            >
              + New task “{text}”
            </button>
          </li>
        )}
      </ul>
    </div>
  );
}
