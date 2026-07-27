// PROTOTYPE — throwaway. Answers wayfinder ticket #46.
//
// The sole home of every wire shape (#41). Predicate helpers, not a declarative
// table — and every gesture is an OR over each form the terminal can send it in,
// so no terminal detection is needed (the adopted hermes-agent mechanism).
//
// Returns null for "not a text gesture" — distinct from the reducer's 'pass',
// which means "a text gesture declining at a boundary".

import type { Gesture } from './text';

type InkKey = {
  ctrl: boolean; meta: boolean; shift: boolean;
  return: boolean; escape: boolean; tab: boolean;
  upArrow: boolean; downArrow: boolean; leftArrow: boolean; rightArrow: boolean;
  backspace: boolean; delete: boolean;
};

const isPrintable = (input: string, key: InkKey) =>
  input.length > 0 &&
  !key.ctrl && !key.meta && !key.return && !key.escape && !key.tab &&
  !key.upArrow && !key.downArrow && !key.leftArrow && !key.rightArrow &&
  !key.backspace && !key.delete &&
  // eslint-disable-next-line no-control-regex
  !/[\x00-\x1f\x7f]/.test(input);

// Terminals may deliver ctrl+letter as a raw control byte Ink doesn't decode.
const isCtrl = (input: string, key: InkKey, letter: string) =>
  (key.ctrl && input === letter) || input === String.fromCharCode(letter.charCodeAt(0) - 96);

export function toGesture(input: string, key: InkKey): Gesture | null {
  // A bare escape is the screen's (quit / back), never text.
  if (key.escape) return null;

  // Opt+←/→ arrive as readline `ESC b` / `ESC f` on all three macOS terminals at
  // default settings, and as `\x1b[1;9D` / `\x1b[1;9C` on stock iTerm2 (#35).
  // Both shapes, one branch — this is the OR that removes terminal detection.
  if ((key.leftArrow && key.meta) || (key.meta && input === 'b')) return { type: 'wordLeft' };
  if ((key.rightArrow && key.meta) || (key.meta && input === 'f')) return { type: 'wordRight' };

  if (key.leftArrow) return { type: 'left' };
  if (key.rightArrow) return { type: 'right' };
  if (key.upArrow) return { type: 'up' };
  if (key.downArrow) return { type: 'down' };

  // key.backspace and key.delete are mutually exclusive and both deliverable;
  // App.tsx's `input === '\x7f'` arm is dead code that conflated the two (#40).
  // Opt+Backspace is bound only in its deliverable shape: ^W was offered and
  // declined (#40), so on a stock Mac this gesture simply never fires.
  if (key.meta && key.backspace) return { type: 'wordBackspace' };
  if (key.backspace) return { type: 'backspace' };
  if (key.delete) return { type: 'forwardDelete' };

  // The three the Cmd gestures are aliased onto (#40). Editing screens only —
  // the reducer gates them on caps.lineEnds, so the list screen keeps ^A/^E.
  if (isCtrl(input, key, 'a')) return { type: 'lineStart' };
  if (isCtrl(input, key, 'e')) return { type: 'lineEnd' };
  if (isCtrl(input, key, 'u')) return { type: 'deleteToLineStart' };

  // ^J is already `input === '\n'`, and key.return is false for it (#39).
  if (input === '\n') return { type: 'newline' };
  if (key.return) return { type: 'enter' };

  if (isPrintable(input, key)) return { type: 'insert', text: input };
  return null;
}
