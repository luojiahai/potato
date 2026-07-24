// PROTOTYPE — throwaway. Wayfinder ticket #6: what does potato look and feel like?
// Three list-layout variants (Ctrl-V cycles). All state in memory. Not the implementation.

import React, { useState } from 'react';
import { render, Box, Text, useInput, useApp, useStdout } from 'ink';
import { seedCommands, seedState, type Command, type CommandState } from './data';
import { search } from './fuzzy';
import { parsePlaceholders, renderCommand, renderSegments } from './placeholders';

type Variant = 'A' | 'B' | 'C';
const VARIANTS: Record<Variant, string> = {
  A: 'compact',
  B: 'two-line',
  C: 'split',
};

type Screen =
  | { kind: 'list' }
  | { kind: 'args'; cmd: Command }
  | { kind: 'edit'; original: Command | null }
  | { kind: 'delete'; cmd: Command };

let handoff: string | null = null; // printed after exit, simulating the shell pre-fill

function copyOsc52(text: string) {
  // stderr: stdout is reserved for the selection hand-off
  process.stderr.write(`\x1b]52;c;${Buffer.from(text).toString('base64')}\x07`);
}

// ---------- shared chrome ----------

function Footer({ keys, flash }: { keys: Array<[string, string]>; flash: string | null }) {
  return (
    <Box marginTop={1}>
      {flash ? (
        <Text backgroundColor="yellow" color="black">
          {' '}{flash}{' '}
        </Text>
      ) : (
        keys.map(([k, label], i) => (
          <Text key={k}>
            {i > 0 && <Text dimColor> · </Text>}
            <Text bold color="magenta">{k}</Text>
            <Text dimColor> {label}</Text>
          </Text>
        ))
      )}
    </Box>
  );
}

function Field({
  label,
  value,
  focused,
  hint,
}: {
  label: string;
  value: string;
  focused: boolean;
  hint?: string;
}) {
  return (
    <Box>
      <Box width={14}>
        <Text bold color={focused ? 'cyan' : undefined}>
          {focused ? '❯ ' : '  '}{label}
        </Text>
      </Box>
      <Text>
        {value}
        {focused && <Text color="cyan">▌</Text>}
        {hint && <Text dimColor>  {hint}</Text>}
      </Text>
    </Box>
  );
}

const isPrintable = (input: string, key: any) =>
  input.length > 0 &&
  !key.ctrl && !key.meta && !key.return && !key.escape && !key.tab &&
  !key.upArrow && !key.downArrow && !key.leftArrow && !key.rightArrow &&
  !key.backspace && !key.delete &&
  // eslint-disable-next-line no-control-regex
  !/[\x00-\x1f\x7f]/.test(input);

// Terminals may deliver ctrl+letter as a raw control byte Ink doesn't decode.
const isCtrl = (input: string, key: any, letter: string) =>
  (key.ctrl && input === letter) ||
  input === String.fromCharCode(letter.charCodeAt(0) - 96);

// ---------- app ----------

function App() {
  const { exit } = useApp();
  const { stdout } = useStdout();
  const [commands, setCommands] = useState<Command[]>(seedCommands);
  const [cmdState, setCmdState] = useState<Record<string, CommandState>>(seedState);
  const [screen, setScreen] = useState<Screen>({ kind: 'list' });
  const [variant, setVariant] = useState<Variant>('A');
  const [flash, setFlashRaw] = useState<string | null>(null);

  const setFlash = (msg: string) => {
    setFlashRaw(msg);
    setTimeout(() => setFlashRaw(null), 1500);
  };

  const run = (cmd: Command, values: Record<string, string>) => {
    handoff = renderCommand(cmd.command, values);
    exit();
  };

  const rememberArgs = (cmd: Command, values: Record<string, string>) =>
    setCmdState((s) => ({
      ...s,
      [cmd.name]: { lastUsedAt: Date.now(), args: { ...s[cmd.name]?.args, ...values } },
    }));

  const rows = stdout?.rows ?? 24;
  const cols = stdout?.columns ?? 80;

  return (
    <Box flexDirection="column" paddingX={1}>
      <Box>
        <Text bold color="yellow">🥔 potato</Text>
        <Text dimColor>  — prototype · layout variant </Text>
        <Text bold>{variant} ({VARIANTS[variant]})</Text>
      </Box>
      {screen.kind === 'list' && (
        <ListScreen
          commands={commands}
          cmdState={cmdState}
          variant={variant}
          rows={rows}
          cols={cols}
          flash={flash}
          onCycleVariant={() => setVariant((v) => (v === 'A' ? 'B' : v === 'B' ? 'C' : 'A'))}
          onRun={(cmd) => {
            const ph = parsePlaceholders(cmd.command);
            if (ph.length > 0) setScreen({ kind: 'args', cmd });
            else run(cmd, {});
          }}
          onCopy={(cmd) => {
            const ph = parsePlaceholders(cmd.command);
            if (ph.length > 0) {
              setScreen({ kind: 'args', cmd });
              setFlash('needs args — fill in, then ^Y');
            } else {
              copyOsc52(cmd.command);
              setFlash('copied to clipboard');
            }
          }}
          onAdd={() => setScreen({ kind: 'edit', original: null })}
          onEdit={(cmd) => setScreen({ kind: 'edit', original: cmd })}
          onDelete={(cmd) => setScreen({ kind: 'delete', cmd })}
          onQuit={exit}
        />
      )}
      {screen.kind === 'args' && (
        <ArgsScreen
          cmd={screen.cmd}
          lastArgs={cmdState[screen.cmd.name]?.args}
          flash={flash}
          onRun={(values) => {
            rememberArgs(screen.cmd, values);
            run(screen.cmd, values);
          }}
          onCopy={(values) => {
            rememberArgs(screen.cmd, values);
            copyOsc52(renderCommand(screen.cmd.command, values));
            setFlash('copied to clipboard');
          }}
          onBack={() => setScreen({ kind: 'list' })}
        />
      )}
      {screen.kind === 'edit' && (
        <EditScreen
          original={screen.original}
          flash={flash}
          onSave={(next) => {
            setCommands((cs) =>
              screen.original
                ? cs.map((c) => (c.name === screen.original!.name ? next : c))
                : [...cs, next],
            );
            setScreen({ kind: 'list' });
            setFlash(screen.original ? 'saved' : 'added');
          }}
          onInvalid={setFlash}
          onBack={() => setScreen({ kind: 'list' })}
        />
      )}
      {screen.kind === 'delete' && (
        <DeleteScreen
          cmd={screen.cmd}
          onConfirm={() => {
            setCommands((cs) => cs.filter((c) => c.name !== screen.cmd.name));
            setScreen({ kind: 'list' });
            setFlash(`deleted '${screen.cmd.name}'`);
          }}
          onBack={() => setScreen({ kind: 'list' })}
        />
      )}
    </Box>
  );
}

// ---------- list ----------

function ListScreen(props: {
  commands: Command[];
  cmdState: Record<string, CommandState>;
  variant: Variant;
  rows: number;
  cols: number;
  flash: string | null;
  onCycleVariant: () => void;
  onRun: (cmd: Command) => void;
  onCopy: (cmd: Command) => void;
  onAdd: () => void;
  onEdit: (cmd: Command) => void;
  onDelete: (cmd: Command) => void;
  onQuit: () => void;
}) {
  const [query, setQuery] = useState('');
  const [sel, setSel] = useState(0);

  const results = search(props.commands, props.cmdState, query);
  const selected = results[Math.min(sel, results.length - 1)] as Command | undefined;

  useInput((input, key) => {
    if (key.escape) return props.onQuit();
    if (isCtrl(input, key, 'v')) return props.onCycleVariant();
    if (isCtrl(input, key, 'a')) return props.onAdd();
    if (isCtrl(input, key, 'e') && selected) return props.onEdit(selected);
    if (isCtrl(input, key, 'd') && selected) return props.onDelete(selected);
    if (isCtrl(input, key, 'y') && selected) return props.onCopy(selected);
    if (key.return && selected) return props.onRun(selected);
    if (key.upArrow) return setSel((s) => Math.max(0, s - 1));
    if (key.downArrow) return setSel((s) => Math.min(results.length - 1, s + 1));
    if (key.backspace || key.delete) {
      setSel(0);
      return setQuery((q) => q.slice(0, -1));
    }
    if (isPrintable(input, key)) {
      setSel(0);
      setQuery((q) => q + input);
    }
  });

  const selIdx = Math.min(sel, Math.max(0, results.length - 1));
  const chrome = 6; // header + search + footer + padding
  const available = Math.max(4, props.rows - chrome);
  const perItem = props.variant === 'B' ? 2 : 1;
  const visible = Math.max(2, Math.floor(available / perItem));
  const start = Math.max(0, Math.min(selIdx - visible + 1, results.length - visible));
  const window = results.slice(start, start + visible);

  return (
    <Box flexDirection="column">
      <Box>
        <Text bold color="cyan">/ </Text>
        <Text>
          {query}
          <Text color="cyan">▌</Text>
        </Text>
        <Text dimColor>
          {'  '}{results.length}/{props.commands.length}
          {query === '' && '  (recently used first)'}
        </Text>
      </Box>
      <Box flexDirection="column" marginTop={1}>
        {results.length === 0 && <Text dimColor>  no matches</Text>}
        {props.variant === 'A' &&
          window.map((cmd, i) => {
            const isSel = start + i === selIdx;
            return (
              <Box key={cmd.name}>
                <Text color="magenta">{isSel ? '❯ ' : '  '}</Text>
                <Box width={22}>
                  <Text bold inverse={isSel}>{cmd.name}</Text>
                </Box>
                <Text dimColor wrap="truncate">  {cmd.command}</Text>
              </Box>
            );
          })}
        {props.variant === 'B' &&
          window.map((cmd, i) => {
            const isSel = start + i === selIdx;
            return (
              <Box key={cmd.name} flexDirection="column">
                <Box>
                  <Text color="magenta">{isSel ? '❯ ' : '  '}</Text>
                  <Text bold inverse={isSel}>{cmd.name}</Text>
                  {cmd.description && <Text dimColor>  — {cmd.description}</Text>}
                </Box>
                <Text color="cyan" dimColor wrap="truncate">
                  {'    '}{cmd.command}
                </Text>
              </Box>
            );
          })}
        {props.variant === 'C' && (
          <Box>
            <Box flexDirection="column" width={26} marginRight={1}>
              {window.map((cmd, i) => {
                const isSel = start + i === selIdx;
                return (
                  <Box key={cmd.name}>
                    <Text color="magenta">{isSel ? '❯ ' : '  '}</Text>
                    <Text bold inverse={isSel} wrap="truncate">{cmd.name}</Text>
                  </Box>
                );
              })}
            </Box>
            <Box
              flexDirection="column"
              borderStyle="round"
              borderDimColor
              paddingX={1}
              flexGrow={1}
            >
              {selected ? (
                <>
                  <Text bold>{selected.name}</Text>
                  {selected.description && <Text dimColor>{selected.description}</Text>}
                  <Box marginTop={1}>
                    <Text color="cyan" wrap="wrap">{selected.command}</Text>
                  </Box>
                  {parsePlaceholders(selected.command).length > 0 && (
                    <Box marginTop={1} flexDirection="column">
                      <Text dimColor>args:</Text>
                      {parsePlaceholders(selected.command).map((p) => (
                        <Text key={p.name}>
                          {'  '}<Text color="yellow">{p.name}</Text>
                          {p.default !== undefined && <Text dimColor> = {p.default}</Text>}
                        </Text>
                      ))}
                    </Box>
                  )}
                </>
              ) : (
                <Text dimColor>nothing selected</Text>
              )}
            </Box>
          </Box>
        )}
      </Box>
      <Footer
        flash={props.flash}
        keys={[
          ['↵', 'run'],
          ['^Y', 'copy'],
          ['^A', 'add'],
          ['^E', 'edit'],
          ['^D', 'delete'],
          ['^V', 'layout'],
          ['esc', 'quit'],
        ]}
      />
    </Box>
  );
}

// ---------- arg prompt ----------

function ArgsScreen(props: {
  cmd: Command;
  lastArgs?: Record<string, string>;
  flash: string | null;
  onRun: (values: Record<string, string>) => void;
  onCopy: (values: Record<string, string>) => void;
  onBack: () => void;
}) {
  const placeholders = parsePlaceholders(props.cmd.command);
  // pre-fill precedence: last value > default > empty
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      placeholders.map((p) => [p.name, props.lastArgs?.[p.name] ?? p.default ?? '']),
    ),
  );
  const [focus, setFocus] = useState(0);

  useInput((input, key) => {
    if (key.escape) return props.onBack();
    if (key.return) return props.onRun(values);
    if (isCtrl(input, key, 'y')) return props.onCopy(values);
    if (key.tab || key.downArrow) return setFocus((f) => (f + 1) % placeholders.length);
    if (key.upArrow) return setFocus((f) => (f - 1 + placeholders.length) % placeholders.length);
    const name = placeholders[focus].name;
    if (key.backspace || key.delete)
      return setValues((v) => ({ ...v, [name]: v[name].slice(0, -1) }));
    if (isPrintable(input, key)) setValues((v) => ({ ...v, [name]: v[name] + input }));
  });

  return (
    <Box flexDirection="column" marginTop={1}>
      <Text>
        <Text bold>{props.cmd.name}</Text>
        <Text dimColor>  needs {placeholders.length} arg{placeholders.length > 1 ? 's' : ''}</Text>
      </Text>
      <Box flexDirection="column" marginTop={1}>
        {placeholders.map((p, i) => {
          const fromLast = props.lastArgs?.[p.name] !== undefined;
          return (
            <Field
              key={p.name}
              label={p.name}
              value={values[p.name]}
              focused={i === focus}
              hint={
                fromLast ? '(last used)' : p.default !== undefined ? `(default: ${p.default})` : undefined
              }
            />
          );
        })}
      </Box>
      <Box flexDirection="column" marginTop={1} borderStyle="round" borderDimColor paddingX={1}>
        <Text dimColor>will run:</Text>
        <Text wrap="wrap">
          {renderSegments(props.cmd.command, values).map((seg, i) =>
            seg.substituted ? (
              <Text key={i} bold color="cyan">{seg.text}</Text>
            ) : (
              <Text key={i}>{seg.text}</Text>
            ),
          )}
        </Text>
      </Box>
      <Footer
        flash={props.flash}
        keys={[
          ['↵', 'run'],
          ['^Y', 'copy'],
          ['tab', 'next field'],
          ['esc', 'back'],
        ]}
      />
    </Box>
  );
}

// ---------- add / edit ----------

function EditScreen(props: {
  original: Command | null;
  flash: string | null;
  onSave: (cmd: Command) => void;
  onInvalid: (msg: string) => void;
  onBack: () => void;
}) {
  const [fields, setFields] = useState({
    name: props.original?.name ?? '',
    command: props.original?.command ?? '',
    description: props.original?.description ?? '',
  });
  const [focus, setFocus] = useState(0);
  const order = ['name', 'command', 'description'] as const;

  useInput((input, key) => {
    if (key.escape) return props.onBack();
    if (key.return) {
      if (!fields.name.trim()) return props.onInvalid('name is required');
      if (!fields.command.trim()) return props.onInvalid('command is required');
      return props.onSave({
        name: fields.name.trim(),
        command: fields.command,
        description: fields.description.trim() || undefined,
      });
    }
    if (key.tab || key.downArrow) return setFocus((f) => (f + 1) % order.length);
    if (key.upArrow) return setFocus((f) => (f - 1 + order.length) % order.length);
    const name = order[focus];
    if (key.backspace || key.delete)
      return setFields((v) => ({ ...v, [name]: v[name].slice(0, -1) }));
    if (isPrintable(input, key)) setFields((v) => ({ ...v, [name]: v[name] + input }));
  });

  const placeholders = parsePlaceholders(fields.command);

  return (
    <Box flexDirection="column" marginTop={1}>
      <Text bold>{props.original ? `edit '${props.original.name}'` : 'new command'}</Text>
      <Box flexDirection="column" marginTop={1}>
        <Field label="name" value={fields.name} focused={focus === 0} />
        <Field label="command" value={fields.command} focused={focus === 1} />
        <Field label="description" value={fields.description} focused={focus === 2} hint="(optional)" />
      </Box>
      {placeholders.length > 0 && (
        <Box marginTop={1}>
          <Text dimColor>
            args detected:{' '}
            {placeholders.map((p) => p.name + (p.default !== undefined ? `=${p.default}` : '')).join(', ')}
          </Text>
        </Box>
      )}
      <Footer
        flash={props.flash}
        keys={[
          ['↵', 'save'],
          ['tab', 'next field'],
          ['esc', 'cancel'],
        ]}
      />
    </Box>
  );
}

// ---------- delete ----------

function DeleteScreen(props: { cmd: Command; onConfirm: () => void; onBack: () => void }) {
  useInput((input, key) => {
    if (input === 'y' || input === 'Y') return props.onConfirm();
    if (input === 'n' || input === 'N' || key.escape || key.return) return props.onBack();
  });
  return (
    <Box flexDirection="column" marginTop={1}>
      <Text>
        delete <Text bold color="red">{props.cmd.name}</Text>?
      </Text>
      <Text dimColor wrap="truncate">  {props.cmd.command}</Text>
      <Footer flash={null} keys={[['y', 'delete'], ['n / esc', 'keep']]} />
    </Box>
  );
}

// ---------- bootstrap ----------

// fzf-style split: the TUI renders on stderr, stdout carries only the selected
// command — so `cmd="$(bun run index.tsx)"` in a shell function captures it.
process.stderr.write('\x1b[?1049h'); // alt screen so the prototype leaves no scrollback
const app = render(<App />, { stdout: process.stderr, exitOnCtrlC: true });
await app.waitUntilExit();
process.stderr.write('\x1b[?1049l');
if (handoff !== null) {
  if (process.stdout.isTTY) {
    // run directly, not through the shell widget — explain instead of pre-filling
    console.log('\x1b[2mwould pre-fill your shell prompt with:\x1b[0m');
    console.log(`  ${handoff}`);
    console.log('\x1b[2mtip: `source potato-proto.zsh` then run `pp` to get it on your real prompt\x1b[0m');
  } else {
    process.stdout.write(handoff + '\n');
  }
}
