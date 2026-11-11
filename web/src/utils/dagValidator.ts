import { load as yamlLoad } from 'js-yaml';
import { WorkflowSpec } from '../types';

export interface ValidationResult {
  valid: boolean;
  error?: string;
  taskCount?: number;
  workflow?: WorkflowSpec;
}

export function validateWorkflowText(text: string): ValidationResult {
  const trimmed = text.trim();
  if (!trimmed) {
    return { valid: false, error: 'Workflow definition is empty' };
  }

  let parsed: any;
  try {
    if (trimmed.startsWith('{')) {
      parsed = JSON.parse(trimmed);
    } else {
      parsed = yamlLoad(trimmed);
    }
  } catch (err: any) {
    return { valid: false, error: `Syntax error: ${err.message}` };
  }

  if (!parsed || typeof parsed !== 'object') {
    return { valid: false, error: 'Root object must be a valid mapping' };
  }

  const id = parsed.id;
  if (!id || typeof id !== 'string' || id.trim() === '') {
    return { valid: false, error: 'Workflow requires a valid "id"' };
  }

  const tasksRaw = parsed.tasks;
  if (!Array.isArray(tasksRaw) || tasksRaw.length === 0) {
    return { valid: false, error: 'Workflow must declare at least one task in "tasks"' };
  }

  // Check duplicate IDs, self-dependencies, unknown dependencies
  const taskMap = new Map<string, any>();
  for (const t of tasksRaw) {
    if (!t || !t.id || typeof t.id !== 'string') {
      return { valid: false, error: 'All tasks must have a non-empty string "id"' };
    }
    const tid = t.id.trim();
    if (taskMap.has(tid)) {
      return { valid: false, error: `Duplicate task ID: "${tid}"` };
    }
    taskMap.set(tid, t);
  }

  // Validate dependencies
  const adj = new Map<string, string[]>();
  for (const [tid, t] of taskMap.entries()) {
    const deps: string[] = Array.isArray(t.dependencies) ? t.dependencies : [];
    for (const dep of deps) {
      if (typeof dep !== 'string') continue;
      const depTrimmed = dep.trim();
      if (depTrimmed === tid) {
        return { valid: false, error: `Task "${tid}" depends on itself` };
      }
      if (!taskMap.has(depTrimmed)) {
        return { valid: false, error: `Task "${tid}" depends on unknown task "${depTrimmed}"` };
      }
    }
    adj.set(tid, deps);
  }

  // Cycle detection via DFS (visiting / visited states)
  const state = new Map<string, number>(); // 0: unvisited, 1: visiting, 2: visited
  const parent = new Map<string, string>();
  let cyclePath: string[] | null = null;

  function dfs(curr: string): boolean {
    state.set(curr, 1);
    const deps = adj.get(curr) || [];
    for (const dep of deps) {
      const s = state.get(dep) || 0;
      if (s === 1) {
        // Cycle found
        cyclePath = [dep, curr];
        let p = parent.get(curr);
        while (p && p !== dep) {
          cyclePath.push(p);
          p = parent.get(p);
        }
        cyclePath.push(dep);
        cyclePath.reverse();
        return true;
      }
      if (s === 0) {
        parent.set(dep, curr);
        if (dfs(dep)) return true;
      }
    }
    state.set(curr, 2);
    return false;
  }

  for (const tid of taskMap.keys()) {
    if ((state.get(tid) || 0) === 0) {
      if (dfs(tid)) {
        const pathStr = cyclePath ? (cyclePath as string[]).join(' -> ') : 'circular dependency';
        return {
          valid: false,
          error: `Cycle detected: ${pathStr}`,
        };
      }
    }
  }

  return {
    valid: true,
    taskCount: tasksRaw.length,
    workflow: parsed as WorkflowSpec,
  };
}
