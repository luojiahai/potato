import { describe, expect, test } from 'bun:test';
import React from 'react';
import { render } from 'ink-testing-library';
import { App, type AppDeps } from '../src/tui/App';
import type { Library } from '../src/library';
import type { State } from '../src/state';

// TUI flow tests (spec §3): split-pane list with fuzzy search, single-form
// arg screen with live preview, footer keybindings, hand-off via callback.

const KEYS = { enter: '\r', esc: '\x1b', ctrlY: '\x19', ctrlA: '\x01', ctrlE: '\x05', ctrlD: '\x04' };

function makeDeps(overrides: Partial<AppDeps> = {}) {
  const library: Library = {
    version: 1,
    commands: {
      'deploy prod': { command: "ssh {{host=prod-1}} 'deploy.sh'", description: 'Roll out to production' },
      'list ports': { command: 'lsof -iTCP -sTCP:LISTEN', description: 'Show listening processes' },
    },
  };
  const saved: { library: Library[]; state: State[]; copied: string[] } = { library: [], state: [], copied: [] };
  const deps: AppDeps = {
    library,
    state: {},
    saveLibrary: (lib) => saved.library.push(structuredClone(lib)),
    saveState: (s) => saved.state.push(structuredClone(s)),
    copy: (text) => saved.copied.push(text),
    now: () => new Date('2026-07-24T10:00:00Z'),
    ...overrides,
  };
  return { deps, saved };
}

const tick = () => new Promise((r) => setTimeout(r, 20));

describe('list screen', () => {
  test('shows command names and the detail pane for the selection', async () => {
    const { deps } = makeDeps();
    const { lastFrame } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    const frame = lastFrame()!;
    expect(frame).toContain('deploy prod');
    expect(frame).toContain('list ports');
    // detail pane of the first (selected) command
    expect(frame).toContain('Roll out to production');
    expect(frame).toContain("ssh {{host=prod-1}} 'deploy.sh'");
    // placeholder listed with its default
    expect(frame).toContain('host');
    expect(frame).toContain('prod-1');
  });

  test('typing filters the list', async () => {
    const { deps } = makeDeps();
    const { lastFrame, stdin } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    stdin.write('ports');
    await tick();
    const frame = lastFrame()!;
    expect(frame).toContain('list ports');
    expect(frame).not.toContain('deploy prod');
  });
});

describe('hand-off', () => {
  test('Enter on a command without Placeholders hands off immediately and records use', async () => {
    const { deps, saved } = makeDeps();
    const handoffs: string[] = [];
    const { stdin } = render(<App deps={deps} onHandoff={(c) => handoffs.push(c)} />);
    await tick();
    stdin.write('ports');
    await tick();
    stdin.write(KEYS.enter);
    await tick();
    expect(handoffs).toEqual(['lsof -iTCP -sTCP:LISTEN']);
    expect(saved.state.at(-1)?.['list ports']?.lastUsedAt).toBe('2026-07-24T10:00:00.000Z');
  });

  test('Enter on a templated command opens the arg form pre-filled with the default', async () => {
    const { deps } = makeDeps();
    const { lastFrame, stdin } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    stdin.write(KEYS.enter);
    await tick();
    const frame = lastFrame()!;
    expect(frame).toContain('host');
    // live preview shows the default substituted
    expect(frame).toContain("ssh prod-1 'deploy.sh'");
  });

  test('editing a field updates the preview; Enter hands off the rendered command', async () => {
    const { deps, saved } = makeDeps();
    const handoffs: string[] = [];
    const { lastFrame, stdin } = render(<App deps={deps} onHandoff={(c) => handoffs.push(c)} />);
    await tick();
    stdin.write(KEYS.enter); // open arg form for deploy prod
    await tick();
    for (let i = 0; i < 'prod-1'.length; i++) stdin.write('\x7f'); // clear default
    stdin.write('prod-2');
    await tick();
    expect(lastFrame()!).toContain("ssh prod-2 'deploy.sh'");
    stdin.write(KEYS.enter);
    await tick();
    expect(handoffs).toEqual(["ssh prod-2 'deploy.sh'"]);
    // last value remembered in State
    expect(saved.state.at(-1)?.['deploy prod']?.args).toEqual({ host: 'prod-2' });
  });

  test('last value outranks the default on the next visit', async () => {
    const { deps } = makeDeps({
      state: { 'deploy prod': { lastUsedAt: '2026-07-01T00:00:00Z', args: { host: 'prod-9' } } },
    });
    const { lastFrame, stdin } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    stdin.write(KEYS.enter);
    await tick();
    expect(lastFrame()!).toContain("ssh prod-9 'deploy.sh'");
  });

  test('Ctrl-Y copies the rendered command instead of running', async () => {
    const { deps, saved } = makeDeps();
    const { stdin } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    stdin.write('ports');
    await tick();
    stdin.write(KEYS.ctrlY);
    await tick();
    expect(saved.copied).toEqual(['lsof -iTCP -sTCP:LISTEN']);
  });
});

describe('CRUD', () => {
  test('Ctrl-A add flow saves a new Command to the Library', async () => {
    const { deps, saved } = makeDeps();
    const { stdin } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    stdin.write(KEYS.ctrlA);
    await tick();
    stdin.write('pwd now'); // name field
    await tick();
    stdin.write('\t');
    await tick();
    stdin.write('pwd'); // command field
    await tick();
    stdin.write(KEYS.enter);
    await tick();
    const lib = saved.library.at(-1)!;
    expect(lib.commands['pwd now']).toEqual({ command: 'pwd' });
  });

  test('Ctrl-D asks for confirmation and y deletes', async () => {
    const { deps, saved } = makeDeps();
    const { lastFrame, stdin } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    stdin.write(KEYS.ctrlD);
    await tick();
    expect(lastFrame()!.toLowerCase()).toContain('delete');
    stdin.write('y');
    await tick();
    expect(saved.library.at(-1)!.commands['deploy prod']).toBeUndefined();
  });

  test('renaming a Command keeps its position in the file (order is meaningful)', async () => {
    const { deps, saved } = makeDeps();
    const { stdin } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    stdin.write(KEYS.ctrlE); // edit first command: 'deploy prod'
    await tick();
    for (let i = 0; i < 20; i++) stdin.write('\x7f'); // clear name field
    await tick();
    stdin.write('deploy stage');
    await tick();
    stdin.write(KEYS.enter);
    await tick();
    expect(Object.keys(saved.library.at(-1)!.commands)).toEqual(['deploy stage', 'list ports']);
  });

  test('Ctrl-E edits the selected Command in place', async () => {
    const { deps, saved } = makeDeps();
    const { stdin } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    stdin.write(KEYS.ctrlE);
    await tick();
    stdin.write('\t'); // to command field
    await tick();
    for (let i = 0; i < 40; i++) stdin.write('\x7f');
    await tick();
    stdin.write('ssh prod-3');
    await tick();
    stdin.write(KEYS.enter);
    await tick();
    expect(saved.library.at(-1)!.commands['deploy prod']!.command).toBe('ssh prod-3');
  });
});
