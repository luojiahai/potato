// PROTOTYPE — throwaway. Answers https://github.com/luojiahai/potato/issues/25.
//
// Three cursor conventions for potato's text fields, switchable with `esc`,
// rendered on real copies of the search row and the edit panel's name /
// command / description fields. (An arg field is visually identical to `name`
// — same `Field`, plain value — so it isn't rendered separately.)
//
//   A  block      inverse video on the grapheme under the cursor
//   B  bar        a thin ▏ between graphemes, occupying its own column
//   C  underline  underline the grapheme under the cursor, colour preserved
//
// Each variant also takes a different position on the multi-line gutter:
// A blank continuation, B a dim rail, C right-aligned line numbers.
//
// Run:  bun run src/tui/cursor-prototype.tsx
//
// Not production: no tests, no persistence, dumb word motion (the real
// definition is issue #23's question), and the keymap is only enough to drive
// the cursor around.

import React, { useState } from 'react';
import { Box, render, Text, useInput } from 'ink';
import { templateSegments } from '../placeholders';
import { bunSafeStdin } from './stdin';

const ACCENT = '#ffaf5f';

// ---------- graphemes ----------

const SEG = new Intl.Segmenter(undefined, { granularity: 'grapheme' });

function bounds(s: string): number[] {
  const out = [0];
  for (const g of SEG.segment(s)) out.push(g.index + g.segment.length);
  return out;
}

function stepLeft(s: string, i: number): number {
  const b = bounds(s);
  const k = b.findIndex((x) => x >= i);
  return b[Math.max(0, k - 1)] ?? 0;
}

function stepRight(s: string, i: number): number {
  return bounds(s).find((x) => x > i) ?? s.length;
}

// Whitespace-delimited only — what "word" means in shell text is issue #23.
function wordLeft(s: string, i: number): number {
  let j = i;
  while (j > 0 && /\s/.test(s[j - 1]!)) j--;
  while (j > 0 && !/\s/.test(s[j - 1]!)) j--;
  return j;
}

function wordRight(s: string, i: number): number {
  let j = i;
  while (j < s.length && /\s/.test(s[j]!)) j++;
  while (j < s.length && !/\s/.test(s[j]!)) j++;
  return j;
}

const lineStart = (s: string, i: number) => s.lastIndexOf('\n', i - 1) + 1;
const lineEnd = (s: string, i: number) => (s.indexOf('\n', i) === -1 ? s.length : s.indexOf('\n', i));

// ---------- spans → lines ----------

type Span = { text: string; color?: string; bold?: boolean };
type Piece = Span & { start: number };
type Line = { pieces: Piece[]; start: number; end: number };

function toLines(spans: Span[]): Line[] {
  const lines: Line[] = [{ pieces: [], start: 0, end: 0 }];
  let abs = 0;
  for (const s of spans) {
    s.text.split('\n').forEach((part, i) => {
      if (i > 0) {
        lines[lines.length - 1]!.end = abs;
        abs += 1;
        lines.push({ pieces: [], start: abs, end: abs });
      }
      if (part !== '') lines[lines.length - 1]!.pieces.push({ ...s, text: part, start: abs });
      abs += part.length;
    });
  }
  lines[lines.length - 1]!.end = abs;
  return lines;
}

// ---------- variants ----------

type Variant = {
  key: string;
  name: string;
  blurb: string;
  line: (pieces: Piece[], cursor: number | null, end: number) => React.ReactNode;
  gutter: (label: string, idx: number, total: number, focused: boolean) => React.ReactNode;
};

/** Splits the piece holding the cursor into before / under / after. */
function split(p: Piece, cursor: number): [string, string, string] {
  const c = cursor - p.start;
  const e = stepRight(p.text, c);
  return [p.text.slice(0, c), p.text.slice(c, e), p.text.slice(e)];
}

const holds = (p: Piece, cursor: number | null) =>
  cursor !== null && cursor >= p.start && cursor < p.start + p.text.length;

const BLANK_GUTTER = (label: string, idx: number, _t: number, focused: boolean) => (
  <Box width={14}>
    <Text bold color={focused ? 'cyan' : undefined}>
      {idx === 0 ? `${focused ? '❯ ' : '  '}${label}` : ''}
    </Text>
  </Box>
);

const VARIANTS: Variant[] = [
  {
    key: 'A',
    name: 'block',
    blurb: 'inverse video on the char under the cursor · blank continuation gutter',
    line: (pieces, cursor, end) => (
      <>
        {pieces.map((p, i) => {
          if (!holds(p, cursor)) {
            return (
              <Text key={i} color={p.color} bold={p.bold}>
                {p.text}
              </Text>
            );
          }
          const [before, under, after] = split(p, cursor!);
          return (
            <Text key={i}>
              <Text color={p.color} bold={p.bold}>{before}</Text>
              <Text color={p.color} bold={p.bold} inverse>{under}</Text>
              <Text color={p.color} bold={p.bold}>{after}</Text>
            </Text>
          );
        })}
        {cursor !== null && cursor >= end && <Text inverse> </Text>}
      </>
    ),
    gutter: BLANK_GUTTER,
  },
  {
    key: 'B',
    name: 'bar',
    blurb: 'thin ▏ between chars, own column · dim rail in the continuation gutter',
    line: (pieces, cursor, end) => (
      <>
        {pieces.map((p, i) => {
          if (!holds(p, cursor)) {
            return (
              <Text key={i} color={p.color} bold={p.bold}>
                {p.text}
              </Text>
            );
          }
          const [before, under, after] = split(p, cursor!);
          return (
            <Text key={i}>
              <Text color={p.color} bold={p.bold}>{before}</Text>
              <Text bold color={ACCENT}>▏</Text>
              <Text color={p.color} bold={p.bold}>{under}{after}</Text>
            </Text>
          );
        })}
        {cursor !== null && cursor >= end && <Text bold color={ACCENT}>▏</Text>}
      </>
    ),
    gutter: (label, idx, _t, focused) => (
      <Box width={14} justifyContent={idx === 0 ? 'flex-start' : 'flex-end'}>
        <Text bold color={focused ? 'cyan' : undefined}>
          {idx === 0 ? `${focused ? '❯ ' : '  '}${label}` : ''}
        </Text>
        {idx > 0 && <Text dimColor>│ </Text>}
      </Box>
    ),
  },
  {
    key: 'C',
    name: 'underline',
    blurb: 'underline the char, colour preserved · line numbers in the gutter',
    line: (pieces, cursor, end) => (
      <>
        {pieces.map((p, i) => {
          if (!holds(p, cursor)) {
            return (
              <Text key={i} color={p.color} bold={p.bold}>
                {p.text}
              </Text>
            );
          }
          const [before, under, after] = split(p, cursor!);
          return (
            <Text key={i}>
              <Text color={p.color} bold={p.bold}>{before}</Text>
              <Text color={p.color} bold underline>{under}</Text>
              <Text color={p.color} bold={p.bold}>{after}</Text>
            </Text>
          );
        })}
        {cursor !== null && cursor >= end && <Text underline color={ACCENT}> </Text>}
      </>
    ),
    gutter: (label, idx, total, focused) => (
      <Box width={14}>
        <Text bold color={focused ? 'cyan' : undefined}>
          {idx === 0 ? `${focused ? '❯ ' : '  '}${label}` : ''}
        </Text>
        <Box flexGrow={1} />
        {total > 1 && <Text dimColor>{idx + 1} </Text>}
      </Box>
    ),
  },
];

// ---------- the field ----------

function ProtoField(props: {
  label: string;
  value: string;
  offset: number;
  focused: boolean;
  variant: Variant;
  spans?: Span[];
  hint?: string;
}) {
  const spans = props.spans ?? [{ text: props.value }];
  const lines = toLines(spans);
  return (
    <>
      {lines.map((ln, i) => (
        <Box key={i}>
          {props.variant.gutter(props.label, i, lines.length, props.focused)}
          <Text>
            {props.variant.line(
              ln.pieces,
              props.focused && props.offset >= ln.start && props.offset <= ln.end ? props.offset : null,
              ln.end,
            )}
            {props.hint && i === 0 && <Text dimColor>  {props.hint}</Text>}
          </Text>
        </Box>
      ))}
    </>
  );
}

function Panel(props: { title: string; children: React.ReactNode; color?: string }) {
  return (
    <Box borderStyle="round" borderColor={props.color ?? 'gray'} flexDirection="column" paddingX={1}>
      <Box marginTop={-1}>
        <Text bold color={props.color}> {props.title} </Text>
      </Box>
      {props.children}
    </Box>
  );
}

// ---------- app ----------

const ORDER = ['query', 'name', 'command', 'description'] as const;
type FieldName = (typeof ORDER)[number];

const INITIAL: Record<FieldName, string> = {
  query: 'deploy staging',
  name: 'deploy',
  command: 'ssh {{host=web-01}} \\\n  "cd /srv/{{app}} && git pull && \\\n   systemctl restart {{app}}"',
  description: '',
};

function App() {
  const [vi, setVi] = useState(0);
  const [focus, setFocus] = useState<number>(2); // command, the interesting one
  const [values, setValues] = useState(INITIAL);
  const [offsets, setOffsets] = useState<Record<FieldName, number>>({
    query: INITIAL.query.length,
    name: INITIAL.name.length,
    command: 30,
    description: 0,
  });

  const variant = VARIANTS[vi]!;
  const field = ORDER[focus]!;
  const value = values[field]!;
  const offset = offsets[field]!;
  const multiline = field === 'command';

  const setOffset = (n: number) => setOffsets((o) => ({ ...o, [field]: Math.max(0, Math.min(value.length, n)) }));
  const edit = (next: string, at: number) => {
    setValues((v) => ({ ...v, [field]: next }));
    setOffsets((o) => ({ ...o, [field]: at }));
  };

  useInput((input, key) => {
    if (key.escape) return setVi((i) => (i + 1) % VARIANTS.length);
    if (key.tab) return setFocus((f) => (f + (key.shift ? -1 : 1) + ORDER.length) % ORDER.length);

    // opt+←/→ arrive as {meta, input:'b'/'f'} in Ghostty — see issue #20.
    if (key.meta && input === 'b') return setOffset(wordLeft(value, offset));
    if (key.meta && input === 'f') return setOffset(wordRight(value, offset));

    if (key.leftArrow) return setOffset(stepLeft(value, offset));
    if (key.rightArrow) return setOffset(stepRight(value, offset));

    if (key.upArrow || key.downArrow) {
      const start = lineStart(value, offset);
      const col = offset - start;
      if (key.upArrow) {
        if (!multiline || start === 0) return setFocus((f) => (f - 1 + ORDER.length) % ORDER.length);
        const prev = lineStart(value, start - 1);
        return setOffset(Math.min(prev + col, start - 1));
      }
      const end = lineEnd(value, offset);
      if (!multiline || end === value.length) return setFocus((f) => (f + 1) % ORDER.length);
      const nextEnd = lineEnd(value, end + 1);
      return setOffset(Math.min(end + 1 + col, nextEnd));
    }

    if (key.return && multiline) return edit(value.slice(0, offset) + '\n' + value.slice(offset), offset + 1);

    if (key.backspace || key.delete || input === '\x7f') {
      if (offset === 0) return;
      const at = stepLeft(value, offset);
      return edit(value.slice(0, at) + value.slice(offset), at);
    }

    if (input && !key.ctrl && !key.meta && input >= ' ') {
      return edit(value.slice(0, offset) + input + value.slice(offset), offset + input.length);
    }
  });

  const spansFor = (name: FieldName): Span[] | undefined =>
    name === 'command'
      ? templateSegments(values.command).map((s) => ({
          text: s.text,
          color: s.placeholder ? 'yellow' : 'cyan',
          bold: s.placeholder,
        }))
      : undefined;

  const qLines = toLines([{ text: values.query }]);

  return (
    <Box flexDirection="column">
      <Panel title="potato" color="yellow">
        <Box>
          <Text bold color="cyan">/ </Text>
          <Text>
            {variant.line(qLines[0]!.pieces, focus === 0 ? offsets.query : null, qLines[0]!.end)}
          </Text>
          <Box flexGrow={1} />
          <Text dimColor>
            {values.query === '' && '(recently used first)  '}
            12/40
          </Text>
        </Box>
      </Panel>

      <Panel title="new command">
        <ProtoField label="name" value={values.name} offset={offsets.name} focused={focus === 1} variant={variant} />
        <ProtoField
          label="command"
          value={values.command}
          offset={offsets.command}
          focused={focus === 2}
          variant={variant}
          spans={spansFor('command')}
        />
        <ProtoField
          label="description"
          value={values.description}
          offset={offsets.description}
          focused={focus === 3}
          variant={variant}
          hint="(optional)"
        />
      </Panel>

      <Panel title="template">
        <Text wrap="wrap">
          <Text dimColor>$ </Text>
          {templateSegments(values.command).map((seg, i) => (
            <Text key={i} bold={seg.placeholder} color={seg.placeholder ? 'yellow' : 'cyan'}>
              {seg.text}
            </Text>
          ))}
        </Text>
      </Panel>

      <Panel title="specimens — cursor at each interesting position">
        {[
          ['start', 0],
          ['mid-word', 5],
          ['on a space', 7],
          ['on the {{', 8],
          ['in placeholder', 11],
          ['on last char', 32],
          ['end', -1],
        ].map(([label, at]) => {
          const s = 'git tag {{version}} -m "{{note}}"';
          const spans = templateSegments(s).map((x) => ({
            text: x.text,
            color: x.placeholder ? 'yellow' : 'cyan',
            bold: x.placeholder,
          }));
          const ln = toLines(spans)[0]!;
          const c = at === -1 ? s.length : (at as number);
          return (
            <Box key={label as string}>
              <Box width={16}>
                <Text dimColor>{label}</Text>
              </Box>
              <Text>{variant.line(ln.pieces, c, ln.end)}</Text>
            </Box>
          );
        })}
      </Panel>

      <Box marginTop={1} paddingX={1}>
        <Text>
          <Text bold color={ACCENT}>esc</Text>
          <Text dimColor> variant · </Text>
          <Text bold color={ACCENT}>tab</Text>
          <Text dimColor> field · </Text>
          <Text bold color={ACCENT}>←→</Text>
          <Text dimColor> char · </Text>
          <Text bold color={ACCENT}>opt+←→</Text>
          <Text dimColor> word · </Text>
          <Text bold color={ACCENT}>↑↓</Text>
          <Text dimColor> line/field · </Text>
          <Text bold color={ACCENT}>^C</Text>
          <Text dimColor> quit</Text>
        </Text>
      </Box>
      <Box paddingX={1}>
        <Text backgroundColor={ACCENT} color="black" bold>
          {' '}‹ {variant.key} — {variant.name} ›{' '}
        </Text>
        <Text dimColor>  {variant.blurb}</Text>
      </Box>
    </Box>
  );
}

export { App as PrototypeApp };

if (import.meta.main) {
  const { stdin, cleanup } = bunSafeStdin();
  const app = render(<App />, { stdin, exitOnCtrlC: true });
  await app.waitUntilExit();
  cleanup();
}
