import { useState } from "react";
import { Link } from "react-router-dom";
import { TaskWithWorktrees, updateTask } from "./api";
import { useOpenNewWorktree } from "./newWorktreeRequest";
import { useRepoContext } from "./RepoContext";
import { sortTasks, taskId, taskKeyLabel } from "./tasks";

// The last column ("Done") only grows; past this many it folds the rest
// behind "show more".
const DONE_LIMIT = 20;

// Every task, one column per status (docs/tasks-plan.md). Drag a card to
// another column to change its status; archiving a task's last worktree
// moves it to the last column by itself.
export default function Board() {
  const { tasks, taskStatuses, repos, refreshTasks } = useRepoContext();
  const openNewWorktree = useOpenNewWorktree();
  const [error, setError] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<number | null>(null);
  const [showAllDone, setShowAllDone] = useState(false);

  const repoName = (id: string) => repos.find((r) => r.id === id)?.name ?? id;
  const sorted = sortTasks(tasks, taskStatuses);

  async function move(id: string, statusId: number) {
    const t = tasks.find((t) => taskId(t.source, t.key) === id);
    if (!t || t.status_id === statusId) return;
    setError(null);
    try {
      await updateTask(t.source, t.key, { status_id: statusId });
    } catch (err) {
      setError((err as Error).message);
    }
    refreshTasks();
  }

  function card(t: TaskWithWorktrees) {
    const id = taskId(t.source, t.key);
    const live = t.worktrees.filter((w) => w.status !== "deleted");
    return (
      <li
        key={id}
        className="board-card"
        draggable
        onDragStart={(e) => e.dataTransfer.setData("text/plain", id)}
      >
        <div className="board-card-head">
          <span className="task-key">{taskKeyLabel(t)}</span>
          {t.source !== "local" && <span className="task-source">{t.source}</span>}
          {t.url && (
            <a href={t.url} target="_blank" rel="noreferrer" title={t.url}>
              ↗
            </a>
          )}
          <button
            type="button"
            className="icon-button"
            title="New worktree for this task"
            aria-label={`New worktree for ${taskKeyLabel(t)}`}
            onClick={() =>
              openNewWorktree(live[0]?.repo_id ?? repos[0]?.id ?? "", {
                ref: { source: t.source, key: t.key },
                key: taskKeyLabel(t),
                title: t.title,
              })
            }
          >
            +
          </button>
        </div>
        {t.title && <div className="board-card-title">{t.title}</div>}
        <ul className="board-card-worktrees">
          {live.map((w) => (
            <li key={w.id} className={w.status === "archived" ? "archived" : undefined}>
              {w.status === "active" ? (
                <Link to={`/repo/${w.repo_id}/worktree/${w.id}`}>{w.name}</Link>
              ) : (
                <span title="Archived">{w.name}</span>
              )}
              <span className="task-source">{repoName(w.repo_id)}</span>
            </li>
          ))}
        </ul>
      </li>
    );
  }

  return (
    <div className="board">
      <h1 className="board-title">Tasks</h1>
      {error && <p className="error">{error}</p>}
      <div className="board-columns">
        {taskStatuses.map((s, i) => {
          const isLast = i === taskStatuses.length - 1;
          let column = sorted.filter((t) => t.status_id === s.id);
          const hidden = isLast && !showAllDone ? Math.max(0, column.length - DONE_LIMIT) : 0;
          if (hidden) column = column.slice(0, DONE_LIMIT);
          return (
            <section
              key={s.id}
              className={dropTarget === s.id ? "board-column drop-target" : "board-column"}
              onDragOver={(e) => {
                e.preventDefault();
                setDropTarget(s.id);
              }}
              onDragLeave={() => setDropTarget(null)}
              onDrop={(e) => {
                e.preventDefault();
                setDropTarget(null);
                move(e.dataTransfer.getData("text/plain"), s.id);
              }}
            >
              <h2>
                {s.name} <span className="board-count">{column.length + hidden}</span>
              </h2>
              <ul className="board-cards">{column.map(card)}</ul>
              {hidden > 0 && (
                <button type="button" className="board-more" onClick={() => setShowAllDone(true)}>
                  Show {hidden} more
                </button>
              )}
            </section>
          );
        })}
      </div>
      {tasks.length === 0 && (
        <p className="muted">
          No tasks yet. Pick or create one in the Task field when you start a worktree, or run{" "}
          <code>worktree-studio set-task</code> inside an existing one.
        </p>
      )}
    </div>
  );
}
