import {
  createContext,
  ReactNode,
  useContext,
  useEffect,
  useState,
} from "react";
import { useMatch } from "react-router-dom";
import {
  getSpotlightStatus,
  getWorktreeStatus,
  listRepos,
  listWorktrees,
  Repo,
  SpotlightStatus,
  Worktree,
  WorktreeStatus,
} from "./api";

// How often to re-poll git/spotlight status for the sidebar + dashboard.
// A REST-polling loop rather than a ws push — see docs/architecture.md's
// documented simplification (no other consumer of a shared status-push
// channel exists). Moved here from Workspace.tsx so the sidebar and the
// Workspace page read the same fetched data instead of polling twice.
const STATUS_POLL_INTERVAL_MS = 5000;

interface RepoContextValue {
  repos: Repo[];
  reposLoading: boolean;
  reposError: string | null;
  refreshRepos: () => void;

  selectedRepoId: string | null;

  // Every registered repo's worktrees, keyed by repo id — the sidebar
  // nests each repo's worktrees under it, so it needs all of them, not
  // just the currently-selected repo's.
  worktreesByRepo: Record<string, Worktree[]>;
  worktreesLoading: boolean;
  worktreesError: string | null;
  refreshWorktrees: () => void;

  // Keyed by worktree id (globally unique), regardless of which repo it
  // belongs to — a flat map is enough since ids never collide across repos.
  gitStatus: Record<string, WorktreeStatus>;
  spotlightStatus: Record<string, SpotlightStatus>;
}

const RepoContext = createContext<RepoContextValue | null>(null);

export function useRepoContext(): RepoContextValue {
  const ctx = useContext(RepoContext);
  if (!ctx) {
    throw new Error("useRepoContext() must be used within a <RepoProvider>");
  }
  return ctx;
}

export function RepoProvider({ children }: { children: ReactNode }) {
  // useMatch works from anywhere under <Routes>, unlike useParams, which
  // only sees params for the matched route's own subtree — RepoProvider is
  // mounted in Layout.tsx, above the <Route> elements, so it needs this to
  // see :repoId regardless of which of the three routes is active.
  const match = useMatch("/repo/:repoId/*");
  const selectedRepoId = match?.params.repoId ?? null;

  const [repos, setRepos] = useState<Repo[]>([]);
  const [reposLoading, setReposLoading] = useState(true);
  const [reposError, setReposError] = useState<string | null>(null);

  const [worktreesByRepo, setWorktreesByRepo] = useState<Record<string, Worktree[]>>({});
  const [worktreesLoading, setWorktreesLoading] = useState(false);
  const [worktreesError, setWorktreesError] = useState<string | null>(null);

  const [gitStatus, setGitStatus] = useState<Record<string, WorktreeStatus>>({});
  const [spotlightStatus, setSpotlightStatus] = useState<Record<string, SpotlightStatus>>({});

  function refreshRepos() {
    setReposLoading(true);
    listRepos()
      .then((r) => {
        setRepos(r);
        setReposError(null);
      })
      .catch((err) => setReposError(err.message))
      .finally(() => setReposLoading(false));
  }

  useEffect(refreshRepos, []);

  // wt.repo_id is always present on a Worktree, so status calls don't need
  // a separately-threaded repoId — this is what lets refreshStatuses work
  // uniformly across every repo's worktrees at once.
  function refreshStatuses(wts: Worktree[]) {
    Promise.all(
      wts.map((wt) =>
        getWorktreeStatus(wt.repo_id, wt.id)
          .then((s) => [wt.id, s] as const)
          .catch(() => null)
      )
    ).then((entries) =>
      setGitStatus((prev) => ({
        ...prev,
        ...Object.fromEntries(entries.filter((e): e is [string, WorktreeStatus] => e !== null)),
      }))
    );
    Promise.all(
      wts.map((wt) =>
        getSpotlightStatus(wt.repo_id, wt.id)
          .then((s) => [wt.id, s] as const)
          .catch(() => [wt.id, { available: false, active: false }] as const)
      )
    ).then((entries) => setSpotlightStatus((prev) => ({ ...prev, ...Object.fromEntries(entries) })));
  }

  function refreshWorktrees() {
    if (repos.length === 0) {
      setWorktreesByRepo({});
      return;
    }
    setWorktreesLoading(true);
    Promise.all(
      repos.map((r) =>
        listWorktrees(r.id)
          .then((wts) => [r.id, wts] as const)
          .catch(() => [r.id, []] as const)
      )
    )
      .then((entries) => {
        setWorktreesByRepo(Object.fromEntries(entries));
        setWorktreesError(null);
        refreshStatuses(entries.flatMap(([, wts]) => wts));
      })
      .catch((err) => setWorktreesError(err.message))
      .finally(() => setWorktreesLoading(false));
  }

  // Re-fetch every repo's worktrees whenever the set of repos changes
  // (registering a new one, or the initial load finishing).
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(refreshWorktrees, [repos]);

  const allWorktrees = Object.values(worktreesByRepo).flat();

  // Poll status on an interval so dirty/ahead-behind/spotlight badges stay
  // current in both the sidebar and Workspace without a manual refresh —
  // doesn't re-fetch the worktree lists themselves, so an in-progress
  // dialog or terminal elsewhere isn't disrupted by this.
  useEffect(() => {
    if (allWorktrees.length === 0) return;
    const id = setInterval(() => refreshStatuses(allWorktrees), STATUS_POLL_INTERVAL_MS);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [worktreesByRepo]);

  const value: RepoContextValue = {
    repos,
    reposLoading,
    reposError,
    refreshRepos,
    selectedRepoId,
    worktreesByRepo,
    worktreesLoading,
    worktreesError,
    refreshWorktrees,
    gitStatus,
    spotlightStatus,
  };

  return <RepoContext.Provider value={value}>{children}</RepoContext.Provider>;
}
