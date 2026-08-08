import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SettingsModal from "./SettingsModal";

vi.mock("./api", () => ({
  getAllWorktrees: vi.fn(),
  getDependencyStatus: vi.fn(),
  installClaudeHook: vi.fn(),
  uninstallClaudeHook: vi.fn(),
  installSkill: vi.fn(),
}));

import {
  getAllWorktrees,
  getDependencyStatus,
  installClaudeHook,
  installSkill,
  uninstallClaudeHook,
} from "./api";

const worktrees = [
  {
    id: "w1",
    repo_id: "r1",
    repo_name: "adelaide",
    name: "feature",
    branch: "feature",
    path: "/tmp/adelaide-wt/feature",
    created_at: "2026-01-01T00:00:00Z",
    status: "active" as const,
  },
];

const depsAllMissing = {
  tmux: { installed: false, install_hint: "brew install tmux" },
  spotlight: { installed: false, install_hint: "see docs" },
  skill: { installed: false, install_hint: "install from here" },
  claude_hook: { installed: false, install_hint: "install from here" },
  vscode_cli: { installed: false, install_hint: "install from here" },
};

beforeEach(() => {
  vi.mocked(getAllWorktrees).mockResolvedValue(worktrees);
  vi.mocked(getDependencyStatus).mockResolvedValue(depsAllMissing);
  vi.mocked(installClaudeHook).mockResolvedValue(undefined);
  vi.mocked(uninstallClaudeHook).mockResolvedValue(undefined);
  vi.mocked(installSkill).mockResolvedValue(undefined);
});

describe("SettingsModal", () => {
  it("shows the Worktrees tab by default with a cross-repo table", async () => {
    render(<SettingsModal onClose={() => {}} />);
    expect(await screen.findByText("adelaide")).toBeInTheDocument();
    expect(screen.getByText("feature")).toBeInTheDocument();
    expect(getAllWorktrees).toHaveBeenCalled();
  });

  it("switches to the Installation tab and shows dependency status", async () => {
    const user = userEvent.setup();
    render(<SettingsModal onClose={() => {}} />);
    await user.click(screen.getByRole("tab", { name: "Installation" }));

    expect(await screen.findByText("tmux")).toBeInTheDocument();
    expect(screen.getByText("Claude session-tracking hook")).toBeInTheDocument();
    expect(screen.getByText("worktree-studio skill (global)")).toBeInTheDocument();
  });

  it("installs the claude hook from its own row and refreshes status", async () => {
    const user = userEvent.setup();
    render(<SettingsModal onClose={() => {}} />);
    await user.click(screen.getByRole("tab", { name: "Installation" }));
    const hookLabel = await screen.findByText("Claude session-tracking hook");
    const hookRow = hookLabel.closest("li")!;

    vi.mocked(getDependencyStatus).mockResolvedValue({
      ...depsAllMissing,
      claude_hook: { installed: true },
    });

    await user.click(within(hookRow).getByRole("button", { name: "Install" }));

    expect(installClaudeHook).toHaveBeenCalledTimes(1);
    expect(await within(hookRow).findByRole("button", { name: "Uninstall" })).toBeInTheDocument();
  });

  it("calls onClose when the close button is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<SettingsModal onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: /close settings/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
