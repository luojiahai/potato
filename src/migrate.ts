// v1 → v2 migration. v1 keyed Commands by name; v2 identifies them by a stable
// UUID with the name as a unique field (see library.ts). Migration runs
// auto-on-load: an entry point reading the Library detects `version: 1`,
// upgrades it in memory, and writes the v2 file back atomically. The name→id
// map minted here — the only moment it exists — also rekeys state.json so
// last-used history survives the upgrade.

import { existsSync, readFileSync } from 'node:fs';
import { randomUUID } from 'node:crypto';
import {
  emptyLibrary,
  LibraryError,
  parseLibrary,
  saveLibrary,
  type CommandEntry,
  type Library,
} from './library';
import { loadState, saveState, type State } from './state';

// ---------- retained v1 reader ----------

interface CommandEntryV1 {
  command: string;
  description?: string;
  [extra: string]: unknown;
}

interface LibraryV1 {
  version: 1;
  commands: Record<string, CommandEntryV1>;
  [extra: string]: unknown;
}

function fail(source: string, reason: string): never {
  throw new LibraryError(`${source}: ${reason}`);
}

// Parses a v1 file, fail-loud on anything invalid — the same guarantee the v1
// loader gave, kept so migration only ever upgrades a file it fully understood.
export function parseLibraryV1(text: string, source: string): LibraryV1 {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch (e) {
    fail(source, `not valid JSON (${(e as Error).message})`);
  }
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) fail(source, 'top level must be an object');
  const obj = raw as Record<string, unknown>;
  if (obj.version !== 1) fail(source, `expected a version 1 library`);
  const commands = obj.commands;
  if (typeof commands !== 'object' || commands === null || Array.isArray(commands))
    fail(source, '"commands" must be an object keyed by command name');
  for (const [name, entry] of Object.entries(commands)) {
    if (name.trim() === '') fail(source, 'command names must be non-empty after trimming');
    if (typeof entry !== 'object' || entry === null || Array.isArray(entry))
      fail(source, `command ${JSON.stringify(name)} must be an object`);
    const e = entry as Record<string, unknown>;
    if (typeof e.command !== 'string' || e.command === '')
      fail(source, `command ${JSON.stringify(name)} needs a non-empty "command" string`);
    if (e.description !== undefined && typeof e.description !== 'string')
      fail(source, `command ${JSON.stringify(name)} has a non-string "description"`);
  }
  return obj as LibraryV1;
}

// ---------- pure migration ----------

// Assigns each v1 Command a fresh UUID, producing the v2 array. Returns the
// name→id map so state.json can be rekeyed in the same operation.
export function migrateLibrary(v1: LibraryV1): { library: Library; nameToId: Map<string, string> } {
  const { version, commands: v1commands, ...extra } = v1;
  const nameToId = new Map<string, string>();
  const commands: CommandEntry[] = [];
  for (const [name, entry] of Object.entries(v1commands)) {
    const { command, description, ...entryExtra } = entry;
    const id = randomUUID();
    nameToId.set(name, id);
    const e: CommandEntry = { id, name, ...(description !== undefined ? { description } : {}), command, ...entryExtra };
    commands.push(e);
  }
  return { library: { version: 2, commands, ...extra }, nameToId };
}

// Moves each State entry from its name key to the new id. Orphans — a state
// key whose Command no longer exists — are dropped; State is disposable.
export function rekeyState(state: State, nameToId: Map<string, string>): State {
  const next: State = {};
  for (const [name, entry] of Object.entries(state)) {
    const id = nameToId.get(name);
    if (id !== undefined) next[id] = entry;
  }
  return next;
}

// ---------- load orchestration ----------

export interface LoadResult {
  library: Library;
  state: State;
  migrated: boolean;
}

// The Library load path for every app entry point: read → if v1, migrate +
// write both files back → parse as v2. A v1 Library's commands.json write is
// atomic and authoritative (fail-loud, original preserved on failure); the
// state.json rekey rides along as best-effort (its loss is disposable).
export function loadLibraryMigrating(commandsPath: string, statePath: string): LoadResult {
  const state = loadState(statePath);
  if (!existsSync(commandsPath)) return { library: emptyLibrary(), state, migrated: false };

  const text = readFileSync(commandsPath, 'utf8');
  if (peekVersion(text) === 1) {
    const { library, nameToId } = migrateLibrary(parseLibraryV1(text, commandsPath));
    saveLibrary(commandsPath, library); // atomic + authoritative — fail loud, leave v1 intact on failure
    const rekeyed = rekeyState(state, nameToId);
    try {
      saveState(statePath, rekeyed);
    } catch {
      // best-effort: a lost state rewrite only forfeits disposable timestamps
    }
    return { library, state: rekeyed, migrated: true };
  }

  // v2, or an unknown/future version — parseLibrary fail-louds on the latter.
  return { library: parseLibrary(text, commandsPath), state, migrated: false };
}

// Cheap peek at the version to route the load; a malformed file returns
// undefined and falls through to parseLibrary, which reports the real error.
function peekVersion(text: string): unknown {
  try {
    return (JSON.parse(text) as { version?: unknown })?.version;
  } catch {
    return undefined;
  }
}
