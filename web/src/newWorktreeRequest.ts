import { createContext, useContext } from "react";
import { PickedTask } from "./TaskPicker";

// Opens Layout's single NewWorktreeDialog from anywhere below it (sidebar,
// task popup, board), optionally with a task already picked. A context of
// its own rather than a prop chain, and a file of its own so those
// components don't import Layout.
export type OpenNewWorktree = (repoId: string, task?: PickedTask) => void;

export const NewWorktreeContext = createContext<OpenNewWorktree>(() => {});

export function useOpenNewWorktree(): OpenNewWorktree {
  return useContext(NewWorktreeContext);
}
