import { describe, expect, test } from 'bun:test';
import { parsePlaceholders, renderCommand } from '../src/placeholders';

// Spec §2: {{name}} / {{name=default}}, name = [A-Za-z0-9_-]+, no escapes,
// repeats prompt once (first default wins), verbatim substitution.

describe('parsePlaceholders', () => {
  test('finds named placeholders with and without defaults', () => {
    expect(parsePlaceholders("ssh {{host=prod-1}} 'deploy {{env}}'")).toEqual([
      { name: 'host', default: 'prod-1' },
      { name: 'env', default: undefined },
    ]);
  });

  test('leaves non-placeholder brace forms literal', () => {
    expect(parsePlaceholders("awk '{print $1}' ${HOME}/f {{ spaced }} {1}")).toEqual([]);
  });

  test('a repeated name appears once; first default wins', () => {
    expect(parsePlaceholders('cp {{f=a.txt}} {{f=b.txt}}.bak {{f}}')).toEqual([
      { name: 'f', default: 'a.txt' },
    ]);
  });
});

describe('renderCommand', () => {
  test('substitutes verbatim, character for character', () => {
    expect(renderCommand('git commit -m "{{msg}}"', { msg: 'fix: a "quoted" thing' })).toBe(
      'git commit -m "fix: a "quoted" thing"',
    );
  });

  test('a repeated name fills every occurrence', () => {
    expect(renderCommand('cp {{f}} {{f}}.bak', { f: 'notes.md' })).toBe('cp notes.md notes.md.bak');
  });

  test('empty values render as empty string', () => {
    expect(renderCommand('ls {{flags}} .', { flags: '' })).toBe('ls  .');
  });

  test('missing value falls back to the default, then empty', () => {
    expect(renderCommand('ssh {{host=prod-1}} {{cmd}}', {})).toBe('ssh prod-1 ');
  });

  test('on conflicting defaults for a repeated name, the first wins everywhere', () => {
    expect(renderCommand('cp {{f=a.txt}} {{f=b.txt}}.bak', {})).toBe('cp a.txt a.txt.bak');
  });

  test('literal brace forms pass through untouched', () => {
    expect(renderCommand("awk '{print $1}' ${HOME}", {})).toBe("awk '{print $1}' ${HOME}");
  });
});
