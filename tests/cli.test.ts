import { describe, expect, test } from 'bun:test';
import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// CLI seams: `potato import` (merge / override / migrate-on-load) run as a real
// subprocess against a POTATO_INSTALL temp dir, plus the pure helpers behind
// update/uninstall.

const CLI = join(import.meta.dir, '..', 'src', 'cli.tsx');

function run(args: string[], opts: { home?: string; stdin?: string } = {}) {
  const home = opts.home ?? mkdtempSync(join(tmpdir(), 'potato-home-'));
  const proc = Bun.spawnSync(['bun', CLI, ...args], {
    env: { ...process.env, POTATO_INSTALL: home },
    stdin: opts.stdin === undefined ? 'ignore' : new TextEncoder().encode(opts.stdin),
  });
  return {
    home,
    exitCode: proc.exitCode,
    stdout: proc.stdout.toString(),
    stderr: proc.stderr.toString(),
  };
}

type Entry = { id: string; name: string; command: string; description?: string };
const v2 = (commands: Entry[]) => JSON.stringify({ version: 2, commands });
const readLib = (home: string) => JSON.parse(readFileSync(join(home, 'commands.json'), 'utf8'));

describe('potato import (--merge, the default)', () => {
  test('adds new names and reports them; writes a v2 library; exit 0', () => {
    const dir = mkdtempSync(join(tmpdir(), 'potato-in-'));
    const incoming = join(dir, 'theirs.json');
    writeFileSync(incoming, v2([{ id: 't1', name: 'hello', command: 'echo hi' }]));
    const result = run(['import', incoming]);
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain('added: hello');
    const saved = readLib(result.home);
    expect(saved.version).toBe(2);
    const hello = saved.commands.find((c: Entry) => c.name === 'hello');
    expect(hello.command).toBe('echo hi');
    // a fresh id was minted (incoming id ignored)
    expect(hello.id).not.toBe('t1');
  });

  test('a name collision keeps both, renaming the incoming copy', () => {
    const home = mkdtempSync(join(tmpdir(), 'potato-home-'));
    writeFileSync(join(home, 'commands.json'), v2([{ id: 'o1', name: 'x', command: 'ours' }]));
    const dir = mkdtempSync(join(tmpdir(), 'potato-in-'));
    const incoming = join(dir, 'theirs.json');
    writeFileSync(incoming, v2([{ id: 't1', name: 'x', command: 'theirs' }]));
    const result = run(['import', incoming], { home });
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain('kept both: x → x (1)');
    const saved = readLib(home);
    expect(saved.commands.find((c: Entry) => c.name === 'x').command).toBe('ours');
    expect(saved.commands.find((c: Entry) => c.name === 'x (1)').command).toBe('theirs');
  });

  test('nothing to import on an empty incoming library', () => {
    const result = run(['import', '-'], { stdin: v2([]) });
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain('nothing to import');
  });

  test('- reads from stdin', () => {
    const result = run(['import', '-'], { stdin: v2([{ id: 't1', name: 'piped', command: 'echo pipe' }]) });
    expect(result.exitCode).toBe(0);
    expect(readLib(result.home).commands.find((c: Entry) => c.name === 'piped').command).toBe('echo pipe');
  });
});

describe('potato import --override', () => {
  test('replaces the whole library with the incoming file as-is', () => {
    const home = mkdtempSync(join(tmpdir(), 'potato-home-'));
    writeFileSync(join(home, 'commands.json'), v2([{ id: 'o1', name: 'mine', command: 'keep?' }]));
    const dir = mkdtempSync(join(tmpdir(), 'potato-in-'));
    const incoming = join(dir, 'theirs.json');
    writeFileSync(incoming, v2([{ id: 't1', name: 'theirs', command: 'echo t' }]));
    const result = run(['import', incoming, '--override'], { home });
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain('replaced your Library');
    const saved = readLib(home);
    expect(saved.commands.map((c: Entry) => c.name)).toEqual(['theirs']);
    // ids kept as-is under --override
    expect(saved.commands[0].id).toBe('t1');
  });
});

describe('potato import — failure and migration', () => {
  test('an invalid incoming file aborts all-or-nothing with file + reason', () => {
    const home = mkdtempSync(join(tmpdir(), 'potato-home-'));
    writeFileSync(join(home, 'commands.json'), v2([{ id: 'o1', name: 'keep', command: 'ls' }]));
    const dir = mkdtempSync(join(tmpdir(), 'potato-in-'));
    const incoming = join(dir, 'bad.json');
    writeFileSync(incoming, '{"version": 99, "commands": []}');
    const result = run(['import', incoming], { home });
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr).toContain('bad.json');
    // library untouched
    expect(readLib(home).commands.find((c: Entry) => c.name === 'keep').command).toBe('ls');
  });

  test('a still-v1 incoming file is rejected (import reads v2 only)', () => {
    const dir = mkdtempSync(join(tmpdir(), 'potato-in-'));
    const incoming = join(dir, 'old.json');
    writeFileSync(incoming, JSON.stringify({ version: 1, commands: { legacy: { command: 'ls' } } }));
    const result = run(['import', incoming]);
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr.toLowerCase()).toContain('unsupported version 1');
  });

  test('our own v1 library auto-migrates on load, rekeying state, with a notice', () => {
    const home = mkdtempSync(join(tmpdir(), 'potato-home-'));
    writeFileSync(
      join(home, 'commands.json'),
      JSON.stringify({ version: 1, commands: { legacy: { command: 'echo old' } } }),
    );
    writeFileSync(
      join(home, 'state.json'),
      JSON.stringify({ legacy: { lastUsedAt: '2026-07-01T00:00:00.000Z' }, ghost: { lastUsedAt: '2026-01-01T00:00:00.000Z' } }),
    );
    const dir = mkdtempSync(join(tmpdir(), 'potato-in-'));
    const incoming = join(dir, 'theirs.json');
    writeFileSync(incoming, v2([{ id: 't1', name: 'fresh', command: 'echo new' }]));

    const result = run(['import', incoming], { home });
    expect(result.exitCode).toBe(0);
    expect(result.stderr).toContain('upgraded your library to v2');

    const saved = readLib(home);
    expect(saved.version).toBe(2);
    const legacy = saved.commands.find((c: Entry) => c.name === 'legacy');
    expect(legacy.command).toBe('echo old');
    // state rekeyed name→id; orphan 'ghost' dropped
    const state = JSON.parse(readFileSync(join(home, 'state.json'), 'utf8'));
    expect(state.legacy).toBeUndefined();
    expect(state.ghost).toBeUndefined();
    expect(state[legacy.id].lastUsedAt).toBe('2026-07-01T00:00:00.000Z');
  });
});

describe('uninstall helper: removeInitLines', () => {
  test('removes exactly the sourced init line, keeping everything else', async () => {
    const { removeInitLines } = await import('../src/uninstall');
    const rc = ['# my rc', 'alias ll="ls -l"', 'source ~/.potato/init.zsh', 'export FOO=1', ''].join('\n');
    expect(removeInitLines(rc)).toBe(['# my rc', 'alias ll="ls -l"', 'export FOO=1', ''].join('\n'));
  });

  test('leaves an rc without the line unchanged', async () => {
    const { removeInitLines } = await import('../src/uninstall');
    const rc = '# nothing potato here\n';
    expect(removeInitLines(rc)).toBe(rc);
  });
});

describe('update helpers', () => {
  test('parseSha256Sums maps file names to hashes', async () => {
    const { parseSha256Sums } = await import('../src/update');
    const text = [
      'abc123  potato-darwin-arm64.tar.gz',
      'def456  potato-linux-x64.tar.gz',
      '',
    ].join('\n');
    expect(parseSha256Sums(text)).toEqual({
      'potato-darwin-arm64.tar.gz': 'abc123',
      'potato-linux-x64.tar.gz': 'def456',
    });
  });

  test('targetTriple maps platform/arch to release asset names', async () => {
    const { targetTriple } = await import('../src/update');
    expect(targetTriple('darwin', 'arm64')).toBe('darwin-arm64');
    expect(targetTriple('linux', 'x64')).toBe('linux-x64');
    expect(() => targetTriple('win32', 'x64')).toThrow();
  });
});
