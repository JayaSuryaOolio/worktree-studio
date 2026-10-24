import { useState } from "react";
import { Link } from "react-router-dom";
import { setWorktreeTask, TaskRef, updateTask, Worktree } from "./api";
import { useOpenNewWorktree } from "./newWorktreeRequest";
import { useRepoContext } from "./RepoContext";
import TaskPicker from "./TaskPicker";
import { findTask, taskKeyLabel } from "./tasks";

interface Props {
  worktree: Worktree;
  onClose: () => void;
}

// Opened from the task icon (breadcrumb, sidebar card): the worktree's task
// — status, title, link, the other worktrees on it — or a picker to label
// an ad-hoc worktree with one.
export default function TaskPopup({ worktree, onClose }: Props) {
  const { tasks, taskStatuses, repos, refreshTasks, refreshWorktrees } = useRepoContext();
  const openNewWorktree = useOpenNewWorktree();
  const [error, setError] = useState<string | null>(null);
  const task = findTask(tasks, worktree);

  async function run(fn: () => Promise<unknown>, refresh = refreshTasks): Promise<boolean> {
    setError(null);
    try {
      await fn();
      refresh();
      return true;
    } catch (err) {
      setError((err as Error).message);
      return false;
    }
  }

  const setTask = (ref: TaskRef) =>
    run(() => setWorktreeTask(worktree.repo_id, worktree.id, ref), refreshWorktrees);

  const repoName = (id: string) => repos.find((r) => r.id === id)?.name ?? id;

  return (
    <div className="dialog-backdrop" onClick={onClose}>
      <div className="dialog task-popup" onClick={(e) => e.stopPropagation()}>
        {!worktree.task_key ? (
          <>
            <h2>Set task for {worktree.name}</h2>
            <TaskPicker value={null} onChange={(p) => p && setTask(p.ref).then((ok) => ok && onClose())} autoFocus />
          </>
        ) : !task ? (
          <h2>
            <span className="task-key">{worktree.task_key}</span> loading…
          </h2>
        ) : (
          <>
            <h2>
              <span className="task-key">{taskKeyLabel(task)}</span>
              {task.source !== "local" && <span className="task-source"> {task.source}</span>}
            </h2>

            <label htmlFor="task-status">Status</label>
            <select
              id="task-status"
              value={task.status_id}
              onChange={(e) => run(() => updateTask(task.source, task.key, { status_id: Number(e.target.value) }))}
            >
              {taskStatuses.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>

            {/* key= so a refresh from elsewhere resets the uncontrolled
                field; saved on blur, like a rename. */}
            <label htmlFor="task-title">Title</label>
            <input
              id="task-title"
              type="text"
              key={`t:${task.title}`}
              defaultValue={task.title}
              placeholder={task.key}
              onBlur={(e) =>
                e.target.value !== task.title && run(() => updateTask(task.source, task.key, { title: e.target.value }))
              }
            />

            <label htmlFor="task-url">Link</label>
            <div className="task-popup-url">
              <input
                id="task-url"
                type="text"
                key={`u:${task.url}`}
                defaultValue={task.url}
                placeholder="https://…"
                onBlur={(e) =>
                  e.target.value !== task.url && run(() => updateTask(task.source, task.key, { url: e.target.value }))
                }
              />
              {task.url && (
                <a href={task.url} target="_blank" rel="noreferrer">
                  Open ↗
                </a>
              )}
            </div>

            <label>Worktrees</label>
            <ul className="task-popup-worktrees">
              {task.worktrees
                .filter((w) => w.status === "active")
                .map((w) => (
                  <li key={w.id}>
                    <Link to={`/repo/${w.repo_id}/worktree/${w.id}`} onClick={onClose}>
                      {w.name}
                    </Link>
                    <span className="task-source">{repoName(w.repo_id)}</span>
                  </li>
                ))}
            </ul>

            <div className="actions">
              <button type="button" onClick={() => setTask({}).then((ok) => ok && onClose())}>
                Remove task from this worktree
              </button>
              <button
                type="button"
                onClick={() => {
                  onClose();
                  openNewWorktree(worktree.repo_id, {
                    ref: { source: task.source, key: task.key },
                    key: taskKeyLabel(task),
                    title: task.title,
                  });
                }}
              >
                + Worktree for this task
              </button>
            </div>
          </>
        )}
        {error && <p className="error">{error}</p>}
      </div>
    </div>
  );
}

// "#12 Invoice export ›" ahead of the branch name in the worktree header —
// `task(source) > worktree`. Clicking opens the popup; an ad-hoc worktree
// shows a faint "+ task" instead.
export function TaskCrumb({ worktree }: { worktree: Worktree }) {
  const { tasks } = useRepoContext();
  const [open, setOpen] = useState(false);
  const task = findTask(tasks, worktree);

  return (
    <>
      <button
        type="button"
        className={worktree.task_key ? "task-crumb" : "task-crumb task-crumb-empty"}
        title={worktree.task_key ? "Task" : "Set task"}
        onClick={() => setOpen(true)}
      >
        {worktree.task_key ? (
          <>
            <span className="task-key">{task ? taskKeyLabel(task) : worktree.task_key}</span>
            {task?.title && <span className="task-crumb-title">{task.title}</span>}
            {worktree.task_source !== "local" && <span className="task-source">{worktree.task_source}</span>}
            <span className="task-crumb-sep">›</span>
          </>
        ) : (
          "+ task"
        )}
      </button>
      {open && <TaskPopup worktree={worktree} onClose={() => setOpen(false)} />}
    </>
  );
}
