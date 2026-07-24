// The Library: ~/.potato/commands.json (v2). A Command is identified by a
// stable `id` (a UUID); its `name` is a unique, human-facing field. Fail loud
// on anything invalid — potato never writes to a file it couldn't parse.
// Unknown fields are tolerated and preserved; array order is meaningful and
// kept (renames hold their slot, new entries append).

import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';

export interface CommandEntry {
  id: string;
  name: string;
  description?: string;
  command: string;
  [extra: string]: unknown;
}

export interface Library {
  version: 2;
  commands: CommandEntry[];
  [extra: string]: unknown;
}

export class LibraryError extends Error {}

function fail(source: string, reason: string): never {
  throw new LibraryError(`${source}: ${reason}`);
}

export function emptyLibrary(): Library {
  return { version: 2, commands: [] };
}

// Version-strict: parses v2 and fail-loud rejects anything else, including v1
// (v1 files are upgraded by the migration loader, not by this parser).
export function parseLibrary(text: string, source: string): Library {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch (e) {
    fail(source, `not valid JSON (${(e as Error).message})`);
  }
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) fail(source, 'top level must be an object');
  const obj = raw as Record<string, unknown>;
  if (obj.version !== 2) fail(source, `unsupported version ${JSON.stringify(obj.version)} (expected 2)`);
  const commands = obj.commands;
  if (!Array.isArray(commands)) fail(source, '"commands" must be an array of entries');

  const ids = new Set<string>();
  const names = new Set<string>();
  commands.forEach((entry, i) => {
    if (typeof entry !== 'object' || entry === null || Array.isArray(entry))
      fail(source, `command at index ${i} must be an object`);
    const e = entry as Record<string, unknown>;
    // id: loose — non-empty string, unique within the file (UUID format not enforced)
    if (typeof e.id !== 'string' || e.id === '')
      fail(source, `command at index ${i} needs a non-empty "id" string`);
    if (ids.has(e.id)) fail(source, `duplicate id ${JSON.stringify(e.id)}`);
    ids.add(e.id);
    // name: unique, case-sensitive, non-empty after trimming
    if (typeof e.name !== 'string' || e.name.trim() === '')
      fail(source, `command ${JSON.stringify(e.id)} needs a non-empty "name"`);
    if (names.has(e.name)) fail(source, `duplicate name ${JSON.stringify(e.name)}`);
    names.add(e.name);
    if (typeof e.command !== 'string' || e.command === '')
      fail(source, `command ${JSON.stringify(e.name)} needs a non-empty "command" string`);
    if (e.description !== undefined && typeof e.description !== 'string')
      fail(source, `command ${JSON.stringify(e.name)} has a non-string "description"`);
  });
  return obj as Library;
}

export function serializeLibrary(lib: Library): string {
  return JSON.stringify(lib, null, 2) + '\n';
}

export function loadLibrary(path: string): Library {
  if (!existsSync(path)) return emptyLibrary();
  return parseLibrary(readFileSync(path, 'utf8'), path);
}

// Atomic write (temp + rename): a failed write leaves the original untouched,
// so a crashed migration or save never corrupts the Library.
export function saveLibrary(path: string, lib: Library): void {
  mkdirSync(dirname(path), { recursive: true });
  const tmp = `${path}.${process.pid}.tmp`;
  writeFileSync(tmp, serializeLibrary(lib));
  renameSync(tmp, path);
}

export const findById = (lib: Library, id: string): CommandEntry | undefined =>
  lib.commands.find((c) => c.id === id);
