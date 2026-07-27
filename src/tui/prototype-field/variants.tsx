// PROTOTYPE — throwaway. Answers wayfinder ticket #46.
//
// Three structurally different command fields. They disagree about geometry,
// not colour:
//
//   A  in-column growth  — today's 14-column label layout, field grows downward,
//                          continuation rows indented under the value column
//   B  full-width block  — label breaks onto its own row, body is a #44-style
//                          two-column gutter block at full panel width
//   C  one-row viewport  — the field never grows; horizontal scroll follows the
//                          caret, ↑/↓ page through logical lines
//
// Caret style is orthogonal and toggles independently (^T), so the overlay
// cursor and the between-characters bar can be judged under any of the three.

import React from 'react';
import { Box, Text } from 'ink';
import { templateSegments } from '../../placeholders';
import { clampViewport, rowOfOffset, visualRows } from './wrap';

export const LABEL_WIDTH = 14;

/**
 * 'overlay' — a solid cell *on* the character, the way a terminal's own block
 *   cursor works. Occupies no column of its own, so nothing shifts and the
 *   wrap point never moves as you navigate.
 * 'bar' — the `▌` *between* characters. Kept only for comparison: it costs a
 *   real column, so text slides right under the caret.
 */
export type CaretStyle = 'overlay' | 'bar';

const CARET_BG = 'cyan';

/**
 * Fixed colour regardless of what sits under it — a real cursor doesn't change
 * colour over a placeholder. `inverse` would, since it just swaps the run's
 * own foreground.
 */
const caretCell = (ch: string, key: number) => (
  <Text key={key} backgroundColor={CARET_BG} color="black">{ch}</Text>
);

export interface FieldViewProps {
  label: string;
  value: string;
  cursor: number;
  focused: boolean;
  /** columns inside the panel, i.e. after its border and padding */
  innerWidth: number;
  /** visual rows this field may occupy, indicators included */
  budget: number;
  caretStyle: CaretStyle;
  /** show a `↳` gutter marker on soft-wrapped rows, so they read differently from a real newline */
  softMarks: boolean;
}

// ---------- shared: placeholder highlighting composed by offset (#44) ----------

/** 1 wherever the offset falls inside a `{{...}}` token — spans line breaks (#51). */
function placeholderMarks(value: string): Uint8Array {
  const marks = new Uint8Array(value.length);
  let off = 0;
  for (const seg of templateSegments(value)) {
    if (seg.placeholder) marks.fill(1, off, off + seg.text.length);
    off += seg.text.length;
  }
  return marks;
}

function renderRow(
  text: string,
  start: number,
  marks: Uint8Array,
  caret: number | null,
  style: CaretStyle,
): React.ReactNode {
  const out: React.ReactNode[] = [];
  let run = '';
  let runPh = false;
  let key = 0;
  const flush = () => {
    if (run !== '') {
      out.push(
        <Text key={key++} bold={runPh} color={runPh ? 'yellow' : 'cyan'}>{run}</Text>,
      );
      run = '';
    }
  };

  for (let i = 0; i < text.length; i++) {
    const off = start + i;
    const ph = marks[off] === 1;
    if (caret === off) {
      flush();
      if (style === 'bar') {
        out.push(<Text key={key++} color="cyan">▌</Text>);
      } else {
        out.push(caretCell(text[i]!, key++));
        continue;
      }
    }
    if (run !== '' && ph !== runPh) flush();
    runPh = ph;
    run += text[i];
  }
  flush();

  // caret parked past the last character of this row
  if (caret !== null && caret === start + text.length) {
    out.push(
      style === 'bar'
        ? <Text key={key++} color="cyan">▌</Text>
        : caretCell(' ', key++),
    );
  }
  return <Text>{out.length ? out : ' '}</Text>;
}

/** Vertical window over the wrapped rows, with dim indicators eating from the budget. */
function windowRows(value: string, width: number, cursor: number, focused: boolean, budget: number) {
  const all = visualRows(value, width, { caretRows: true });
  // a row is a *soft* continuation when no '\n' separates it from the one above
  const rows = all.map((r, i) => ({ ...r, soft: i > 0 && all[i - 1]!.end === r.start }));
  const caretRow = focused ? rowOfOffset(rows, cursor) : 0;
  if (rows.length <= budget) return { rows, top: 0, above: 0, below: 0, shown: rows.length };

  // reserve one line per active indicator, then re-clamp against what's left
  let content = budget;
  let top = clampViewport(0, caretRow, content, rows.length);
  for (let pass = 0; pass < 3; pass++) {
    const needAbove = top > 0 ? 1 : 0;
    const needBelow = top + content < rows.length ? 1 : 0;
    const next = Math.max(1, budget - needAbove - needBelow);
    if (next === content) break;
    content = next;
    top = clampViewport(top, caretRow, content, rows.length);
  }
  return {
    rows: rows.slice(top, top + content),
    top,
    above: top,
    below: Math.max(0, rows.length - (top + content)),
    shown: content,
  };
}

/** How many rows this field would like, before any budget is applied. */
export function rowsNeeded(value: string, width: number): number {
  return visualRows(value, width, { caretRows: true }).length;
}

const Indicator = ({ text }: { text: string }) => <Text dimColor>{text}</Text>;

// ---------- Variant A — in-column growth ----------

export function VariantA(props: FieldViewProps) {
  const width = Math.max(1, props.innerWidth - LABEL_WIDTH);
  const marks = placeholderMarks(props.value);
  const w = windowRows(props.value, width, props.cursor, props.focused, props.budget);
  // one label-column entry per value-column child, so the two stay in step
  const slots: Array<{ soft: boolean } | null> = [
    ...(w.above > 0 ? [null] : []),
    ...w.rows.map((r) => ({ soft: r.soft })),
    ...(w.below > 0 ? [null] : []),
  ];

  return (
    <Box>
      <Box width={LABEL_WIDTH} flexDirection="column">
        {slots.map((s, i) =>
          i === 0 ? (
            <Text key={i} bold color={props.focused ? 'cyan' : undefined}>
              {props.focused ? '❯ ' : '  '}{props.label}
            </Text>
          ) : (
            <Text key={i} dimColor>{props.softMarks && s?.soft ? '           ↳' : ' '}</Text>
          ),
        )}
      </Box>
      <Box flexDirection="column">
        {w.above > 0 && <Indicator text={`⋮ +${w.above} above`} />}
        {w.rows.map((r, i) => (
          <Box key={w.top + i}>
            {renderRow(r.text, r.start, marks, props.focused ? props.cursor : null, props.caretStyle)}
          </Box>
        ))}
        {w.below > 0 && <Indicator text={`⋮ +${w.below} below`} />}
      </Box>
    </Box>
  );
}
VariantA.title = 'A — in-column growth';
VariantA.extraRows = 0;
VariantA.valueWidth = (inner: number) => inner - LABEL_WIDTH;
VariantA.fixedRows = 0; // grows

// ---------- Variant B — full-width gutter block ----------

export function VariantB(props: FieldViewProps) {
  const width = Math.max(1, props.innerWidth - 2);
  const marks = placeholderMarks(props.value);
  const w = windowRows(props.value, width, props.cursor, props.focused, Math.max(1, props.budget - 1));

  return (
    <Box flexDirection="column">
      <Text bold color={props.focused ? 'cyan' : undefined}>
        {props.focused ? '❯ ' : '  '}{props.label}
      </Text>
      <Box>
        {/* two-column gutter, so hard newlines and soft wraps align (#44) */}
        <Box width={2} flexDirection="column">
          {w.above > 0 && <Text> </Text>}
          {w.rows.map((r, i) => (
            <Text key={i} dimColor>
              {w.top + i === 0 ? '$ ' : props.softMarks && r.soft ? '↳ ' : '  '}
            </Text>
          ))}
          {w.below > 0 && <Text> </Text>}
        </Box>
        <Box flexDirection="column">
          {w.above > 0 && <Indicator text={`⋮ +${w.above} above`} />}
          {w.rows.map((r, i) => (
            <Box key={w.top + i}>
              {renderRow(r.text, r.start, marks, props.focused ? props.cursor : null, props.caretStyle)}
            </Box>
          ))}
          {w.below > 0 && <Indicator text={`⋮ +${w.below} below`} />}
        </Box>
      </Box>
    </Box>
  );
}
VariantB.title = 'B — full-width gutter block';
VariantB.extraRows = 1; // the label gets its own row
VariantB.valueWidth = (inner: number) => inner - 2;
VariantB.fixedRows = 0; // grows

// ---------- Variant C — one-row viewport, horizontal scroll ----------

export function VariantC(props: FieldViewProps) {
  const marks = placeholderMarks(props.value);
  const lines = props.value.split('\n');

  // which logical line the caret is on, and where in it
  let lineIdx = 0;
  let lineStart = 0;
  for (let i = 0; i < lines.length; i++) {
    const end = lineStart + lines[i]!.length;
    if (props.cursor <= end) { lineIdx = i; break; }
    lineStart = end + 1;
    lineIdx = i + 1;
  }
  if (lineIdx >= lines.length) { lineIdx = lines.length - 1; lineStart = props.value.length - lines[lineIdx]!.length; }

  const line = lines[lineIdx]!;
  const col = props.cursor - lineStart;
  const badge = lines.length > 1 ? ` ${lineIdx + 1}/${lines.length}` : '';
  const width = Math.max(4, props.innerWidth - LABEL_WIDTH - badge.length - 2);

  // scroll only as far as the caret forces
  const start = col < width ? 0 : col - width + 1;
  const slice = line.slice(start, start + width);
  const more = start + width < line.length;

  return (
    <Box>
      <Box width={LABEL_WIDTH}>
        <Text bold color={props.focused ? 'cyan' : undefined}>
          {props.focused ? '❯ ' : '  '}{props.label}
        </Text>
      </Box>
      <Text dimColor>{start > 0 ? '‹' : ' '}</Text>
      {renderRow(slice, lineStart + start, marks, props.focused ? props.cursor : null, props.caretStyle)}
      <Text dimColor>{more ? '›' : ' '}</Text>
      {badge && <Text dimColor>{badge}</Text>}
    </Box>
  );
}
VariantC.title = 'C — one-row viewport (h-scroll)';
VariantC.extraRows = 0;
VariantC.valueWidth = (inner: number) => inner - LABEL_WIDTH;
VariantC.fixedRows = 1; // never grows, whatever the value

export const VARIANTS = [VariantA, VariantB, VariantC];

// ---------- read-only renderer for the template panel (#44) ----------

export function CommandBlock({
  value, width, maxRows,
}: { value: string; width: number; maxRows: number }) {
  const marks = placeholderMarks(value);
  const rows = visualRows(value, Math.max(1, width - 2), { caretRows: false });
  const capped = rows.length > maxRows;
  const shown = capped ? rows.slice(0, Math.max(1, maxRows - 1)) : rows;

  return (
    <Box>
      <Box width={2} flexDirection="column">
        {shown.map((_, i) => <Text key={i} dimColor>{i === 0 ? '$ ' : '  '}</Text>)}
        {capped && <Text> </Text>}
      </Box>
      <Box flexDirection="column">
        {shown.map((r, i) => (
          <Box key={i}>{renderRow(r.text, r.start, marks, null, 'overlay')}</Box>
        ))}
        {capped && <Indicator text={`… +${rows.length - shown.length} more lines`} />}
      </Box>
    </Box>
  );
}
