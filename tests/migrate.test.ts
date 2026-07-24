import { describe, expect, test } from 'bun:test';
import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { loadLibraryMigrating, migrateLibrary, parseLibraryV1, rekeyState } from '../src/migrate';
import { LibraryError } from '../src/library';

// v1 → v2 migration: fresh UUIDs, a name→id map that also rekeys state, and an
// atomic auto-on-load upgrade that leaves the v1 file intact on failure.

const dir = () => mkdtempSync(join(tmpdir(), 'potato-mig-'));

describe('migrateLibrary', () => {
  test('turns name-keyed v1 commands into a v2 array with fresh ids, preserving order', () => {
    const v1 = {
      version: 1 as const,
      commands: {
        deploy: { command: 'ssh prod', description: 'roll out' },
        list: { command: 'ls' },
      },
    };
    const { library, nameToId } = migrateLibrary(v1);
    expect(library.version).toBe(2);
    expect(library.commands.map((c) => c.name)).toEqual(['deploy', 'list']);
    expect(library.commands[0]!.command).toBe('ssh prod');
    expect(library.commands[0]!.description).toBe('roll out');
    expect(library.commands[1]!.description).toBeUndefined();
    // ids minted and threaded into the map
    expect(nameToId.get('deploy')).toBe(library.commands[0]!.id);
    expect(library.commands[0]!.id).not.toBe(library.commands[1]!.id);
  });

  test('preserves unknown fields at top level and per entry', () => {
    const v1 = { version: 1 as const, color: 'blue', commands: { x: { command: 'ls', note: 'keep' } } };
    const { library } = migrateLibrary(v1);
    expect((library as { color?: string }).color).toBe('blue');
    expect((library.commands[0] as { note?: string }).note).toBe('keep');
  });
});

describe('rekeyState', () => {
  test('moves entries from name keys to id keys and drops orphans', () => {
    const map = new Map([['deploy', 'id-1']]);
    const state = {
      deploy: { lastUsedAt: '2026-07-01T00:00:00.000Z' },
      gone: { lastUsedAt: '2026-01-01T00:00:00.000Z' },
    };
    expect(rekeyState(state, map)).toEqual({ 'id-1': { lastUsedAt: '2026-07-01T00:00:00.000Z' } });
  });
});

describe('parseLibraryV1', () => {
  test('fails loud on a malformed v1 file, naming the source', () => {
    expect(() => parseLibraryV1('{ nope', 'commands.json')).toThrow(LibraryError);
    expect(() => parseLibraryV1('{"version":1,"commands":{"x":{}}}', 'commands.json')).toThrow(/commands\.json/);
  });
});

describe('loadLibraryMigrating', () => {
  test('a missing library is an empty v2 library, not migrated', () => {
    const home = dir();
    const result = loadLibraryMigrating(join(home, 'commands.json'), join(home, 'state.json'));
    expect(result).toEqual({ library: { version: 2, commands: [] }, state: {}, migrated: false });
  });

  test('a v2 library loads unchanged, not migrated', () => {
    const home = dir();
    const cmds = join(home, 'commands.json');
    writeFileSync(cmds, JSON.stringify({ version: 2, commands: [{ id: 'a', name: 'x', command: 'ls' }] }));
    const result = loadLibraryMigrating(cmds, join(home, 'state.json'));
    expect(result.migrated).toBe(false);
    expect(result.library.commands[0]!.name).toBe('x');
  });

  test('a v1 library migrates, writes v2 back, and rekeys state', () => {
    const home = dir();
    const cmds = join(home, 'commands.json');
    const st = join(home, 'state.json');
    writeFileSync(cmds, JSON.stringify({ version: 1, commands: { deploy: { command: 'ssh prod' } } }));
    writeFileSync(st, JSON.stringify({ deploy: { lastUsedAt: '2026-07-01T00:00:00.000Z' } }));

    const result = loadLibraryMigrating(cmds, st);
    expect(result.migrated).toBe(true);
    const id = result.library.commands[0]!.id;
    // file written back as v2
    expect(JSON.parse(readFileSync(cmds, 'utf8')).version).toBe(2);
    // state rekeyed to the new id, on disk and in the result
    expect(result.state[id]!.lastUsedAt).toBe('2026-07-01T00:00:00.000Z');
    expect(JSON.parse(readFileSync(st, 'utf8'))[id].lastUsedAt).toBe('2026-07-01T00:00:00.000Z');
    // a second load sees v2 and does not migrate again (idempotent)
    expect(loadLibraryMigrating(cmds, st).migrated).toBe(false);
  });

  test('a malformed v1 library fails loud, leaving the file untouched', () => {
    const home = dir();
    const cmds = join(home, 'commands.json');
    const original = JSON.stringify({ version: 1, commands: { x: { command: '' } } });
    writeFileSync(cmds, original);
    expect(() => loadLibraryMigrating(cmds, join(home, 'state.json'))).toThrow(LibraryError);
    expect(readFileSync(cmds, 'utf8')).toBe(original);
  });
});
