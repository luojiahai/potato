// Import: a simple, local merge — not a sync tool. It never reasons about the
// incoming file's ids (identity stays internal): every incoming Command is
// added as a brand-new Command with a fresh id, and on a name Collision the
// incoming copy is renamed so both are kept. `--override` (handled in cli.tsx)
// is the other mode: replace the whole Library with the imported file as-is.

import { randomUUID } from 'node:crypto';
import type { Library } from './library';

export interface MergeResult {
  merged: Library;
  added: string[]; // incoming names taken as-is (no collision)
  renamed: Array<[string, string]>; // [incoming name, renamed to] for collisions
}

// Add every incoming Command with a fresh id, appending in incoming order. On a
// name Collision (case-sensitive, against the running merged set) rename to
// `name (N)` — lowest free N from 1 — keeping both. Deliberately not
// idempotent: re-merging the same file duplicates everything.
export function mergeLibraries(ours: Library, theirs: Library): MergeResult {
  const commands = [...ours.commands];
  const taken = new Set(commands.map((c) => c.name));
  const added: string[] = [];
  const renamed: Array<[string, string]> = [];

  for (const entry of theirs.commands) {
    let name = entry.name;
    if (taken.has(name)) {
      let n = 1;
      while (taken.has(`${entry.name} (${n})`)) n++;
      name = `${entry.name} (${n})`;
      renamed.push([entry.name, name]);
    } else {
      added.push(name);
    }
    taken.add(name);
    commands.push({ ...entry, id: randomUUID(), name });
  }

  return { merged: { ...ours, commands }, added, renamed };
}
