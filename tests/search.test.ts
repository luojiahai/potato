import { describe, expect, test } from 'bun:test';
import { searchCommands } from '../src/search';
import type { CommandEntry } from '../src/library';
import type { State } from '../src/state';

// Spec §3.1: match over name + description + command, name weighted highest,
// then description, then command. Empty query: MRU first, never-used follow
// in file order.

const commands: Record<string, CommandEntry> = {
  'deploy prod': { command: 'ssh prod-1 deploy.sh', description: 'Roll out to production' },
  'tail logs': { command: 'aws logs tail /ecs/api --follow', description: 'Tail ECS logs' },
  'docker nuke': { command: 'docker system prune -af', description: 'Remove unused containers' },
  'list ports': { command: 'lsof -iTCP -sTCP:LISTEN', description: 'Show listening processes' },
};

describe('empty query', () => {
  test('MRU first, never-used in file order after', () => {
    const state: State = {
      'docker nuke': { lastUsedAt: '2026-07-20T00:00:00Z' },
      'tail logs': { lastUsedAt: '2026-07-23T00:00:00Z' },
    };
    expect(searchCommands(commands, state, '')).toEqual([
      'tail logs',
      'docker nuke',
      'deploy prod',
      'list ports',
    ]);
  });

  test('no state at all keeps pure file order', () => {
    expect(searchCommands(commands, {}, '')).toEqual([
      'deploy prod',
      'tail logs',
      'docker nuke',
      'list ports',
    ]);
  });
});

describe('fuzzy matching', () => {
  test('subsequence matches on the name', () => {
    expect(searchCommands(commands, {}, 'dpl')).toContain('deploy prod');
  });

  test('non-matching commands are filtered out', () => {
    expect(searchCommands(commands, {}, 'zzzz')).toEqual([]);
  });

  test('a name hit outranks a description hit', () => {
    // "tail" is the name of one command and in the description of another
    const result = searchCommands(commands, {}, 'tail');
    expect(result[0]).toBe('tail logs');
  });

  test('a description hit outranks a command-text hit', () => {
    // "listen" appears in list ports' description and docker nuke matches nothing
    const cmds: Record<string, CommandEntry> = {
      a: { command: 'echo listening' },
      b: { command: 'echo x', description: 'listening things' },
    };
    expect(searchCommands(cmds, {}, 'listening')[0]).toBe('b');
  });
});
