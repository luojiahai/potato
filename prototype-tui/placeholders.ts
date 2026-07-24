// PROTOTYPE — {{name}} / {{name=default}} parsing and rendering, per ticket #4's decision.

export interface Placeholder {
  name: string;
  default?: string;
}

const RE = /\{\{([A-Za-z0-9_-]+)(?:=([^}]*))?\}\}/g;

// Unique by name (repeats prompt once); first default wins.
export function parsePlaceholders(command: string): Placeholder[] {
  const seen = new Map<string, Placeholder>();
  for (const m of command.matchAll(RE)) {
    if (!seen.has(m[1])) seen.set(m[1], { name: m[1], default: m[2] });
  }
  return [...seen.values()];
}

export function renderCommand(command: string, values: Record<string, string>): string {
  return command.replace(RE, (_, name, def) => values[name] ?? def ?? '');
}

// Split into literal/substituted segments so the preview can highlight values.
export function renderSegments(
  command: string,
  values: Record<string, string>,
): Array<{ text: string; substituted: boolean }> {
  const out: Array<{ text: string; substituted: boolean }> = [];
  let last = 0;
  for (const m of command.matchAll(RE)) {
    if (m.index! > last) out.push({ text: command.slice(last, m.index), substituted: false });
    out.push({ text: values[m[1]] ?? m[2] ?? '', substituted: true });
    last = m.index! + m[0].length;
  }
  if (last < command.length) out.push({ text: command.slice(last), substituted: false });
  return out;
}
