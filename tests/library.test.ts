import { describe, expect, test } from 'bun:test';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { LibraryError, loadLibrary, parseLibrary, saveLibrary, serializeLibrary } from '../src/library';

// Library v2: an array of UUID-identified entries { id, name, description?,
// command }; name is unique and case-sensitive, id is loosely validated
// (non-empty + unique). Version-strict — v1 and unknown versions fail loud.
// Unknown fields tolerated and preserved; saves are 2-space pretty, array
// order kept.

const VALID = JSON.stringify({
  version: 2,
  commands: [
    {
      id: 'b3f1c2a4-5d6e-4f80-9a1b-2c3d4e5f6a7b',
      name: 'deploy prod',
      description: 'Roll out to production',
      command: "ssh {{host=prod-1}} 'deploy.sh'",
    },
  ],
});

describe('parseLibrary', () => {
  test('parses a valid library', () => {
    const lib = parseLibrary(VALID, 'commands.json');
    expect(lib.commands[0]!.name).toBe('deploy prod');
    expect(lib.commands[0]!.command).toBe("ssh {{host=prod-1}} 'deploy.sh'");
    expect(lib.commands[0]!.description).toBe('Roll out to production');
  });

  test('a non-UUID id is allowed (loose validation)', () => {
    const text = JSON.stringify({ version: 2, commands: [{ id: 'not-a-uuid', name: 'x', command: 'ls' }] });
    expect(parseLibrary(text, 'f').commands[0]!.id).toBe('not-a-uuid');
  });

  test.each([
    ['bad JSON', '{ nope'],
    ['missing version', '{"commands": []}'],
    ['v1 is rejected version-strict', '{"version": 1, "commands": {}}'],
    ['future version', '{"version": 3, "commands": []}'],
    ['commands not an array', '{"version": 2, "commands": {}}'],
    ['entry missing id', '{"version": 2, "commands": [{"name": "x", "command": "ls"}]}'],
    ['entry with empty id', '{"version": 2, "commands": [{"id": "", "name": "x", "command": "ls"}]}'],
    ['duplicate id', '{"version": 2, "commands": [{"id":"a","name":"x","command":"ls"},{"id":"a","name":"y","command":"pwd"}]}'],
    ['duplicate name', '{"version": 2, "commands": [{"id":"a","name":"x","command":"ls"},{"id":"b","name":"x","command":"pwd"}]}'],
    ['name empty after trimming', '{"version": 2, "commands": [{"id":"a","name":"  ","command":"ls"}]}'],
    ['entry missing command', '{"version": 2, "commands": [{"id":"a","name":"x"}]}'],
    ['entry with empty command', '{"version": 2, "commands": [{"id":"a","name":"x","command":""}]}'],
    ['non-string description', '{"version": 2, "commands": [{"id":"a","name":"x","command":"ls","description":3}]}'],
  ])('fails loud on %s, naming the source file', (_label, text) => {
    expect(() => parseLibrary(text, 'commands.json')).toThrow(LibraryError);
    expect(() => parseLibrary(text, 'commands.json')).toThrow(/commands\.json/);
  });

  test('names are case-sensitive — deploy and Deploy coexist', () => {
    const text = JSON.stringify({
      version: 2,
      commands: [
        { id: 'a', name: 'deploy', command: 'ls' },
        { id: 'b', name: 'Deploy', command: 'pwd' },
      ],
    });
    expect(parseLibrary(text, 'f').commands).toHaveLength(2);
  });
});

describe('serializeLibrary', () => {
  test('round-trips unknown fields on entries and at the top level', () => {
    const text = JSON.stringify({
      version: 2,
      color: 'blue',
      commands: [{ id: 'a', name: 'x', command: 'ls', note: 'keep me' }],
    });
    const out = JSON.parse(serializeLibrary(parseLibrary(text, 'f')));
    expect(out.color).toBe('blue');
    expect(out.commands[0].note).toBe('keep me');
  });

  test('pretty-prints with 2-space indent and preserves array order', () => {
    const text = JSON.stringify({
      version: 2,
      commands: [
        { id: 'a', name: 'zeta', command: 'z' },
        { id: 'b', name: 'alpha', command: 'a' },
      ],
    });
    const out = serializeLibrary(parseLibrary(text, 'f'));
    expect(out).toContain('  "commands"');
    expect(out.indexOf('zeta')).toBeLessThan(out.indexOf('alpha'));
    expect(out.endsWith('\n')).toBe(true);
  });
});

describe('loadLibrary / saveLibrary', () => {
  test('a missing file loads as an empty v2 library', () => {
    const dir = mkdtempSync(join(tmpdir(), 'potato-'));
    const lib = loadLibrary(join(dir, 'commands.json'));
    expect(lib).toEqual({ version: 2, commands: [] });
  });

  test('save/load round-trip keeps entries, appending new commands last', () => {
    const dir = mkdtempSync(join(tmpdir(), 'potato-'));
    const file = join(dir, 'commands.json');
    const lib = loadLibrary(file);
    lib.commands.push({ id: 'a', name: 'first', command: 'ls' });
    lib.commands.push({ id: 'b', name: 'second', command: 'pwd' });
    saveLibrary(file, lib);
    expect(loadLibrary(file).commands.map((c) => c.name)).toEqual(['first', 'second']);
  });
});
