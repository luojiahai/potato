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
export const FRAME_COLOR = '#d78700';

/** potato's round-border panel, title overlaid on the top border. */
export function Panel(props: {
  title?: string; titleColor?: string; borderColor?: string; children: React.ReactNode;
}) {
  return (
    <Box
      borderStyle="round"
      borderColor={props.borderColor ?? FRAME_COLOR}
      flexDirection="column"
      paddingX={1}
    >
      {props.title !== undefined && (
        <Box marginTop={-1}>
          <Text bold color={props.titleColor} wrap="truncate"> {props.title} </Text>
        </Box>
      )}
      {props.children}
    </Box>
  );
}

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
  // `budget` counts value rows only; the label row is accounted for by extraRows
  const w = windowRows(props.value, width, props.cursor, props.focused, props.budget);

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

// ---------- Variant D — stacked key/value, every field ----------

export interface StackProps {
  fields: { name: string; command: string; description: string };
  /** 0 name, 1 description, 2 command — command is last so it grows into free space */
  focus: number;
  cursor: number;
  innerWidth: number;
  /** visual rows the *command value* may occupy; name and description take what they need */
  budget: number;
  caretStyle: CaretStyle;
  softMarks: boolean;
  /** a blank row between field groups — readability against two extra rows of chrome */
  separators: boolean;
}

/** One label row + as many value rows as the value needs, indented two columns. */
function StackedField(props: {
  label: string; value: string; cursor: number; focused: boolean;
  width: number; budget: number; caretStyle: CaretStyle; softMarks: boolean;
  hint?: string;
}) {
  const marks = placeholderMarks(props.value);
  const w = windowRows(props.value, props.width, props.cursor, props.focused, props.budget);

  return (
    <Box flexDirection="column">
      <Text bold={props.focused} color={props.focused ? 'cyan' : undefined} dimColor={!props.focused}>
        {props.focused ? '❯ ' : '  '}{props.label}
        {props.hint && <Text dimColor>  {props.hint}</Text>}
      </Text>
      <Box>
        <Box width={2} flexDirection="column">
          {w.above > 0 && <Text> </Text>}
          {w.rows.map((r, i) => (
            <Text key={i} dimColor>{props.softMarks && r.soft ? '↳ ' : '  '}</Text>
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

/**
 * No `$ ` gutter on the editable rows, unlike B — the `$` means "this is what
 * the shell will see", which is the *template* panel's job. The field is where
 * you type, not a preview of a prompt.
 */
export function VariantD(props: StackProps) {
  const width = Math.max(1, props.innerWidth - 2);
  const common = {
    width,
    caretStyle: props.caretStyle,
    softMarks: props.softMarks,
    cursor: props.cursor,
  };
  const gap = props.separators ? 1 : 0;

  return (
    <Box flexDirection="column">
      <StackedField
        {...common} label="name" value={props.fields.name}
        focused={props.focus === 0} budget={rowsNeeded(props.fields.name, width)}
      />
      <Box marginTop={gap}>
        <StackedField
          {...common} label="description" value={props.fields.description}
          focused={props.focus === 1} budget={rowsNeeded(props.fields.description, width)}
          hint="(optional)"
        />
      </Box>
      {/* command last: the one field that grows, so it expands into free space
          instead of pushing name and description around as you type */}
      <Box marginTop={gap}>
        <StackedField
          {...common} label="command" value={props.fields.command}
          focused={props.focus === 2} budget={props.budget}
        />
      </Box>
    </Box>
  );
}
VariantD.title = 'D — stacked key/value, every field';
VariantD.ownsStack = true as const;
VariantD.valueWidth = (inner: number) => inner - 2;
VariantD.fixedRows = 0; // grows
VariantD.extraRows = 0; // accounted for by the caller, which knows all three values

// ---------- Variant E — one frame per field ----------

/**
 * Each field is its own titled panel, in the framed-panel language the rest of
 * potato already speaks. The title rides the top border, so the label costs no
 * content row; focus moves to the border colour, so `❯` isn't needed either.
 * Price: three borders instead of one — six rows of chrome before any value.
 */
export function VariantE(props: StackProps) {
  const one = (
    label: string, value: string, focused: boolean, budget: number, gutter: boolean, hint?: string,
  ) => {
    const width = Math.max(1, props.innerWidth - (gutter ? 2 : 0));
    const marks = placeholderMarks(value);
    const w = windowRows(value, width, props.cursor, focused, budget);
    return (
      <Panel
        title={hint ? `${label} ${hint}` : label}
        titleColor={focused ? 'cyan' : undefined}
        borderColor={focused ? 'cyan' : FRAME_COLOR}
      >
        <Box>
          {gutter && (
            <Box width={2} flexDirection="column">
              {w.above > 0 && <Text> </Text>}
              {w.rows.map((r, i) => (
                <Text key={i} dimColor>{props.softMarks && r.soft ? '↳ ' : '  '}</Text>
              ))}
              {w.below > 0 && <Text> </Text>}
            </Box>
          )}
          <Box flexDirection="column">
            {w.above > 0 && <Indicator text={`⋮ +${w.above} above`} />}
            {w.rows.map((r, i) => (
              <Box key={w.top + i}>
                {renderRow(r.text, r.start, marks, focused ? props.cursor : null, props.caretStyle)}
              </Box>
            ))}
            {w.below > 0 && <Indicator text={`⋮ +${w.below} below`} />}
          </Box>
        </Box>
      </Panel>
    );
  };

  const full = Math.max(1, props.innerWidth);
  return (
    <Box flexDirection="column">
      {one('name', props.fields.name, props.focus === 0, rowsNeeded(props.fields.name, full), false)}
      {one('description', props.fields.description, props.focus === 1,
        rowsNeeded(props.fields.description, full), false, '(optional)')}
      {/* command last: the only field that grows, so it expands into free space.
          The gutter costs two columns and only earns them when it carries the
          soft-wrap marks — otherwise command would sit indented from the other
          two panels for no visible reason. */}
      {one('command', props.fields.command, props.focus === 2, props.budget, props.softMarks)}
    </Box>
  );
}
VariantE.title = 'E — one frame per field';
VariantE.valueWidth = (inner: number) => inner - 2; // the command panel's gutter
VariantE.fixedRows = 0; // grows

export const VARIANTS = [VariantA, VariantB, VariantC];

// ---------- read-only renderer for the template panel (#44) ----------

export function CommandBlock({
  value, width, maxRows,
}: { value: string; width: number; maxRows: number }) {
  const marks = placeholderMarks(value);
  const rows = visualRows(value, Math.max(1, width - 2), { caretRows: false });
  // the `… +N more lines` indicator costs a row, so it has to come out of
  // maxRows — `Math.max(1, …)` here would render maxRows+1 rows at maxRows=1
  // and punch the panel's bottom border.
  const capped = rows.length > maxRows;
  const shown = capped ? rows.slice(0, Math.max(0, maxRows - 1)) : rows;

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
