// PROTOTYPE — throwaway. Answers wayfinder ticket #46:
// "What does the command field look and feel like with a real cursor, wrapped
//  lines, and continuations?"
//
// Sub-shape A: the variants are mounted inside the *real* edit screen — real
// panels, real name/description fields, real template panel, real footer — so
// each one is judged against the chrome it has to live with, not in a vacuum.
//
// Nothing here persists. ^C to quit.

import React, { useState } from 'react';
import { Box, Text, useApp, useInput, usePaste, useStdout } from 'ink';
import { parsePlaceholders } from '../../placeholders';
import { apply, type Caps, type TextState } from './text';
import { toGesture } from './keys';
import {
  CommandBlock, LABEL_WIDTH, rowsNeeded, VARIANTS, type CaretStyle,
} from './variants';

const FRAME_COLOR = '#d78700';
const ACCENT_COLOR = '#ffaf5f';

// ---------- real chrome, copied from App.tsx (throwaway: no sharing) ----------

function Panel(props: { title?: string; children: React.ReactNode }) {
  return (
    <Box borderStyle="round" borderColor={FRAME_COLOR} flexDirection="column" paddingX={1}>
      {props.title !== undefined && (
        <Box marginTop={-1}>
          <Text bold wrap="truncate"> {props.title} </Text>
        </Box>
      )}
      {props.children}
    </Box>
  );
}

function Footer({ keys, flash }: { keys: Array<[string, string]>; flash: string | null }) {
  return (
    <Box marginTop={1} paddingX={1}>
      {flash ? (
        <Text backgroundColor="yellow" color="black"> {flash} </Text>
      ) : (
        keys.map(([k, label], i) => (
          <Text key={k}>
            {i > 0 && <Text dimColor> · </Text>}
            <Text bold color={ACCENT_COLOR}>{k}</Text>
            <Text dimColor> {label}</Text>
          </Text>
        ))
      )}
    </Box>
  );
}

/** A plain single-line field for name/description — same reducer, multiline off. */
function PlainField(props: {
  label: string; value: string; cursor: number; focused: boolean; caretStyle: CaretStyle; hint?: string;
}) {
  const before = props.value.slice(0, props.cursor);
  const at = props.value.slice(props.cursor, props.cursor + 1);
  const after = props.value.slice(props.cursor + 1);
  return (
    <Box>
      <Box width={LABEL_WIDTH}>
        <Text bold color={props.focused ? 'cyan' : undefined}>
          {props.focused ? '❯ ' : '  '}{props.label}
        </Text>
      </Box>
      <Text>
        {props.focused ? (
          props.caretStyle === 'bar' ? (
            <>
              {before}<Text color="cyan">▌</Text>{at}{after}
            </>
          ) : (
            <>
              {before}
              <Text backgroundColor="cyan" color="black">{at || ' '}</Text>
              {after}
            </>
          )
        ) : (
          props.value
        )}
        {props.hint && <Text dimColor>  {props.hint}</Text>}
      </Text>
    </Box>
  );
}

// ---------- samples ----------

const SAMPLES: Array<{ label: string; value: string }> = [
  {
    label: 'short + placeholders',
    value: 'docker run --rm -it -v {{dir=$PWD}}:/work {{image}} bash',
  },
  {
    label: 'long one-liner, no newlines',
    value:
      'ffmpeg -hide_banner -i {{input}} -vf "scale=1280:-2,unsharp=5:5:0.8" -c:v libx264 ' +
      '-preset slow -crf {{crf=23}} -c:a aac -b:a 192k -movflags +faststart {{output}}',
  },
  {
    label: 'continuations',
    value: 'ffmpeg -i {{input}} \\\n  -vf "scale=1280:-2" \\\n  -c:v libx264 -crf {{crf=23}} \\\n  {{output}}',
  },
  {
    label: 'placeholder across a break (#51)',
    value: "curl -sS -X POST {{url}} \\\n  -H 'content-type: application/json' \\\n  -d '{{body=first\nsecond}}'",
  },
  {
    label: 'tall — 12 lines',
    value: Array.from({ length: 12 }, (_, i) => `  --flag-${i + 1} {{v${i + 1}=value-${i + 1}}} \\`)
      .join('\n')
      .replace(/^ {2}/, 'deploy '),
  },
  { label: 'empty', value: '' },
];

const isCtrl = (input: string, key: { ctrl: boolean }, letter: string) =>
  (key.ctrl && input === letter) || input === String.fromCharCode(letter.charCodeAt(0) - 96);

const ORDER = ['name', 'command', 'description'] as const;
type FieldName = (typeof ORDER)[number];

// ---------- the prototype app ----------

export function Proto() {
  const { exit } = useApp();
  const { stdout } = useStdout();

  const [sample, setSample] = useState(2); // start on the continuations case
  const [fields, setFields] = useState<Record<FieldName, string>>({
    name: 'transcode',
    command: SAMPLES[2]!.value,
    description: 'shrink a video for the web',
  });
  const [focus, setFocus] = useState<number>(1); // start on the command field
  const [cursor, setCursor] = useState(SAMPLES[2]!.value.length);
  const [variant, setVariant] = useState(0);
  const [caretStyle, setCaretStyle] = useState<CaretStyle>('overlay');
  const [softMarks, setSoftMarks] = useState(false);
  const [flash, setFlash] = useState<string | null>(null);
  const [armed, setArmed] = useState(false); // escape-confirm-when-dirty (#39)
  const [dirty, setDirty] = useState(false);

  const rows = stdout?.rows ?? 24;
  const columns = stdout?.columns ?? 80;
  const innerWidth = Math.max(20, columns - 6); // root paddingX + panel border + panel paddingX

  const Variant = VARIANTS[variant]!;
  const field = ORDER[focus]!;
  const multiline = field === 'command';
  const valueWidth = Math.max(1, Variant.valueWidth(innerWidth));

  const placeholders = parsePlaceholders(fields.command);
  const argRows = placeholders.length > 0 ? 1 + placeholders.length : 0;

  // edit panel (2 border + name + description) + template panel (2 border)
  // + footer (2) + switcher (2) = 10, plus the args list
  const free = Math.max(2, rows - 10 - argRows);
  const needed = rowsNeeded(fields.command, valueWidth) + Variant.extraRows;
  const fieldBudget = Variant.fixedRows || Math.max(1, Math.min(needed, free));
  // #44: the field wins the budget; the template panel's body shrinks to zero
  const templateBudget = Math.max(0, free - fieldBudget);

  const caps: Caps = { multiline, lineEnds: true, width: Math.max(1, valueWidth) };

  const setFocusTo = (next: number) => {
    const n = (next + ORDER.length) % ORDER.length;
    setFocus(n);
    setCursor(fields[ORDER[n]!].length); // reset to end on focus change (#41)
  };

  const commit = (next: TextState & { notice?: string }) => {
    if (next.value !== fields[field]) setDirty(true);
    setFields((f) => ({ ...f, [field]: next.value }));
    setCursor(next.cursor);
    if (next.notice) setFlash(next.notice);
  };

  const loadSample = (i: number) => {
    const s = SAMPLES[i]!;
    setSample(i);
    setFields((f) => ({ ...f, command: s.value }));
    if (field === 'command') setCursor(s.value.length);
    setFlash(`sample: ${s.label}`);
    setDirty(false);
  };

  // Ink 7.1.1 hands the handler a bare string — not the `{paste}` event object
  // #42's write-up assumed. Same channel, one less indirection.
  usePaste((text) => {
    const r = apply({ value: fields[field], cursor }, { type: 'paste', text }, caps);
    if (r !== 'pass') commit(r);
  });

  useInput((input, key) => {
    // ---- prototype harness keys, checked first ----
    if (isCtrl(input, key, 'v')) return setVariant((v) => (v + 1) % VARIANTS.length);
    if (isCtrl(input, key, 't')) return setCaretStyle((s) => (s === 'overlay' ? 'bar' : 'overlay'));
    if (isCtrl(input, key, 'l')) return loadSample((sample + 1) % SAMPLES.length);
    if (isCtrl(input, key, 'r')) return setSoftMarks((s) => !s);

    if (key.escape) {
      // #39: confirm when dirty, closing the data-loss path from Ink's 20 ms flush
      if (dirty && !armed) { setArmed(true); return setFlash('unsaved — esc again to discard'); }
      return exit();
    }
    setArmed(false);

    if (key.tab) return setFocusTo(focus + (key.shift ? -1 : 1));

    const gesture = toGesture(input, key);
    if (gesture === null) return;

    const result = apply({ value: fields[field], cursor }, gesture, caps);
    if (result !== 'pass') return commit(result);

    // ---- 'pass': the screen's existing chain, unchanged ----
    if (gesture.type === 'up') return setFocusTo(focus - 1);
    if (gesture.type === 'down') return setFocusTo(focus + 1);
    if (gesture.type === 'enter') { setDirty(false); return setFlash('saved (prototype: no-op)'); }
  });

  return (
    <Box flexDirection="column" paddingX={1} height={rows}>
      <Panel title="edit 'transcode'">
        <PlainField
          label="name" value={fields.name} cursor={cursor}
          focused={focus === 0} caretStyle={caretStyle}
        />
        <Variant
          label="command"
          value={fields.command}
          cursor={cursor}
          focused={focus === 1}
          innerWidth={innerWidth}
          budget={fieldBudget}
          caretStyle={caretStyle}
          softMarks={softMarks}
        />
        <PlainField
          label="description" value={fields.description} cursor={cursor}
          focused={focus === 2} caretStyle={caretStyle} hint="(optional)"
        />
      </Panel>

      <Panel title="template">
        {templateBudget > 0 && fields.command.trim() !== '' && (
          <CommandBlock value={fields.command} width={innerWidth} maxRows={templateBudget} />
        )}
        {placeholders.length > 0 && (
          <Box flexDirection="column">
            <Text dimColor>args:</Text>
            {placeholders.map((p) => (
              <Text key={p.name}>
                {'  '}<Text color="yellow">{p.name}</Text>
                {p.default !== undefined && <Text dimColor> = {p.default.replace(/\n/g, '␊')}</Text>}
              </Text>
            ))}
          </Box>
        )}
      </Panel>

      <Box flexGrow={1} />

      <Footer
        flash={flash}
        keys={[['↵', 'save'], ['tab', 'next field'], ['esc', 'cancel']]}
      />

      {/* obviously not part of the design under evaluation */}
      <Box marginTop={1} paddingX={1}>
        <Text wrap="truncate">
          <Text backgroundColor="magenta" color="white" bold> PROTOTYPE </Text>
          <Text color="magenta"> {(Variant as { title: string }).title}</Text>
          <Text dimColor>
            {'  ^V var · ^T caret '}{caretStyle}{' · ^R soft-marks '}{softMarks ? 'on' : 'off'}
            {' · ^L '}{SAMPLES[sample]!.label}
            {`  │ ${columns}×${rows} field ${fieldBudget}/${needed} tmpl ${templateBudget} cur ${cursor}`}
          </Text>
        </Text>
      </Box>
    </Box>
  );
}
