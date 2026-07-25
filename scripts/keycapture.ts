#!/usr/bin/env bun
// Keypress byte capture, for the open rows in docs/research/terminal-key-sequences.md
// (issue #20). Run it in Ghostty and again in Terminal.app:
//
//   bun scripts/keycapture.ts            # legacy encoding (what potato ships today)
//   bun scripts/keycapture.ts --kitty    # with the Kitty keyboard protocol enabled
//
// It reuses Ink 7's *real* decoder modules rather than reimplementing them, so the
// "useInput would receive" line is exactly what src/tui/App.tsx would see.
//
// Raw mode is enabled BEFORE the stream starts flowing and input is read via 'data',
// mirroring src/tui/stdin.ts — so this script is subject to the same Bun behaviour the
// TUI is.

// Ink's package.json "exports" only publishes the barrel, so these internals are
// reached by path. Pinned to Ink 7.1.1's build layout; if an Ink upgrade moves them,
// this script fails loudly at import rather than silently decoding differently.
// @ts-expect-error - no type declarations for the deep path
import { createInputParser } from '../node_modules/ink/build/input-parser.js';
// @ts-expect-error - ditto
import parseKeypress, { nonAlphanumericKeys } from '../node_modules/ink/build/parse-keypress.js';

const KITTY = process.argv.includes('--kitty');
// disambiguateEscapeCodes (1) | reportEventTypes (2). Flag 2 is what makes Ink's
// kittySpecialKeyRe match modified arrows and expose key.super / key.eventType.
const KITTY_FLAGS = 3;

const KEYS = [
  'Left',
  'Right',
  'Up',
  'Down',
  'opt+Left',
  'opt+Right',
  'opt+Up',
  'opt+Down',
  'cmd+Left',
  'cmd+Right',
  'cmd+Up',
  'cmd+Down',
  'Enter',
  'shift+Enter',
  'ctrl+J',
  'Backspace',
  'opt+Backspace',
  'cmd+Backspace',
  'fn+Left',
  'fn+Right',
  'Escape',
  'Escape (again)',
  'ctrl+A',
  'ctrl+E',
  'ctrl+U',
  'b',
  'opt+b',
];

const dim = (s: string) => `\x1b[2m${s}\x1b[0m`;
const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const warn = (s: string) => `\x1b[33m${s}\x1b[0m`;

const hex = (s: string) =>
  [...Buffer.from(s, 'utf8')].map((b) => b.toString(16).padStart(2, '0')).join(' ');

const escaped = (s: string) =>
  [...s]
    .map((c) => {
      const code = c.codePointAt(0)!;
      if (c === '\x1b') return '\\x1b';
      if (c === '\r') return '\\r';
      if (c === '\n') return '\\n';
      if (c === '\t') return '\\t';
      if (code < 0x20 || code === 0x7f) return `\\x${code.toString(16).padStart(2, '0')}`;
      return c;
    })
    .join('');

// Exactly the key object hooks/use-input.js builds (Ink 7.1.1), so the report matches
// what potato's useInput callback is handed.
function inkKeyAndInput(keypress: any): { input: string; key: Record<string, unknown> } {
  const key: Record<string, unknown> = {
    upArrow: keypress.name === 'up',
    downArrow: keypress.name === 'down',
    leftArrow: keypress.name === 'left',
    rightArrow: keypress.name === 'right',
    pageDown: keypress.name === 'pagedown',
    pageUp: keypress.name === 'pageup',
    home: keypress.name === 'home',
    end: keypress.name === 'end',
    return: keypress.name === 'return',
    escape: keypress.name === 'escape',
    ctrl: keypress.ctrl,
    shift: keypress.shift,
    tab: keypress.name === 'tab',
    backspace: keypress.name === 'backspace',
    delete: keypress.name === 'delete',
    meta: keypress.meta,
    super: keypress.super ?? false,
    hyper: keypress.hyper ?? false,
    capsLock: keypress.capsLock ?? false,
    numLock: keypress.numLock ?? false,
    eventType: keypress.eventType,
  };

  let input: string;
  if (keypress.isKittyProtocol) {
    if (keypress.isPrintable) input = keypress.text ?? keypress.name;
    else if (keypress.ctrl && keypress.name.length === 1) input = keypress.name;
    else input = '';
  } else if (keypress.ctrl) {
    input = keypress.name ?? '';
  } else {
    input = keypress.sequence;
  }
  if (!keypress.isKittyProtocol && nonAlphanumericKeys.includes(keypress.name)) input = '';
  if (input.startsWith('\u001B')) input = input.slice(1);
  if (input.length === 1 && /[A-Z]/.test(input)) key.shift = true;

  return { input, key };
}

// The three helpers from src/tui/App.tsx (kept in sync by hand — see App.tsx:162-174).
const isPrintable = (input: string, key: any) =>
  input.length > 0 &&
  !key.ctrl && !key.meta && !key.return && !key.escape && !key.tab &&
  !key.upArrow && !key.downArrow && !key.leftArrow && !key.rightArrow &&
  !key.backspace && !key.delete &&
  // eslint-disable-next-line no-control-regex
  !/[\x00-\x1f\x7f]/.test(input);

const isCtrl = (input: string, key: any, letter: string) =>
  (key.ctrl && input === letter) || input === String.fromCharCode(letter.charCodeAt(0) - 96);

const isBackspace = (input: string, key: any) =>
  key.backspace || key.delete || input === '\x7f';

// What potato's ListScreen would actually do with this event (App.tsx:341-358).
function potatoVerdict(input: string, key: any): string {
  const hits: string[] = [];
  if (key.escape) hits.push('ListScreen: QUIT');
  if (isCtrl(input, key, 'a')) hits.push("ListScreen: onAdd()  <- collision if this wasn't ctrl+A");
  if (isCtrl(input, key, 'e')) hits.push("ListScreen: onEdit()  <- collision if this wasn't ctrl+E");
  if (isCtrl(input, key, 'd')) hits.push('ListScreen: onDelete()');
  if (isCtrl(input, key, 'y')) hits.push('ListScreen: onCopy()');
  if (key.return) hits.push('ListScreen: onRun()');
  if (key.upArrow) hits.push('ListScreen: selection up');
  if (key.downArrow) hits.push('ListScreen: selection down');
  if (isBackspace(input, key)) hits.push('ListScreen: delete last query char');
  if (isPrintable(input, key)) hits.push(`ListScreen: types ${JSON.stringify(input)} into the query`);
  return hits.length > 0 ? hits.join('; ') : 'ignored (no handler matches)';
}

const truthy = (key: Record<string, unknown>) =>
  Object.entries(key)
    .filter(([, v]) => v === true || (typeof v === 'string' && v.length > 0))
    .map(([k, v]) => (v === true ? k : `${k}:${v}`))
    .join(', ') || '(none)';

// ---------------------------------------------------------------------------

if (!process.stdin.isTTY) {
  console.error('keycapture needs a terminal (stdin is not a TTY)');
  process.exit(1);
}

// Raw mode first, then read via 'data' — the src/tui/stdin.ts ordering.
process.stdin.setRawMode(true);
process.stdin.resume();

let cleanedUp = false;
const cleanup = () => {
  if (cleanedUp) return;
  cleanedUp = true;
  if (KITTY) process.stdout.write('\x1b[<u'); // pop the Kitty flags we pushed
  try {
    process.stdin.setRawMode(false);
  } catch {
    // stdin may already be closed
  }
  process.stdin.pause();
};
process.on('exit', cleanup);

// Probe for Kitty keyboard protocol support. A terminal that supports it answers
// `CSI ? flags u`; one that doesn't stays silent. Settles "does Terminal.app support
// the Kitty protocol?" without any guessing.
let probeAnswered = false;
process.stdout.write('\x1b[?u');

console.log(bold('\npotato keycapture') + dim(`  (issue #20)  kitty=${KITTY ? 'on' : 'off'}`));
console.log(
  dim(
    'For each keypress: raw bytes, the events Ink splits them into, Ink 7.1.1\'s decode,\n' +
      "and what potato's ListScreen would do. Press ctrl+C to exit.\n",
  ),
);
console.log(bold('Press these keys, in this order:'));
KEYS.forEach((k, i) => {
  const n = String(i + 1).padStart(2, ' ');
  process.stdout.write(dim(`  ${n}. ${k.padEnd(16)}`) + ((i + 1) % 4 === 0 ? '\n' : ''));
});
console.log('\n');
console.log(
  dim(
    'Rows 23-25 (ctrl+A / ctrl+E / ctrl+U) are the control: in Ghostty at defaults,\n' +
      'rows 9 / 10 / 18 (cmd+Left / cmd+Right / cmd+Backspace) must print IDENTICAL bytes.\n',
  ),
);

if (KITTY) {
  process.stdout.write(`\x1b[>${KITTY_FLAGS}u`);
  console.log(dim(`Pushed Kitty flags ${KITTY_FLAGS} (disambiguateEscapeCodes|reportEventTypes).\n`));
}

const parser = createInputParser();
let n = 0;
let flushTimer: ReturnType<typeof setTimeout> | undefined;

// Ink's own 20ms pending-escape flush (components/App.js:45). Reproduced so a lone
// Escape behaves here exactly as it does in the TUI, delay included.
const PENDING_FLUSH_MS = 20;

function report(event: string, note?: string) {
  n += 1;
  const label = KEYS[n - 1] ? dim(`  expected: ${KEYS[n - 1]}`) : '';
  const keypress = parseKeypress(event);
  const { input, key } = inkKeyAndInput(keypress);

  console.log(`${bold(`#${n}`)}${label}`);
  console.log(`  bytes    ${bold(hex(event))}   "${escaped(event)}"   (${event.length} chars)`);
  if (note) console.log(`  ${warn(note)}`);
  console.log(
    `  keypress name=${JSON.stringify(keypress.name)}` +
      ` ctrl=${keypress.ctrl} meta=${keypress.meta} shift=${keypress.shift}` +
      (keypress.isKittyProtocol ? ` ${warn('kitty=true')} super=${keypress.super}` : '') +
      (keypress.eventType ? ` eventType=${keypress.eventType}` : ''),
  );
  console.log(`  useInput input=${JSON.stringify(input)}  key: ${truthy(key)}`);
  console.log(`  potato   ${potatoVerdict(input, key)}`);
  console.log('');
}

process.stdin.on('data', (buf: Buffer) => {
  const chunk = buf.toString('utf8');

  // Swallow (and report) the CSI ? flags u answer to our probe.
  const probe = /\x1b\[\?(\d+)u/.exec(chunk);
  if (probe && !probeAnswered) {
    probeAnswered = true;
    console.log(
      warn(`  [probe] terminal supports the Kitty keyboard protocol; current flags = ${probe[1]}\n`),
    );
    if (chunk.replace(probe[0], '') === '') return;
  }

  if (flushTimer) {
    clearTimeout(flushTimer);
    flushTimer = undefined;
  }

  if (chunk === '\x03') {
    console.log(dim('\nctrl+C — done. Paste this transcript into issue #20.\n'));
    cleanup();
    process.exit(0);
  }

  const rawChunk = hex(chunk);
  const events: Array<string | { paste: string }> = parser.push(chunk);

  if (events.length === 0) {
    console.log(dim(`  ...held ${rawChunk} as an incomplete escape sequence (waiting ${PENDING_FLUSH_MS}ms)`));
  } else if (events.length > 1) {
    console.log(
      warn(`  [one read of ${rawChunk} split into ${events.length} events]`),
    );
  }

  for (const event of events) {
    if (typeof event === 'string') report(event);
    else console.log(`${bold('#paste')}  ${JSON.stringify(event.paste)}\n`);
  }

  // Ink arms this whenever a bare ESC is left pending; the flush is what makes a lone
  // Escape arrive ~20ms late, and what makes a slowly-split sequence misfire.
  if (parser.hasPendingEscape()) {
    flushTimer = setTimeout(() => {
      flushTimer = undefined;
      const pending = parser.flushPendingEscape();
      if (pending) {
        report(pending, `flushed after ${PENDING_FLUSH_MS}ms as an incomplete sequence (Ink's ESC timing hack)`);
      }
    }, PENDING_FLUSH_MS);
  }
});
