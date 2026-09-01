// TaskTreeView renders the forest roots of the task tree (docs
// TASK_TREE_DESIGN.md §4.1). Shared by L1 popover and the L2 dock; TaskTreeNode
// recurses into children.

import type { TaskNode } from "../lib/taskCatalogTypes";
import { taskNodeKey } from "../lib/taskTree";
import { TaskTreeNode, type TaskTreeRowProps } from "./TaskTreeNode";

export function TaskTreeView({ roots, ...rowProps }: { roots: TaskNode[] } & TaskTreeRowProps) {
  if (roots.length === 0) return null;
  return (
    <div className="taskmonitor__tree">
      {roots.map((root) => (
        <TaskTreeNode key={taskNodeKey(root.projectKey, root.task.task_id)} node={root} depth={0} {...rowProps} />
      ))}
    </div>
  );
}
