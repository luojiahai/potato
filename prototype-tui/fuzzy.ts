// PROTOTYPE — minimal subsequence fuzzy matcher, name > description > command.

import type { Command, CommandState } from './data';

// Subsequence match; higher is better, null = no match.
function subsequenceScore(query: string, text: string): number | null {
  const q = query.toLowerCase();
  const t = text.toLowerCase();
  let ti = 0;
  let score = 0;
  let lastMatch = -2;
  for (let qi = 0; qi < q.length; qi++) {
    const found = t.indexOf(q[qi], ti);
    if (found === -1) return null;
    score += found === lastMatch + 1 ? 3 : 1; // consecutive runs score higher
    lastMatch = found;
    ti = found + 1;
  }
  return score - t.length / 100; // slight bias toward shorter targets
}

export function search(
  commands: Command[],
  state: Record<string, CommandState>,
  query: string,
): Command[] {
  if (query.trim() === '') {
    // MRU first, then the rest in insertion order.
    return [...commands].sort(
      (a, b) => (state[b.name]?.lastUsedAt ?? 0) - (state[a.name]?.lastUsedAt ?? 0),
    );
  }
  const scored: Array<{ cmd: Command; score: number }> = [];
  for (const cmd of commands) {
    const name = subsequenceScore(query, cmd.name);
    const desc = cmd.description ? subsequenceScore(query, cmd.description) : null;
    const body = subsequenceScore(query, cmd.command);
    const best = Math.max(
      name === null ? -Infinity : name * 100,
      desc === null ? -Infinity : desc * 10,
      body === null ? -Infinity : body,
    );
    if (best !== -Infinity) scored.push({ cmd, score: best });
  }
  return scored.sort((a, b) => b.score - a.score).map((s) => s.cmd);
}
