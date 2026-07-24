import { describe, expect, test } from 'bun:test';
import { mergeLibraries } from '../src/import';
import type { Library } from '../src/library';

// Spec §7: merge, ours wins. New names appended in incoming-file order;
// differing Collisions skipped and reported; identical entries are silent
// no-ops; --theirs flips collisions to overwrite. State is untouched (the
// merge never sees it).

const ours = (): Library => ({
  version: 1,
  commands: {
    alpha: { command: 'echo ours-alpha' },
    beta: { command: 'echo beta' },
  },
});

describe('mergeLibraries (ours wins)', () => {
  test('adds unknown incoming names, appended in incoming order', () => {
    const theirs: Library = {
      version: 1,
      commands: { zeta: { command: 'echo z' }, gamma: { command: 'echo g' } },
    };
    const result = mergeLibraries(ours(), theirs, {});
    expect(Object.keys(result.merged.commands)).toEqual(['alpha', 'beta', 'zeta', 'gamma']);
    expect(result.added).toEqual(['zeta', 'gamma']);
    expect(result.skipped).toEqual([]);
  });

  test('skips and reports differing collisions, keeping ours', () => {
    const theirs: Library = { version: 1, commands: { alpha: { command: 'echo theirs-alpha' } } };
    const result = mergeLibraries(ours(), theirs, {});
    expect(result.merged.commands['alpha']!.command).toBe('echo ours-alpha');
    expect(result.skipped).toEqual(['alpha']);
    expect(result.added).toEqual([]);
  });

  test('identical collisions are silent no-ops', () => {
    const theirs: Library = { version: 1, commands: { beta: { command: 'echo beta' } } };
    const result = mergeLibraries(ours(), theirs, {});
    expect(result.added).toEqual([]);
    expect(result.skipped).toEqual([]);
  });

  test('--theirs overwrites collisions instead of skipping', () => {
    const theirs: Library = { version: 1, commands: { alpha: { command: 'echo theirs-alpha' } } };
    const result = mergeLibraries(ours(), theirs, { theirs: true });
    expect(result.merged.commands['alpha']!.command).toBe('echo theirs-alpha');
    expect(result.overwritten).toEqual(['alpha']);
    expect(result.skipped).toEqual([]);
  });

  test('an entry differing only in unknown extra fields is a differing collision', () => {
    const theirs: Library = { version: 1, commands: { beta: { command: 'echo beta', note: 'hi' } } };
    const result = mergeLibraries(ours(), theirs, {});
    expect(result.skipped).toEqual(['beta']);
  });
});
