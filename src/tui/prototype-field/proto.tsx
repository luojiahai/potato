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
  CommandBlock, LABEL_WIDTH, Panel, rowsNeeded,
  VariantA, VariantB, VariantC, VariantD, VariantE, type CaretStyle,
} from './variants';

/**
 * Layout metadata the host needs before it can size anything.
 * `editChrome` — border rows the field area spends: one panel, or three.
 * `labelRows`  — content rows spent on labels (panel titles ride the border).
 */
const LAYOUTS = [
  { title: VariantE.title, kind: 'panels', valueWidth: VariantE.valueWidth, fixedRows: 0, editChrome: 6, labelRows: 0 },
  { title: VariantD.title, kind: 'stacked', valueWidth: VariantD.valueWidth, fixedRows: 0, editChrome: 2, labelRows: 3 },
  { title: VariantA.title, kind: 'inline', valueWidth: VariantA.valueWidth, fixedRows: 0, editChrome: 2, labelRows: 0 },
  { title: VariantB.title, kind: 'inline', valueWidth: VariantB.valueWidth, fixedRows: 0, editChrome: 2, labelRows: 1 },
  { title: VariantC.title, kind: 'inline', valueWidth: VariantC.valueWidth, fixedRows: 1, editChrome: 2, labelRows: 0 },
] as const;

const ACCENT_COLOR = '#ffaf5f';

// ---------- real chrome, copied from App.tsx (throwaway: no sharing) ----------

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

// command last: it is the only field that grows, so it expands into free space
// instead of shoving name and description down the screen as you type.
const ORDER = ['name', 'description', 'command'] as const;
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
  const [focus, setFocus] = useState<number>(2); // start on the command field
  const [cursor, setCursor] = useState(SAMPLES[2]!.value.length);
  const [variant, setVariant] = useState(0);
  const [caretStyle, setCaretStyle] = useState<CaretStyle>('overlay');
  const [softMarks, setSoftMarks] = useState(false);
  const [separators, setSeparators] = useState(true);
  const [flash, setFlash] = useState<string | null>(null);
  const [armed, setArmed] = useState(false); // escape-confirm-when-dirty (#39)
  const [dirty, setDirty] = useState(false);

  const rows = stdout?.rows ?? 24;
  const columns = stdout?.columns ?? 80;
  const innerWidth = Math.max(20, columns - 6); // root paddingX + panel border + panel paddingX

  const layout = LAYOUTS[variant]!;
  const field = ORDER[focus]!;
  const multiline = field === 'command';
  // E's gutter exists only while the soft-wrap marks are on (see VariantE)
  const valueWidth = Math.max(
    1,
    layout.kind === 'panels' && !softMarks ? innerWidth : layout.valueWidth(innerWidth),
  );

  const placeholders = parsePlaceholders(fields.command);

  // rows the field area spends on everything *except* the command's value rows
  const inline = layout.kind === 'inline';
  const nameRows = inline ? 1 : rowsNeeded(fields.name, valueWidth);
  const descRows = inline ? 1 : rowsNeeded(fields.description, valueWidth);
  const sepRows = layout.kind === 'stacked' && separators ? 2 : 0;
  const nonCommand = nameRows + descRows + layout.labelRows + sepRows;

  // field borders + template border (2) + footer (2) + switcher (2)
  const fixed = layout.editChrome + 6;
  const avail = Math.max(1, rows - fixed - nonCommand);
  const needed = rowsNeeded(fields.command, valueWidth);
  // #44: the field wins the budget; everything below shrinks to zero
  const fieldBudget = layout.fixedRows || Math.max(1, Math.min(needed, avail));
  const rest = Math.max(0, avail - fieldBudget);

  // The args list is unbounded in App.tsx today — a 12-placeholder Command
  // punches straight through the panel's bottom border. Capped here too.
  const argsWanted = placeholders.length > 0 ? 1 + placeholders.length : 0;
  const argsBudget = Math.min(argsWanted, rest);
  const showArgs = argsBudget >= 2;
  const argsCapped = showArgs && argsBudget < argsWanted;
  const shownArgs = showArgs ? Math.max(0, argsBudget - 1 - (argsCapped ? 1 : 0)) : 0;
  const argsUsed = showArgs ? 1 + shownArgs + (argsCapped ? 1 : 0) : 0;
  const templateBudget = Math.max(0, rest - argsUsed);

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
    if (isCtrl(input, key, 'v')) return setVariant((v) => (v + 1) % LAYOUTS.length);
    if (isCtrl(input, key, 't')) return setCaretStyle((s) => (s === 'overlay' ? 'bar' : 'overlay'));
    if (isCtrl(input, key, 'l')) return loadSample((sample + 1) % SAMPLES.length);
    if (isCtrl(input, key, 'r')) return setSoftMarks((s) => !s);
    if (isCtrl(input, key, 'b')) return setSeparators((s) => !s);

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
      {variant === 0 ? (
        // E owns its own frames — one per field, so there is no outer panel
        <VariantE
          fields={fields} focus={focus} cursor={cursor} innerWidth={innerWidth}
          budget={fieldBudget} caretStyle={caretStyle} softMarks={softMarks}
          separators={separators}
        />
      ) : (
        <Panel title="edit 'transcode'">
          {variant === 1 ? (
            <VariantD
              fields={fields} focus={focus} cursor={cursor} innerWidth={innerWidth}
              budget={fieldBudget} caretStyle={caretStyle} softMarks={softMarks}
              separators={separators}
            />
          ) : (
            <>
              <PlainField
                label="name" value={fields.name} cursor={cursor}
                focused={focus === 0} caretStyle={caretStyle}
              />
              <PlainField
                label="description" value={fields.description} cursor={cursor}
                focused={focus === 1} caretStyle={caretStyle} hint="(optional)"
              />
              {(() => {
                const V = variant === 2 ? VariantA : variant === 3 ? VariantB : VariantC;
                return (
                  <V
                    label="command" value={fields.command} cursor={cursor}
                    focused={focus === 2} innerWidth={innerWidth} budget={fieldBudget}
                    caretStyle={caretStyle} softMarks={softMarks}
                  />
                );
              })()}
            </>
          )}
        </Panel>
      )}

      <Panel title="template">
        {templateBudget > 0 && fields.command.trim() !== '' && (
          <CommandBlock value={fields.command} width={innerWidth} maxRows={templateBudget} />
        )}
        {showArgs && (
          <Box flexDirection="column">
            <Text dimColor>args:</Text>
            {placeholders.slice(0, shownArgs).map((p) => (
              <Text key={p.name}>
                {'  '}<Text color="yellow">{p.name}</Text>
                {p.default !== undefined && <Text dimColor> = {p.default.replace(/\n/g, '␊')}</Text>}
              </Text>
            ))}
            {argsCapped && (
              <Text dimColor>{'  '}… +{placeholders.length - shownArgs} more</Text>
            )}
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
          <Text color="magenta"> {layout.title}</Text>
          <Text dimColor>
            {'  ^V var · ^T '}{caretStyle}{' · ^R soft '}{softMarks ? 'on' : 'off'}
            {variant === 1 ? ` · ^B gaps ${separators ? 'on' : 'off'}` : ''}{' · ^L '}{SAMPLES[sample]!.label}
            {`  │ ${columns}×${rows} field ${fieldBudget}/${needed} tmpl ${templateBudget} cur ${cursor}`}
          </Text>
        </Text>
      </Box>
    </Box>
  );
}
