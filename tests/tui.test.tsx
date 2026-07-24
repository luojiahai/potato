import { describe, expect, test } from 'bun:test';
import React from 'react';
import { render } from 'ink-testing-library';
import { App, type AppDeps } from '../src/tui/App';
import type { CommandEntry, Library } from '../src/library';
import type { State } from '../src/state';

// TUI flow tests (spec §3): split-pane list with fuzzy search, single-form
// arg screen with live preview, footer keybindings, hand-off via callback.
// Identity is the Command id; State is keyed by id; a rename keeps id + slot.

const KEYS = { enter: '\r', esc: '\x1b', ctrlY: '\x19', ctrlA: '\x01', ctrlE: '\x05', ctrlD: '\x04' };

const DEPLOY = 'id-deploy';
const PORTS = 'id-ports';

const byName = (lib: Library, name: string): CommandEntry | undefined =>
  lib.commands.find((c) => c.name === name);

function makeDeps(overrides: Partial<AppDeps> = {}) {
  const library: Library = {
    version: 2,
    commands: [
      { id: DEPLOY, name: 'deploy prod', description: 'Roll out to production', command: "ssh {{host=prod-1}} 'deploy.sh'" },
      { id: PORTS, name: 'list ports', description: 'Show listening processes', command: 'lsof -iTCP -sTCP:LISTEN' },
    ],
  };
  const saved: { library: Library[]; state: State[]; copied: string[] } = { library: [], state: [], copied: [] };
  const deps: AppDeps = {
    library,
    state: {},
    migrated: false,
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

  test('shows the ascii wordmark banner', async () => {
    const { deps } = makeDeps();
    const { lastFrame } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    // the top row of the ANSI-shadow 'T' is distinctive
    expect(lastFrame()!).toContain('████████╗');
  });

  test('commands with Placeholders carry an arg-count badge', async () => {
    const { deps } = makeDeps();
    const { lastFrame } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    // 'deploy prod' has one placeholder, 'list ports' has none
    expect(lastFrame()!).toContain('deploy prod ⌁1');
    expect(lastFrame()!).not.toContain('list ports ⌁');
  });

  test('detail pane shows a relative last-used time from State (keyed by id)', async () => {
    const { deps } = makeDeps({
      state: { [DEPLOY]: { lastUsedAt: '2026-07-24T08:00:00Z' } },
    });
    const { lastFrame } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    expect(lastFrame()!).toContain('used 2h ago');
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

  test('a migrated launch shows a footer toast', async () => {
    const { deps } = makeDeps({ migrated: true });
    const { lastFrame } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    expect(lastFrame()!).toContain('upgraded your library to v2');
  });
});

describe('hand-off', () => {
  test('Enter on a command without Placeholders hands off immediately and records use by id', async () => {
    const { deps, saved } = makeDeps();
    const handoffs: string[] = [];
    const { stdin } = render(<App deps={deps} onHandoff={(c) => handoffs.push(c)} />);
    await tick();
    stdin.write('ports');
    await tick();
    stdin.write(KEYS.enter);
    await tick();
    expect(handoffs).toEqual(['lsof -iTCP -sTCP:LISTEN']);
    expect(saved.state.at(-1)?.[PORTS]?.lastUsedAt).toBe('2026-07-24T10:00:00.000Z');
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
    // last value remembered in State, keyed by id
    expect(saved.state.at(-1)?.[DEPLOY]?.args).toEqual({ host: 'prod-2' });
  });

  test('last value outranks the default on the next visit', async () => {
    const { deps } = makeDeps({
      state: { [DEPLOY]: { lastUsedAt: '2026-07-01T00:00:00Z', args: { host: 'prod-9' } } },
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
  test('Ctrl-A add flow saves a new Command with a fresh id', async () => {
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
    const added = byName(lib, 'pwd now')!;
    expect(added.command).toBe('pwd');
    expect(added.id).toBeTruthy();
    expect(lib.commands).toHaveLength(3);
  });

  test('edit screen shows a live template preview with parsed args', async () => {
    const { deps } = makeDeps();
    const { lastFrame, stdin } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    stdin.write(KEYS.ctrlE); // edit 'deploy prod'
    await tick();
    const frame = lastFrame()!;
    expect(frame).toContain('template');
    expect(frame).toContain("$ ssh {{host=prod-1}} 'deploy.sh'");
    expect(frame).toContain('host = prod-1');
  });

  test('add screen warns live when the name is already taken', async () => {
    const { deps, saved } = makeDeps();
    const { lastFrame, stdin } = render(<App deps={deps} onHandoff={() => {}} />);
    await tick();
    stdin.write(KEYS.ctrlA);
    await tick();
    stdin.write('deploy prod'); // name field — collides
    await tick();
    expect(lastFrame()!).toContain("⚠ 'deploy prod' already exists");
    stdin.write(KEYS.enter); // must not save
    await tick();
    expect(saved.library).toEqual([]);
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
    expect(byName(saved.library.at(-1)!, 'deploy prod')).toBeUndefined();
  });

  test('renaming a Command keeps its id and its slot in the file', async () => {
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
    const lib = saved.library.at(-1)!;
    expect(lib.commands.map((c) => c.name)).toEqual(['deploy stage', 'list ports']);
    // id preserved across the rename (so State survives)
    expect(lib.commands[0]!.id).toBe(DEPLOY);
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
    expect(byName(saved.library.at(-1)!, 'deploy prod')!.command).toBe('ssh prod-3');
  });
});
