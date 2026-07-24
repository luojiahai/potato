import { describe, expect, test } from 'bun:test';
import { mergeLibraries } from '../src/import';
import type { Library } from '../src/library';

// Import v2: a simple local merge. Every incoming Command is added as a new
// Command with a fresh id, appended in incoming order; on a name Collision the
// incoming copy is renamed `name (N)` (case-sensitive) so both are kept.
// Nothing is skipped or overwritten. Ids never match across files.

const ours = (): Library => ({
  version: 2,
  commands: [
    { id: 'o1', name: 'alpha', command: 'echo ours-alpha' },
    { id: 'o2', name: 'beta', command: 'echo beta' },
  ],
});

const isUuidish = (s: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-/i.test(s);

describe('mergeLibraries (keep both)', () => {
  test('adds unknown incoming names with fresh ids, appended in incoming order', () => {
    const theirs: Library = {
      version: 2,
      commands: [
        { id: 't1', name: 'zeta', command: 'echo z' },
        { id: 't2', name: 'gamma', command: 'echo g' },
      ],
    };
    const result = mergeLibraries(ours(), theirs);
    expect(result.merged.commands.map((c) => c.name)).toEqual(['alpha', 'beta', 'zeta', 'gamma']);
    expect(result.added).toEqual(['zeta', 'gamma']);
    expect(result.renamed).toEqual([]);
    // fresh ids — the incoming file's ids are ignored
    const zeta = result.merged.commands.find((c) => c.name === 'zeta')!;
    expect(zeta.id).not.toBe('t1');
    expect(isUuidish(zeta.id)).toBe(true);
  });

  test('renames on a name Collision, keeping both', () => {
    const theirs: Library = { version: 2, commands: [{ id: 't1', name: 'alpha', command: 'echo theirs-alpha' }] };
    const result = mergeLibraries(ours(), theirs);
    expect(result.renamed).toEqual([['alpha', 'alpha (1)']]);
    expect(result.added).toEqual([]);
    const names = result.merged.commands.map((c) => c.name);
    expect(names).toContain('alpha');
    expect(names).toContain('alpha (1)');
    // ours is untouched; theirs came in renamed
    expect(result.merged.commands.find((c) => c.name === 'alpha')!.command).toBe('echo ours-alpha');
    expect(result.merged.commands.find((c) => c.name === 'alpha (1)')!.command).toBe('echo theirs-alpha');
  });

  test('picks the lowest free suffix against the running merged set', () => {
    const base: Library = {
      version: 2,
      commands: [
        { id: 'o1', name: 'ship', command: 'a' },
        { id: 'o2', name: 'ship (1)', command: 'b' },
      ],
    };
    const theirs: Library = { version: 2, commands: [{ id: 't1', name: 'ship', command: 'c' }] };
    const result = mergeLibraries(base, theirs);
    expect(result.renamed).toEqual([['ship', 'ship (2)']]);
  });

  test('a name already ending in a suffix simple-appends', () => {
    const base: Library = { version: 2, commands: [{ id: 'o1', name: 'deploy (1)', command: 'a' }] };
    const theirs: Library = { version: 2, commands: [{ id: 't1', name: 'deploy (1)', command: 'b' }] };
    const result = mergeLibraries(base, theirs);
    expect(result.renamed).toEqual([['deploy (1)', 'deploy (1) (1)']]);
  });

  test('two incoming copies of the same colliding name both get kept', () => {
    const theirs: Library = {
      version: 2,
      commands: [
        { id: 't1', name: 'alpha', command: 'one' },
        { id: 't2', name: 'alpha', command: 'two' },
      ],
    };
    const result = mergeLibraries(ours(), theirs);
    expect(result.merged.commands.map((c) => c.name)).toEqual(['alpha', 'beta', 'alpha (1)', 'alpha (2)']);
  });

  test('is not idempotent — re-merging duplicates everything', () => {
    const theirs: Library = { version: 2, commands: [{ id: 't1', name: 'zeta', command: 'echo z' }] };
    const once = mergeLibraries(ours(), theirs);
    const twice = mergeLibraries(once.merged, theirs);
    expect(twice.merged.commands.map((c) => c.name)).toEqual(['alpha', 'beta', 'zeta', 'zeta (1)']);
  });

  test('case-sensitive collisions — Alpha does not collide with alpha', () => {
    const theirs: Library = { version: 2, commands: [{ id: 't1', name: 'Alpha', command: 'echo cap' }] };
    const result = mergeLibraries(ours(), theirs);
    expect(result.added).toEqual(['Alpha']);
    expect(result.renamed).toEqual([]);
  });
});
