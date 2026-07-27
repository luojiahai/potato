// PROTOTYPE — throwaway. Answers wayfinder ticket #46.
//
// visualRows(): one source of truth for how a value breaks into rows, per #41.
// Character-boundary wrapping, per #44 (Ink's word-wrap is deliberately not
// used — two wrappers fold in different places and the cursor trusts this one).
//
// DELIBERATELY NAIVE: chunks by JS code unit, so a wide CJK glyph counts as 1
// column and an emoji can split. Measuring a column is ticket #50's question,
// not this one's — the prototype must not pre-empt it.

export interface VisualRow {
  /** offset into `value` where this row starts */
  start: number;
  /** offset just past the row's last character (excludes the `\n`) */
  end: number;
  text: string;
}

/**
 * `caretRows: true` appends a trailing empty row for any logical line whose
 * length is an exact positive multiple of `width`. Without it the caret at the
 * end of a full row has no column to live in — it would render at column
 * `width`, one past the edge. Read-only panels (#44) pass false: they have no
 * caret and the extra blank row would just eat their budget.
 */
export function visualRows(
  value: string,
  width: number,
  opts: { caretRows?: boolean } = {},
): VisualRow[] {
  const w = Math.max(1, Math.floor(width));
  const rows: VisualRow[] = [];
  let lineStart = 0;

  for (const line of value.split('\n')) {
    if (line.length === 0) {
      rows.push({ start: lineStart, end: lineStart, text: '' });
    } else {
      for (let i = 0; i < line.length; i += w) {
        const chunk = line.slice(i, i + w);
        rows.push({ start: lineStart + i, end: lineStart + i + chunk.length, text: chunk });
      }
      if (opts.caretRows && line.length % w === 0) {
        rows.push({ start: lineStart + line.length, end: lineStart + line.length, text: '' });
      }
    }
    lineStart += line.length + 1; // + the '\n'
  }
  return rows;
}

/**
 * Which row the caret sits on. At a *soft* wrap boundary the offset belongs to
 * both rows; the caret takes the later one so typing stays visible. At a *hard*
 * newline the next row starts at `end + 1`, so the caret stays put.
 */
export function rowOfOffset(rows: VisualRow[], offset: number): number {
  for (let i = 0; i < rows.length; i++) {
    const r = rows[i]!;
    if (offset < r.end) return i;
    if (offset === r.end) {
      const next = rows[i + 1];
      return next && next.start === r.end ? i + 1 : i;
    }
  }
  return Math.max(0, rows.length - 1);
}

/** Keep `caretRow` inside a `budget`-tall window, moving `top` as little as possible. */
export function clampViewport(top: number, caretRow: number, budget: number, total: number): number {
  const maxTop = Math.max(0, total - budget);
  let t = Math.min(top, maxTop);
  if (caretRow < t) t = caretRow;
  if (caretRow >= t + budget) t = caretRow - budget + 1;
  return Math.max(0, Math.min(t, maxTop));
}
