// PROTOTYPE — throwaway. Answers wayfinder ticket #46.
//
// The pure reducer of #41: apply(state, gesture, caps) -> TextState | 'pass'.
// No Ink types, no I/O. 'pass' means "a text gesture declining at a boundary"
// and falls through to the screen's existing chain; `toGesture` returning null
// means "not a text gesture at all".
//
// Encodes the rulings of #39 (Enter/backslash, ^J, who owns up/down),
// #40 (eight gestures, logical lines, non-whitespace words, mirrored wordRight)
// and #42 (paste is a channel, filtered at the door).

import { rowOfOffset, visualRows } from './wrap';

export interface TextState {
  value: string;
  cursor: number;
}

export interface Caps {
  /** command field only. Gates exactly: Enter-continuation, ^J, and ↑/↓ traversal. */
  multiline: boolean;
  /** editing screens only — in the list screen ^A/^E/^D stay actions (#40). */
  lineEnds: boolean;
  /** columns available to the value, for visual ↑/↓ */
  width: number;
}

export type Gesture =
  | { type: 'insert'; text: string }
  | { type: 'paste'; text: string }
  | { type: 'left' }
  | { type: 'right' }
  | { type: 'wordLeft' }
  | { type: 'wordRight' }
  | { type: 'up' }
  | { type: 'down' }
  | { type: 'lineStart' }
  | { type: 'lineEnd' }
  | { type: 'backspace' }
  | { type: 'wordBackspace' }
  | { type: 'forwardDelete' }
  | { type: 'deleteToLineStart' }
  | { type: 'enter' }
  | { type: 'newline' };

export type Result = (TextState & { notice?: string }) | 'pass';

// ---------- grapheme-snapped offsets (#41) ----------

const SEGMENTER = new Intl.Segmenter(undefined, { granularity: 'grapheme' });

function boundaries(value: string): number[] {
  const out = [0];
  for (const s of SEGMENTER.segment(value)) out.push(s.index + s.segment.length);
  return out;
}

function prevOffset(value: string, offset: number): number {
  const b = boundaries(value);
  for (let i = b.length - 1; i >= 0; i--) if (b[i]! < offset) return b[i]!;
  return 0;
}

function nextOffset(value: string, offset: number): number {
  for (const o of boundaries(value)) if (o > offset) return o;
  return value.length;
}

// ---------- words: a run of non-whitespace (#40) ----------

const isSpace = (ch: string | undefined) => ch === undefined || /\s/.test(ch);

function wordLeft(value: string, cursor: number): number {
  let i = cursor;
  while (i > 0 && isSpace(value[i - 1])) i--;
  while (i > 0 && !isSpace(value[i - 1])) i--;
  return i;
}

/** Mirrors wordLeft, landing at the word *end* so motion round-trips (#40). */
function wordRight(value: string, cursor: number): number {
  let i = cursor;
  while (i < value.length && isSpace(value[i])) i++;
  while (i < value.length && !isSpace(value[i])) i++;
  return i;
}

// ---------- logical lines: what ^A / ^E / ^U mean (#40) ----------

function lineStartOf(value: string, cursor: number): number {
  const i = value.lastIndexOf('\n', cursor - 1);
  return i === -1 ? 0 : i + 1;
}

function lineEndOf(value: string, cursor: number): number {
  const i = value.indexOf('\n', cursor);
  return i === -1 ? value.length : i;
}

// ---------- the paste door (#42) ----------

/** `\r\n`/`\r` -> `\n`; literal tab survives; every other C0 + DEL is dropped. */
export function filterPaste(text: string, multiline: boolean): { text: string; notice?: string } {
  const normalised = text.replace(/\r\n?/g, '\n');
  // eslint-disable-next-line no-control-regex
  const stripped = normalised.replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g, '');
  const dropped = stripped.length !== normalised.length;
  // single-line fields flatten rather than refuse, closing #41's pasted-\n-into-name hole
  const out = multiline ? stripped : stripped.replace(/\n/g, ' ');
  const flattened = !multiline && stripped.includes('\n');
  const notice = dropped
    ? 'pasted — control characters removed'
    : flattened
      ? 'pasted — newlines flattened to spaces'
      : undefined;
  return notice ? { text: out, notice } : { text: out };
}

// ---------- the reducer ----------

const splice = (value: string, at: number, remove: number, insert: string) =>
  value.slice(0, at) + insert + value.slice(at + remove);

export function apply(state: TextState, gesture: Gesture, caps: Caps): Result {
  const { value, cursor } = state;

  switch (gesture.type) {
    case 'insert':
      return { value: splice(value, cursor, 0, gesture.text), cursor: cursor + gesture.text.length };

    case 'paste': {
      const { text, notice } = filterPaste(gesture.text, caps.multiline);
      if (text === '') return notice ? { ...state, notice } : 'pass';
      const next = { value: splice(value, cursor, 0, text), cursor: cursor + text.length };
      return notice ? { ...next, notice } : next;
    }

    case 'left':
      return cursor === 0 ? 'pass' : { value, cursor: prevOffset(value, cursor) };

    case 'right':
      return cursor >= value.length ? 'pass' : { value, cursor: nextOffset(value, cursor) };

    case 'wordLeft':
      return cursor === 0 ? 'pass' : { value, cursor: wordLeft(value, cursor) };

    case 'wordRight':
      return cursor >= value.length ? 'pass' : { value, cursor: wordRight(value, cursor) };

    // ↑/↓ belong to the cursor and escape to the neighbouring field at the
    // first/last *visual* row (#39). With multiline off there is one row, so
    // they always pass and today's field-nav is untouched.
    case 'up':
    case 'down': {
      if (!caps.multiline) return 'pass';
      const rows = visualRows(value, caps.width, { caretRows: true });
      const at = rowOfOffset(rows, cursor);
      const target = gesture.type === 'up' ? at - 1 : at + 1;
      if (target < 0 || target >= rows.length) return 'pass';
      const column = cursor - rows[at]!.start;
      const r = rows[target]!;
      return { value, cursor: Math.min(r.start + column, r.end) };
    }

    case 'lineStart':
      if (!caps.lineEnds) return 'pass';
      return { value, cursor: lineStartOf(value, cursor) };

    case 'lineEnd':
      if (!caps.lineEnds) return 'pass';
      return { value, cursor: lineEndOf(value, cursor) };

    case 'deleteToLineStart': {
      if (!caps.lineEnds) return 'pass';
      const start = lineStartOf(value, cursor);
      if (start === cursor) return 'pass';
      return { value: splice(value, start, cursor - start, ''), cursor: start };
    }

    // '\n' is an ordinary character (#40): lines join with no special case, and
    // a Continuation backslash needs no logic in the delete path.
    case 'backspace': {
      if (cursor === 0) return 'pass';
      const from = prevOffset(value, cursor);
      return { value: splice(value, from, cursor - from, ''), cursor: from };
    }

    case 'wordBackspace': {
      if (cursor === 0) return 'pass';
      const from = wordLeft(value, cursor);
      return { value: splice(value, from, cursor - from, ''), cursor: from };
    }

    case 'forwardDelete': {
      if (cursor >= value.length) return 'pass';
      const to = nextOffset(value, cursor);
      return { value: splice(value, cursor, to - cursor, ''), cursor };
    }

    // Enter makes a newline iff the character immediately left of the cursor is
    // a backslash (#39) — a *typing* rule. The backslash is kept verbatim.
    case 'enter':
      if (!caps.multiline || value[cursor - 1] !== '\\') return 'pass';
      return { value: splice(value, cursor, 0, '\n'), cursor: cursor + 1 };

    // ^J: unconditional newline, no Continuation required (#39).
    case 'newline':
      if (!caps.multiline) return 'pass';
      return { value: splice(value, cursor, 0, '\n'), cursor: cursor + 1 };
  }
}
