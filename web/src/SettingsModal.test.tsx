import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SettingsModal from "./SettingsModal";

vi.mock("./api", () => ({
  getDependencyStatus: vi.fn(),
  getHooks: vi.fn(),
  installHook: vi.fn(),
  uninstallHook: vi.fn(),
  installSkill: vi.fn(),
  getServerLogs: vi.fn(),
}));

import { getDependencyStatus, getHooks, getServerLogs, installHook, installSkill, uninstallHook } from "./api";

const depsAllMissing = {
  tmux: { installed: false, install_hint: "brew install tmux" },
  spotlight: { installed: false, install_hint: "see docs" },
  skill: { installed: false, install_hint: "install from here" },
  vscode_cli: { installed: false, install_hint: "install from here" },
};

const hooksAllMissing = [
  { id: "session-tracking", name: "Claude session-tracking hook", hint: "install from here", installed: false },
  { id: "session-context", name: "Claude worktree-context hook", hint: "install from here", installed: false },
];

beforeEach(() => {
  vi.mocked(getDependencyStatus).mockResolvedValue(depsAllMissing);
  vi.mocked(getHooks).mockResolvedValue(hooksAllMissing);
  vi.mocked(installHook).mockResolvedValue(undefined);
  vi.mocked(uninstallHook).mockResolvedValue(undefined);
  vi.mocked(installSkill).mockResolvedValue(undefined);
  vi.mocked(getServerLogs).mockResolvedValue({
    path: "/home/user/.worktree-studio/server.log",
    lines: ['time=2026-01-01T00:00:00Z level=ERROR msg="list repos" err="boom"'],
  });
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-mode");
});

afterEach(() => {
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-mode");
});

describe("SettingsModal", () => {
  it("shows the Installation tab by default with dependency status", async () => {
    render(<SettingsModal onClose={() => {}} />);
    expect(await screen.findByText("tmux")).toBeInTheDocument();
    expect(screen.getByText("worktree-studio skill (global)")).toBeInTheDocument();
    expect(getDependencyStatus).toHaveBeenCalled();
  });

  it("shows every hook GET /api/settings/hooks returns, dynamically", async () => {
    render(<SettingsModal onClose={() => {}} />);
    expect(await screen.findByText("Claude session-tracking hook")).toBeInTheDocument();
    expect(screen.getByText("Claude worktree-context hook")).toBeInTheDocument();
    expect(getHooks).toHaveBeenCalled();
  });

  it("installs a hook from its own row and refreshes status, independent of other hooks", async () => {
    const user = userEvent.setup();
    render(<SettingsModal onClose={() => {}} />);
    const hookLabel = await screen.findByText("Claude session-tracking hook");
    const hookRow = hookLabel.closest("li")!;
    const otherRow = screen.getByText("Claude worktree-context hook").closest("li")!;

    vi.mocked(getHooks).mockResolvedValue([
      { ...hooksAllMissing[0], installed: true },
      hooksAllMissing[1],
    ]);

    await user.click(within(hookRow).getByRole("button", { name: "Install" }));

    expect(installHook).toHaveBeenCalledTimes(1);
    expect(installHook).toHaveBeenCalledWith("session-tracking");
    expect(await within(hookRow).findByRole("button", { name: "Uninstall" })).toBeInTheDocument();
    expect(within(otherRow).getByRole("button", { name: "Install" })).toBeInTheDocument();
  });

  it("switches to Appearance and picking a theme applies both attributes", async () => {
    const user = userEvent.setup();
    render(<SettingsModal onClose={() => {}} />);
    await user.click(screen.getByRole("tab", { name: "Appearance" }));

    const graphite = await screen.findByRole("radio", { name: /Graphite/ });
    expect(graphite).toHaveAttribute("aria-checked", "true");

    await user.click(screen.getByRole("radio", { name: /Ledger/ }));

    expect(document.documentElement.getAttribute("data-theme")).toBe("ledger");
    expect(localStorage.getItem("worktree-studio-theme-family")).toBe("ledger");
    expect(screen.getByRole("radio", { name: /Ledger/ })).toHaveAttribute("aria-checked", "true");
    expect(graphite).toHaveAttribute("aria-checked", "false");
  });

  it("the mode axis is independent of the theme family", async () => {
    const user = userEvent.setup();
    render(<SettingsModal onClose={() => {}} />);
    await user.click(screen.getByRole("tab", { name: "Appearance" }));

    await user.click(await screen.findByRole("radio", { name: "Light" }));

    expect(document.documentElement.getAttribute("data-mode")).toBe("light");
    expect(localStorage.getItem("worktree-studio-theme-mode")).toBe("light");
    // Family untouched by a mode change.
    expect(document.documentElement.getAttribute("data-theme")).toBe("graphite");

    await user.click(screen.getByRole("radio", { name: /Command Deck/ }));
    expect(document.documentElement.getAttribute("data-theme")).toBe("deck");
    expect(document.documentElement.getAttribute("data-mode")).toBe("light");
  });

  // "system" is a stored choice, but never a value CSS has to understand:
  // what reaches the DOM is always a concrete dark/light (see theme.ts).
  it("resolves System to a concrete mode on the element", async () => {
    const user = userEvent.setup();
    render(<SettingsModal onClose={() => {}} />);
    await user.click(screen.getByRole("tab", { name: "Appearance" }));

    await user.click(await screen.findByRole("radio", { name: "System" }));

    expect(localStorage.getItem("worktree-studio-theme-mode")).toBe("system");
    expect(document.documentElement.getAttribute("data-mode")).toMatch(/^(dark|light)$/);
  });

  it("switches to Logs and shows the log file path plus recent errors", async () => {
    const user = userEvent.setup();
    render(<SettingsModal onClose={() => {}} />);
    await user.click(screen.getByRole("tab", { name: "Logs" }));

    expect(await screen.findByText("/home/user/.worktree-studio/server.log")).toBeInTheDocument();
    expect(screen.getByText(/list repos.*err="boom"/)).toBeInTheDocument();
    expect(getServerLogs).toHaveBeenCalled();
  });

  it("Logs tab's Refresh button re-fetches", async () => {
    const user = userEvent.setup();
    render(<SettingsModal onClose={() => {}} />);
    await user.click(screen.getByRole("tab", { name: "Logs" }));
    await screen.findByText("/home/user/.worktree-studio/server.log");

    await user.click(screen.getByRole("button", { name: "Refresh" }));

    expect(getServerLogs).toHaveBeenCalledTimes(2);
  });

  it("calls onClose when the close button is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<SettingsModal onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: /close settings/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
