// Fuzzy search over the Library (spec §3.1): subsequence match on name +
// description + command text, name hits weighted highest, then description,
// then command. Empty query = MRU first (State.lastUsedAt), never-used in
// file order.

import type { CommandEntry } from './library';
import type { State } from './state';

// Subsequence match; higher is better, null = no match. Consecutive runs
// score higher; slight bias toward shorter targets.
function subsequenceScore(query: string, text: string): number | null {
  const q = query.toLowerCase();
  const t = text.toLowerCase();
  let ti = 0;
  let score = 0;
  let lastMatch = -2;
  for (let qi = 0; qi < q.length; qi++) {
    const found = t.indexOf(q[qi]!, ti);
    if (found === -1) return null;
    score += found === lastMatch + 1 ? 3 : 1;
    lastMatch = found;
    ti = found + 1;
  }
  return score - t.length / 100;
}

// Greedy subsequence match positions of query within name — the same walk the
// scorer takes — for match highlighting in the TUI list. null = no name match
// (the row may still have matched via description/command) or empty query.
export function nameMatchIndices(query: string, name: string): Set<number> | null {
  if (query.trim() === '') return null;
  const q = query.toLowerCase();
  const t = name.toLowerCase();
  const indices = new Set<number>();
  let ti = 0;
  for (let qi = 0; qi < q.length; qi++) {
    const found = t.indexOf(q[qi]!, ti);
    if (found === -1) return null;
    indices.add(found);
    ti = found + 1;
  }
  return indices;
}

export function searchCommands(
  commands: Record<string, CommandEntry>,
  state: State,
  query: string,
): string[] {
  const names = Object.keys(commands);
  if (query.trim() === '') {
    const used = names.filter((n) => state[n]?.lastUsedAt);
    const rest = names.filter((n) => !state[n]?.lastUsedAt);
    used.sort((a, b) => Date.parse(state[b]!.lastUsedAt) - Date.parse(state[a]!.lastUsedAt));
    return [...used, ...rest];
  }
  const scored: Array<{ name: string; score: number }> = [];
  for (const name of names) {
    const entry = commands[name]!;
    const byName = subsequenceScore(query, name);
    const byDesc = entry.description ? subsequenceScore(query, entry.description) : null;
    const byBody = subsequenceScore(query, entry.command);
    const best = Math.max(
      byName === null ? -Infinity : byName * 100,
      byDesc === null ? -Infinity : byDesc * 10,
      byBody === null ? -Infinity : byBody,
    );
    if (best !== -Infinity) scored.push({ name, score: best });
  }
  return scored.sort((a, b) => b.score - a.score).map((s) => s.name);
}
