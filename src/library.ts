// The Library: ~/.potato/commands.json (spec §1.1). Fail loud on anything
// invalid — potato never writes to a file it couldn't parse. Unknown fields
// are tolerated and preserved; key order is meaningful and kept.

import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';

export interface CommandEntry {
  command: string;
  description?: string;
  [extra: string]: unknown;
}

export interface Library {
  version: 1;
  commands: Record<string, CommandEntry>;
  [extra: string]: unknown;
}

export class LibraryError extends Error {}

function fail(source: string, reason: string): never {
  throw new LibraryError(`${source}: ${reason}`);
}

export function parseLibrary(text: string, source: string): Library {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch (e) {
    fail(source, `not valid JSON (${(e as Error).message})`);
  }
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) fail(source, 'top level must be an object');
  const obj = raw as Record<string, unknown>;
  if (obj.version !== 1) fail(source, `unsupported version ${JSON.stringify(obj.version)} (expected 1)`);
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
  return obj as Library;
}

export function serializeLibrary(lib: Library): string {
  return JSON.stringify(lib, null, 2) + '\n';
}

export function loadLibrary(path: string): Library {
  if (!existsSync(path)) return { version: 1, commands: {} };
  return parseLibrary(readFileSync(path, 'utf8'), path);
}

export function saveLibrary(path: string, lib: Library): void {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, serializeLibrary(lib));
}
