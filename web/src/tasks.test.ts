import { describe, expect, it } from "vitest";
import { sortTasks, taskKeyLabel, taskWorktreeName } from "./tasks";

const task = (key: string, status_id: number, updated_at: string) => ({
  source: "local",
  key,
  title: "",
  url: "",
  status_id,
  updated_at,
});

describe("tasks helpers", () => {
  it("sorts by status position, then most recently updated", () => {
    const statuses = [
      { id: 7, name: "Done", position: 2 },
      { id: 5, name: "In progress", position: 0 },
    ];
    const sorted = sortTasks(
      [task("1", 7, "2026-01-03"), task("2", 5, "2026-01-01"), task("3", 5, "2026-01-02")],
      statuses
    );
    expect(sorted.map((t) => t.key)).toEqual(["3", "2", "1"]);
  });

  it("labels local keys with # and leaves external keys alone", () => {
    expect(taskKeyLabel({ source: "local", key: "12" })).toBe("#12");
    expect(taskKeyLabel({ source: "jira", key: "PROJ-1" })).toBe("PROJ-1");
  });

  it("derives a worktree name from key and title", () => {
    expect(taskWorktreeName({ key: "PROJ-123", title: "Split invoice export!" })).toBe(
      "proj-123-split-invoice-export"
    );
    expect(taskWorktreeName({ key: "", title: "x".repeat(60) })).toHaveLength(48);
  });
});
