import { describe, expect, test } from 'bun:test';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { loadState, recordUse, saveState } from '../src/state';

// state.json is a disposable per-Command cache, keyed by Command id —
// unreadable means silently reset to {}. lastUsedAt drives MRU; args are last
// Placeholder values.

const dir = () => mkdtempSync(join(tmpdir(), 'potato-state-'));

describe('loadState', () => {
  test('missing file is an empty state', () => {
    expect(loadState(join(dir(), 'state.json'))).toEqual({});
  });

  test('corrupt file silently resets to empty', () => {
    const file = join(dir(), 'state.json');
    writeFileSync(file, '{ nope');
    expect(loadState(file)).toEqual({});
  });

  test('non-object content silently resets to empty', () => {
    const file = join(dir(), 'state.json');
    writeFileSync(file, '[1, 2]');
    expect(loadState(file)).toEqual({});
  });
});

describe('recordUse + save/load round trip', () => {
  test('stores lastUsedAt and merges arg values per command id', () => {
    const file = join(dir(), 'state.json');
    let state = loadState(file);
    state = recordUse(state, 'cmd-1', { host: 'prod-2' }, new Date('2026-07-24T09:12:00Z'));
    saveState(file, state);
    const loaded = loadState(file);
    expect(loaded['cmd-1']!.lastUsedAt).toBe('2026-07-24T09:12:00.000Z');
    expect(loaded['cmd-1']!.args).toEqual({ host: 'prod-2' });
  });

  test('a later use updates the timestamp and merges new args over old', () => {
    let state = recordUse({}, 'x', { a: '1', b: '2' }, new Date('2026-01-01T00:00:00Z'));
    state = recordUse(state, 'x', { b: '3' }, new Date('2026-02-01T00:00:00Z'));
    expect(state['x']!.lastUsedAt).toBe('2026-02-01T00:00:00.000Z');
    expect(state['x']!.args).toEqual({ a: '1', b: '3' });
  });
});
