import { describe, expect, test } from 'bun:test';
import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// CLI seams: `potato import` (spec §7) run as a real subprocess against a
// POTATO_INSTALL temp dir, plus the pure helpers behind update/uninstall.

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

const lib = (commands: Record<string, unknown>) => JSON.stringify({ version: 1, commands });

describe('potato import', () => {
  test('adds new names and reports them; exit 0', () => {
    const dir = mkdtempSync(join(tmpdir(), 'potato-in-'));
    const incoming = join(dir, 'theirs.json');
    writeFileSync(incoming, lib({ hello: { command: 'echo hi' } }));
    const result = run(['import', incoming]);
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain('hello');
    const saved = JSON.parse(readFileSync(join(result.home, 'commands.json'), 'utf8'));
    expect(saved.commands.hello.command).toBe('echo hi');
  });

  test('differing collisions are skipped and reported; ours kept; exit 0', () => {
    const home = mkdtempSync(join(tmpdir(), 'potato-home-'));
    writeFileSync(join(home, 'commands.json'), lib({ x: { command: 'ours' } }));
    const dir = mkdtempSync(join(tmpdir(), 'potato-in-'));
    const incoming = join(dir, 'theirs.json');
    writeFileSync(incoming, lib({ x: { command: 'theirs' } }));
    const result = run(['import', incoming], { home });
    expect(result.exitCode).toBe(0);
    expect(result.stdout.toLowerCase()).toContain('skipped');
    expect(result.stdout).toContain('x');
    expect(JSON.parse(readFileSync(join(home, 'commands.json'), 'utf8')).commands.x.command).toBe('ours');
  });

  test('--theirs overwrites collisions', () => {
    const home = mkdtempSync(join(tmpdir(), 'potato-home-'));
    writeFileSync(join(home, 'commands.json'), lib({ x: { command: 'ours' } }));
    const dir = mkdtempSync(join(tmpdir(), 'potato-in-'));
    const incoming = join(dir, 'theirs.json');
    writeFileSync(incoming, lib({ x: { command: 'theirs' } }));
    const result = run(['import', incoming, '--theirs'], { home });
    expect(result.exitCode).toBe(0);
    expect(JSON.parse(readFileSync(join(home, 'commands.json'), 'utf8')).commands.x.command).toBe('theirs');
  });

  test('an invalid incoming file aborts all-or-nothing with file + reason', () => {
    const home = mkdtempSync(join(tmpdir(), 'potato-home-'));
    writeFileSync(join(home, 'commands.json'), lib({ keep: { command: 'ls' } }));
    const dir = mkdtempSync(join(tmpdir(), 'potato-in-'));
    const incoming = join(dir, 'bad.json');
    writeFileSync(incoming, '{"version": 99, "commands": {}}');
    const result = run(['import', incoming], { home });
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr).toContain('bad.json');
    // library untouched
    expect(JSON.parse(readFileSync(join(home, 'commands.json'), 'utf8')).commands.keep.command).toBe('ls');
  });

  test('- reads from stdin', () => {
    const result = run(['import', '-'], { stdin: lib({ piped: { command: 'echo pipe' } }) });
    expect(result.exitCode).toBe(0);
    const saved = JSON.parse(readFileSync(join(result.home, 'commands.json'), 'utf8'));
    expect(saved.commands.piped.command).toBe('echo pipe');
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
