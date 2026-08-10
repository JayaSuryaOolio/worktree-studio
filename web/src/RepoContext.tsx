import {
  createContext,
  ReactNode,
  useContext,
  useEffect,
  useRef,
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
import { StatusScheduler } from "./statusScheduler";

// How often a "hot" (recently focused) worktree's git/spotlight status gets
// a background refresh. Not something a human notices lagging by a few
// seconds, so this is loose rather than tight.
const STATUS_POLL_INTERVAL_MS = 15_000;

// How long a worktree stays eligible for that background refresh after its
// last touch (see statusScheduler.ts) before it's excluded from the tick
// loop entirely. It keeps showing its last-known badge either way — this
// just stops spending a `git status` subprocess on a worktree nobody's
// looked at in an hour. Re-touched (and refreshed) the moment it's
// re-focused.
const FOCUS_IDLE_TIMEOUT_MS = 60 * 60 * 1000;

// While a worktree's detail page is open, re-touch it well inside
// FOCUS_IDLE_TIMEOUT_MS so a long-lived single viewing session doesn't
// silently age out of the background-refresh set.
const FOCUS_KEEPALIVE_MS = 60_000;

// If a worktree is focused (its detail page is opened) and its cached
// status is older than this, an immediate on-demand refresh is kicked off
// rather than waiting for the next background tick — this is also the
// threshold WorktreeDetail-adjacent UI uses to decide whether to show a
// brief "refreshing" indicator instead of silently serving stale data.
const STATUS_STALE_MS = 10_000;

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

  // True while a background/on-demand git-status refresh for that worktree
  // id is in flight — lets a "just focused, showing stale data" indicator
  // clear itself the moment fresh data lands, instead of a fixed timeout.
  statusRefreshing: Record<string, boolean>;
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

  // "Focused" per the product decision behind this scheduler: a worktree
  // counts as actively requested only while its own detail page is open,
  // not merely by appearing in the sidebar's always-rendered list.
  const worktreeMatch = useMatch("/repo/:repoId/worktree/:worktreeId");
  const focusedWorktreeId = worktreeMatch?.params.worktreeId ?? null;

  const [repos, setRepos] = useState<Repo[]>([]);
  const [reposLoading, setReposLoading] = useState(true);
  const [reposError, setReposError] = useState<string | null>(null);

  const [worktreesByRepo, setWorktreesByRepo] = useState<Record<string, Worktree[]>>({});
  const [worktreesLoading, setWorktreesLoading] = useState(false);
  const [worktreesError, setWorktreesError] = useState<string | null>(null);

  const [gitStatus, setGitStatus] = useState<Record<string, WorktreeStatus>>({});
  const [spotlightStatus, setSpotlightStatus] = useState<Record<string, SpotlightStatus>>({});
  const [statusRefreshing, setStatusRefreshing] = useState<Record<string, boolean>>({});

  // Kept in sync with worktreesByRepo below — lets the schedulers' fetchers
  // resolve a bare worktree id to the repo_id the status endpoints need,
  // without forcing the schedulers themselves to be recreated (and losing
  // their cached state) every time the worktree list changes.
  const worktreeByIdRef = useRef<Record<string, Worktree>>({});

  const gitSchedulerRef = useRef<StatusScheduler<WorktreeStatus> | null>(null);
  if (gitSchedulerRef.current === null) {
    gitSchedulerRef.current = new StatusScheduler<WorktreeStatus>(
      (id) => {
        const wt = worktreeByIdRef.current[id];
        if (!wt) return Promise.reject(new Error(`unknown worktree ${id}`));
        return getWorktreeStatus(wt.repo_id, id);
      },
      { intervalMs: STATUS_POLL_INTERVAL_MS, idleTimeoutMs: FOCUS_IDLE_TIMEOUT_MS }
    );
  }
  const spotlightSchedulerRef = useRef<StatusScheduler<SpotlightStatus> | null>(null);
  if (spotlightSchedulerRef.current === null) {
    spotlightSchedulerRef.current = new StatusScheduler<SpotlightStatus>(
      (id) => {
        const wt = worktreeByIdRef.current[id];
        if (!wt) return Promise.reject(new Error(`unknown worktree ${id}`));
        return getSpotlightStatus(wt.repo_id, id);
      },
      { intervalMs: STATUS_POLL_INTERVAL_MS, idleTimeoutMs: FOCUS_IDLE_TIMEOUT_MS }
    );
  }

  useEffect(() => {
    return () => {
      gitSchedulerRef.current?.dispose();
      spotlightSchedulerRef.current?.dispose();
    };
  }, []);

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
      })
      .catch((err) => setWorktreesError(err.message))
      .finally(() => setWorktreesLoading(false));
  }

  // Re-fetch every repo's worktrees whenever the set of repos changes
  // (registering a new one, or the initial load finishing).
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(refreshWorktrees, [repos]);

  // Keep worktree lookups current for the schedulers' fetchers, and
  // subscribe every known worktree to both schedulers so this context
  // receives (and re-renders on) every status update they produce.
  //
  // Subscribing here does NOT itself keep a worktree in the actively-
  // polled set — see statusScheduler.ts's touch()/idleTimeoutMs policy.
  // It only registers this listener and, for a worktree whose status has
  // never been fetched at all, triggers that one bootstrap fetch — so
  // every worktree shows *something* once, but only a focused one (below)
  // keeps getting re-fetched on the interval.
  useEffect(() => {
    const wts = Object.values(worktreesByRepo).flat();
    worktreeByIdRef.current = Object.fromEntries(wts.map((wt) => [wt.id, wt]));

    const gitScheduler = gitSchedulerRef.current!;
    const spotlightScheduler = spotlightSchedulerRef.current!;

    const unsubscribes = wts.flatMap((wt) => [
      gitScheduler.subscribe(wt.id, (snapshot) => {
        setGitStatus((prev) => (snapshot.data ? { ...prev, [wt.id]: snapshot.data as WorktreeStatus } : prev));
        setStatusRefreshing((prev) =>
          prev[wt.id] === snapshot.refreshing ? prev : { ...prev, [wt.id]: snapshot.refreshing }
        );
      }),
      spotlightScheduler.subscribe(wt.id, (snapshot) => {
        setSpotlightStatus((prev) => (snapshot.data ? { ...prev, [wt.id]: snapshot.data as SpotlightStatus } : prev));
      }),
    ]);

    return () => unsubscribes.forEach((unsubscribe) => unsubscribe());
  }, [worktreesByRepo]);

  // The actual "focus" policy: touch (and keep touching, for as long as
  // the detail page stays open) the currently-viewed worktree so it stays
  // in the background-refresh set for the next hour; if its cached status
  // is already stale when focused, refresh it immediately rather than
  // waiting for the next tick.
  useEffect(() => {
    if (!focusedWorktreeId) return;
    const gitScheduler = gitSchedulerRef.current!;
    const spotlightScheduler = spotlightSchedulerRef.current!;

    function keepAlive() {
      gitScheduler.touch(focusedWorktreeId!);
      spotlightScheduler.touch(focusedWorktreeId!);
    }
    keepAlive();

    const staleCutoff = Date.now() - STATUS_STALE_MS;
    const gitSnapshot = gitScheduler.peek(focusedWorktreeId);
    if (!gitSnapshot?.fetchedAt || gitSnapshot.fetchedAt < staleCutoff) {
      void gitScheduler.refreshNow(focusedWorktreeId);
    }
    const spotlightSnapshot = spotlightScheduler.peek(focusedWorktreeId);
    if (!spotlightSnapshot?.fetchedAt || spotlightSnapshot.fetchedAt < staleCutoff) {
      void spotlightScheduler.refreshNow(focusedWorktreeId);
    }

    const keepAliveId = setInterval(keepAlive, FOCUS_KEEPALIVE_MS);
    return () => clearInterval(keepAliveId);
  }, [focusedWorktreeId]);

  // Pause both schedulers' background heartbeats while the tab is hidden —
  // there's no badge to keep fresh if nothing's rendering it. Resuming
  // catches up immediately rather than waiting out a full interval.
  useEffect(() => {
    function handleVisibilityChange() {
      const paused = document.hidden;
      gitSchedulerRef.current?.setPaused(paused);
      spotlightSchedulerRef.current?.setPaused(paused);
    }
    handleVisibilityChange();
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => document.removeEventListener("visibilitychange", handleVisibilityChange);
  }, []);

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
    statusRefreshing,
  };

  return <RepoContext.Provider value={value}>{children}</RepoContext.Provider>;
}
