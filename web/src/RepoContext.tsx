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

  worktrees: Worktree[];
  worktreesLoading: boolean;
  worktreesError: string | null;
  refreshWorktrees: () => void;

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

  const [worktrees, setWorktrees] = useState<Worktree[]>([]);
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

  function refreshStatuses(wts: Worktree[], repoId: string) {
    Promise.all(
      wts.map((wt) =>
        getWorktreeStatus(repoId, wt.id)
          .then((s) => [wt.id, s] as const)
          .catch(() => null)
      )
    ).then((entries) =>
      setGitStatus(
        Object.fromEntries(entries.filter((e): e is [string, WorktreeStatus] => e !== null))
      )
    );
    Promise.all(
      wts.map((wt) =>
        getSpotlightStatus(repoId, wt.id)
          .then((s) => [wt.id, s] as const)
          .catch(() => [wt.id, { available: false, active: false }] as const)
      )
    ).then((entries) => setSpotlightStatus(Object.fromEntries(entries)));
  }

  function refreshWorktrees() {
    if (!selectedRepoId) {
      setWorktrees([]);
      setGitStatus({});
      setSpotlightStatus({});
      return;
    }
    setWorktreesLoading(true);
    listWorktrees(selectedRepoId)
      .then((wts) => {
        setWorktrees(wts);
        setWorktreesError(null);
        refreshStatuses(wts, selectedRepoId);
      })
      .catch((err) => setWorktreesError(err.message))
      .finally(() => setWorktreesLoading(false));
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(refreshWorktrees, [selectedRepoId]);

  // Poll status on an interval so dirty/ahead-behind/spotlight badges stay
  // current in both the sidebar and Workspace without a manual refresh —
  // doesn't re-fetch the worktree list itself, so an in-progress dialog or
  // terminal elsewhere isn't disrupted by this.
  useEffect(() => {
    if (!selectedRepoId || worktrees.length === 0) return;
    const id = setInterval(() => refreshStatuses(worktrees, selectedRepoId), STATUS_POLL_INTERVAL_MS);
    return () => clearInterval(id);
  }, [selectedRepoId, worktrees]);

  const value: RepoContextValue = {
    repos,
    reposLoading,
    reposError,
    refreshRepos,
    selectedRepoId,
    worktrees,
    worktreesLoading,
    worktreesError,
    refreshWorktrees,
    gitStatus,
    spotlightStatus,
  };

  return <RepoContext.Provider value={value}>{children}</RepoContext.Provider>;
}
