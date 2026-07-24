// Import: merge another Library into ours (spec §7). Ours wins by default;
// --theirs flips Collisions to overwrite. All-or-nothing: callers validate
// both files with the fail-loud loader before merging.

import type { CommandEntry, Library } from './library';

export interface MergeResult {
  merged: Library;
  added: string[];
  skipped: string[];
  overwritten: string[];
}

const identical = (a: CommandEntry, b: CommandEntry) => JSON.stringify(a) === JSON.stringify(b);

export function mergeLibraries(ours: Library, theirs: Library, opts: { theirs?: boolean }): MergeResult {
  const merged: Library = { ...ours, commands: { ...ours.commands } };
  const added: string[] = [];
  const skipped: string[] = [];
  const overwritten: string[] = [];
  for (const [name, entry] of Object.entries(theirs.commands)) {
    const existing = merged.commands[name];
    if (existing === undefined) {
      merged.commands[name] = entry;
      added.push(name);
    } else if (!identical(existing, entry)) {
      if (opts.theirs) {
        merged.commands[name] = entry;
        overwritten.push(name);
      } else {
        skipped.push(name);
      }
    }
  }
  return { merged, added, skipped, overwritten };
}
