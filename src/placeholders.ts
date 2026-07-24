// Placeholders: {{name}} / {{name=default}} (spec §2). Anything else stays
// literal; substitution is verbatim — template authors do their own quoting.

export interface Placeholder {
  name: string;
  default?: string;
}

const PLACEHOLDER_RE = /\{\{([A-Za-z0-9_-]+)(?:=([^}]*))?\}\}/g;

// Unique by name (repeats prompt once); first default wins.
export function parsePlaceholders(template: string): Placeholder[] {
  const seen = new Map<string, Placeholder>();
  for (const m of template.matchAll(PLACEHOLDER_RE)) {
    const name = m[1]!;
    if (!seen.has(name)) seen.set(name, { name, default: m[2] });
  }
  return [...seen.values()];
}

// Value for a name: supplied value > first-wins default > empty — every
// occurrence of a repeated name renders identically.
function resolveValues(template: string, values: Record<string, string>): Record<string, string> {
  const resolved: Record<string, string> = {};
  for (const p of parsePlaceholders(template)) resolved[p.name] = values[p.name] ?? p.default ?? '';
  return resolved;
}

export function renderCommand(template: string, values: Record<string, string>): string {
  const resolved = resolveValues(template, values);
  return template.replace(PLACEHOLDER_RE, (_, name: string) => resolved[name]!);
}

// Split into literal/placeholder segments with the raw {{...}} tokens kept,
// so the edit screen can highlight the Placeholder slots themselves.
export function templateSegments(template: string): Array<{ text: string; placeholder: boolean }> {
  const out: Array<{ text: string; placeholder: boolean }> = [];
  let last = 0;
  for (const m of template.matchAll(PLACEHOLDER_RE)) {
    if (m.index > last) out.push({ text: template.slice(last, m.index), placeholder: false });
    out.push({ text: m[0], placeholder: true });
    last = m.index + m[0].length;
  }
  if (last < template.length) out.push({ text: template.slice(last), placeholder: false });
  return out;
}

// Split into literal/substituted segments so the live preview can highlight
// the substituted values (spec §3.2).
export function renderSegments(
  template: string,
  values: Record<string, string>,
): Array<{ text: string; substituted: boolean }> {
  const resolved = resolveValues(template, values);
  const out: Array<{ text: string; substituted: boolean }> = [];
  let last = 0;
  for (const m of template.matchAll(PLACEHOLDER_RE)) {
    if (m.index > last) out.push({ text: template.slice(last, m.index), substituted: false });
    out.push({ text: resolved[m[1]!]!, substituted: true });
    last = m.index + m[0].length;
  }
  if (last < template.length) out.push({ text: template.slice(last), substituted: false });
  return out;
}
