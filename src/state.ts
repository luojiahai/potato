// State: ~/.potato/state.json (spec §1.2) — a disposable per-Command cache of
// last-used time and last Placeholder values. Unreadable state resets to {}.

import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';

export interface CommandState {
  lastUsedAt: string;
  args?: Record<string, string>;
}

export type State = Record<string, CommandState>;

export function loadState(path: string): State {
  if (!existsSync(path)) return {};
  try {
    const raw = JSON.parse(readFileSync(path, 'utf8'));
    if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) return {};
    return raw as State;
  } catch {
    return {};
  }
}

export function saveState(path: string, state: State): void {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, JSON.stringify(state, null, 2) + '\n');
}

export function recordUse(state: State, name: string, args: Record<string, string>, now: Date): State {
  return {
    ...state,
    [name]: {
      lastUsedAt: now.toISOString(),
      args: { ...state[name]?.args, ...args },
    },
  };
}
