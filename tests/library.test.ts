import { describe, expect, test } from 'bun:test';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { LibraryError, loadLibrary, parseLibrary, saveLibrary, serializeLibrary } from '../src/library';

// Spec §1.1: name-keyed commands under {"version": 1}; fail loud on any
// parse/validation error; unknown fields tolerated and preserved; saves are
// 2-space pretty with key order preserved.

const VALID = JSON.stringify({
  version: 1,
  commands: {
    'deploy prod': { command: "ssh {{host=prod-1}} 'deploy.sh'", description: 'Roll out to production' },
  },
});

describe('parseLibrary', () => {
  test('parses a valid library', () => {
    const lib = parseLibrary(VALID, 'commands.json');
    expect(lib.commands['deploy prod']!.command).toBe("ssh {{host=prod-1}} 'deploy.sh'");
    expect(lib.commands['deploy prod']!.description).toBe('Roll out to production');
  });

  test.each([
    ['bad JSON', '{ nope'],
    ['missing version', '{"commands": {}}'],
    ['future version', '{"version": 2, "commands": {}}'],
    ['commands not an object', '{"version": 1, "commands": []}'],
    ['entry missing command', '{"version": 1, "commands": {"x": {}}}'],
    ['entry with empty command', '{"version": 1, "commands": {"x": {"command": ""}}}'],
    ['name empty after trimming', '{"version": 1, "commands": {"  ": {"command": "ls"}}}'],
    ['non-string description', '{"version": 1, "commands": {"x": {"command": "ls", "description": 3}}}'],
  ])('fails loud on %s, naming the source file', (_label, text) => {
    expect(() => parseLibrary(text, 'commands.json')).toThrow(LibraryError);
    expect(() => parseLibrary(text, 'commands.json')).toThrow(/commands\.json/);
  });
});

describe('serializeLibrary', () => {
  test('round-trips unknown fields on entries and at the top level', () => {
    const text = JSON.stringify({
      version: 1,
      color: 'blue',
      commands: { x: { command: 'ls', note: 'keep me' } },
    });
    const out = JSON.parse(serializeLibrary(parseLibrary(text, 'f')));
    expect(out.color).toBe('blue');
    expect(out.commands.x.note).toBe('keep me');
  });

  test('pretty-prints with 2-space indent and preserves key order', () => {
    const text = JSON.stringify({ version: 1, commands: { zeta: { command: 'z' }, alpha: { command: 'a' } } });
    const out = serializeLibrary(parseLibrary(text, 'f'));
    expect(out).toContain('  "commands"');
    expect(out.indexOf('zeta')).toBeLessThan(out.indexOf('alpha'));
    expect(out.endsWith('\n')).toBe(true);
  });
});

describe('loadLibrary / saveLibrary', () => {
  test('a missing file loads as an empty library', () => {
    const dir = mkdtempSync(join(tmpdir(), 'potato-'));
    const lib = loadLibrary(join(dir, 'commands.json'));
    expect(lib.commands).toEqual({});
  });

  test('save/load round-trip keeps entries, appending new names last', () => {
    const dir = mkdtempSync(join(tmpdir(), 'potato-'));
    const file = join(dir, 'commands.json');
    const lib = loadLibrary(file);
    lib.commands['first'] = { command: 'ls' };
    lib.commands['second'] = { command: 'pwd' };
    saveLibrary(file, lib);
    expect(Object.keys(loadLibrary(file).commands)).toEqual(['first', 'second']);
  });
});
