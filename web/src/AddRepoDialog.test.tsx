import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import AddRepoDialog from "./AddRepoDialog";

// Regression coverage for "the modal backdrop shows but no dialog content
// is visible": this can't catch a pure-CSS invisibility bug (jsdom
// doesn't compute layout/paint), but it does prove the dialog's actual
// content unconditionally renders into the DOM the moment the component
// mounts — ruling out the "conditional render never fires" class of bug
// so a CSS explanation is the remaining one worth trusting.
describe("AddRepoDialog", () => {
  it("renders its form content (not just a backdrop) as soon as it mounts", () => {
    render(<AddRepoDialog onCreated={vi.fn()} onClose={vi.fn()} />);
    expect(screen.getByRole("heading", { name: "Add repo" })).toBeInTheDocument();
    expect(screen.getByLabelText("Local path")).toBeInTheDocument();
    expect(screen.getByLabelText(/display name/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add repo" })).toBeInTheDocument();
  });

  it("calls onClose when the backdrop (not the dialog box) is clicked", async () => {
    const onClose = vi.fn();
    render(<AddRepoDialog onCreated={vi.fn()} onClose={onClose} />);
    const backdrop = screen.getByRole("heading", { name: "Add repo" }).closest(".dialog-backdrop");
    expect(backdrop).not.toBeNull();
    (backdrop as HTMLElement).click();
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not call onClose when clicking inside the dialog box itself", async () => {
    const onClose = vi.fn();
    render(<AddRepoDialog onCreated={vi.fn()} onClose={onClose} />);
    screen.getByRole("heading", { name: "Add repo" }).click();
    expect(onClose).not.toHaveBeenCalled();
  });
});
