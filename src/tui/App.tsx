// The potato TUI (spec §3): split-pane list with fuzzy search, single-form
// arg screen with live preview, in-app CRUD. Layout and flow follow the
// prototype accepted in the wayfinder effort (variant C).

import React, { useState } from 'react';
import { Box, Text, useApp, useInput, useStdout } from 'ink';
import type { CommandEntry, Library } from '../library';
import { recordUse, type State } from '../state';
import { searchCommands } from '../search';
import { parsePlaceholders, renderCommand, renderSegments } from '../placeholders';

export interface AppDeps {
  library: Library;
  state: State;
  saveLibrary(lib: Library): void;
  saveState(state: State): void;
  copy(text: string): void;
  now(): Date;
}

type Screen =
  | { kind: 'list' }
  | { kind: 'args'; name: string }
  | { kind: 'edit'; original: string | null }
  | { kind: 'delete'; name: string };

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

function Field({ label, value, focused, hint }: { label: string; value: string; focused: boolean; hint?: string }) {
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

type InkKey = Parameters<Parameters<typeof useInput>[0]>[1];

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

const isBackspace = (input: string, key: InkKey) => key.backspace || key.delete || input === '\x7f';

// ---------- app ----------

export function App({ deps, onHandoff }: { deps: AppDeps; onHandoff: (command: string) => void }) {
  const { exit } = useApp();
  const { stdout } = useStdout();
  const [library, setLibrary] = useState<Library>(deps.library);
  const [state, setState] = useState<State>(deps.state);
  const [screen, setScreen] = useState<Screen>({ kind: 'list' });
  const [flash, setFlashRaw] = useState<string | null>(null);

  const setFlash = (msg: string) => {
    setFlashRaw(msg);
    setTimeout(() => setFlashRaw(null), 1500);
  };

  const rememberUse = (name: string, args: Record<string, string>) => {
    const next = recordUse(state, name, args, deps.now());
    setState(next);
    deps.saveState(next);
  };

  const updateLibrary = (next: Library) => {
    setLibrary(next);
    deps.saveLibrary(next);
  };

  const run = (name: string, values: Record<string, string>) => {
    rememberUse(name, values);
    onHandoff(renderCommand(library.commands[name]!.command, values));
    exit();
  };

  const copy = (name: string, values: Record<string, string>) => {
    rememberUse(name, values);
    deps.copy(renderCommand(library.commands[name]!.command, values));
    setFlash('copied to clipboard');
  };

  const rows = stdout?.rows ?? 24;

  return (
    <Box flexDirection="column" paddingX={1}>
      <Box>
        <Text bold color="yellow">🥔 potato</Text>
      </Box>
      {screen.kind === 'list' && (
        <ListScreen
          library={library}
          state={state}
          rows={rows}
          flash={flash}
          onRun={(name) => {
            if (parsePlaceholders(library.commands[name]!.command).length > 0)
              setScreen({ kind: 'args', name });
            else run(name, {});
          }}
          onCopy={(name) => {
            if (parsePlaceholders(library.commands[name]!.command).length > 0) {
              setScreen({ kind: 'args', name });
              setFlash('needs args — fill in, then ^Y');
            } else copy(name, {});
          }}
          onAdd={() => setScreen({ kind: 'edit', original: null })}
          onEdit={(name) => setScreen({ kind: 'edit', original: name })}
          onDelete={(name) => setScreen({ kind: 'delete', name })}
          onQuit={exit}
        />
      )}
      {screen.kind === 'args' && (
        <ArgsScreen
          name={screen.name}
          entry={library.commands[screen.name]!}
          lastArgs={state[screen.name]?.args}
          flash={flash}
          onRun={(values) => run(screen.name, values)}
          onCopy={(values) => copy(screen.name, values)}
          onBack={() => setScreen({ kind: 'list' })}
        />
      )}
      {screen.kind === 'edit' && (
        <EditScreen
          original={screen.original}
          entry={screen.original ? library.commands[screen.original]! : null}
          flash={flash}
          onSave={(name, entry) => {
            // file order is meaningful (spec §1.1): a rename keeps its slot,
            // only genuinely new names are appended
            const commands: typeof library.commands = {};
            for (const [existing, value] of Object.entries(library.commands)) {
              if (existing === screen.original) commands[name] = entry;
              else commands[existing] = value;
            }
            if (screen.original === null) commands[name] = entry;
            updateLibrary({ ...library, commands });
            setScreen({ kind: 'list' });
            setFlash(screen.original ? 'saved' : 'added');
          }}
          onInvalid={setFlash}
          isTaken={(name) => name !== screen.original && library.commands[name] !== undefined}
          onBack={() => setScreen({ kind: 'list' })}
        />
      )}
      {screen.kind === 'delete' && (
        <DeleteScreen
          name={screen.name}
          entry={library.commands[screen.name]!}
          onConfirm={() => {
            const commands = { ...library.commands };
            delete commands[screen.name];
            updateLibrary({ ...library, commands });
            setScreen({ kind: 'list' });
            setFlash(`deleted '${screen.name}'`);
          }}
          onBack={() => setScreen({ kind: 'list' })}
        />
      )}
    </Box>
  );
}

// ---------- list (split pane) ----------

function ListScreen(props: {
  library: Library;
  state: State;
  rows: number;
  flash: string | null;
  onRun: (name: string) => void;
  onCopy: (name: string) => void;
  onAdd: () => void;
  onEdit: (name: string) => void;
  onDelete: (name: string) => void;
  onQuit: () => void;
}) {
  const [query, setQuery] = useState('');
  const [sel, setSel] = useState(0);

  const results = searchCommands(props.library.commands, props.state, query);
  const selIdx = Math.min(sel, Math.max(0, results.length - 1));
  const selected = results[selIdx];
  const total = Object.keys(props.library.commands).length;

  useInput((input, key) => {
    if (key.escape) return props.onQuit();
    if (isCtrl(input, key, 'a')) return props.onAdd();
    if (isCtrl(input, key, 'e') && selected) return props.onEdit(selected);
    if (isCtrl(input, key, 'd') && selected) return props.onDelete(selected);
    if (isCtrl(input, key, 'y') && selected) return props.onCopy(selected);
    if (key.return && selected) return props.onRun(selected);
    if (key.upArrow) return setSel((s) => Math.max(0, s - 1));
    if (key.downArrow) return setSel((s) => Math.min(results.length - 1, s + 1));
    if (isBackspace(input, key)) {
      setSel(0);
      return setQuery((q) => q.slice(0, -1));
    }
    if (isPrintable(input, key)) {
      setSel(0);
      setQuery((q) => q + input);
    }
  });

  const chrome = 6; // header + search + footer + padding
  const visible = Math.max(2, props.rows - chrome);
  const start = Math.max(0, Math.min(selIdx - visible + 1, results.length - visible));
  const window = results.slice(start, start + visible);
  const selectedEntry = selected ? props.library.commands[selected] : undefined;

  return (
    <Box flexDirection="column">
      <Box>
        <Text bold color="cyan">/ </Text>
        <Text>
          {query}
          <Text color="cyan">▌</Text>
        </Text>
        <Text dimColor>
          {'  '}{results.length}/{total}
          {query === '' && '  (recently used first)'}
        </Text>
      </Box>
      <Box marginTop={1}>
        <Box flexDirection="column" width={26} marginRight={1}>
          {results.length === 0 && <Text dimColor>  no matches</Text>}
          {window.map((name, i) => {
            const isSel = start + i === selIdx;
            return (
              <Box key={name}>
                <Text color="magenta">{isSel ? '❯ ' : '  '}</Text>
                <Text bold inverse={isSel} wrap="truncate">{name}</Text>
              </Box>
            );
          })}
        </Box>
        <Box flexDirection="column" borderStyle="round" borderDimColor paddingX={1} flexGrow={1}>
          {selected && selectedEntry ? (
            <>
              <Text bold>{selected}</Text>
              {selectedEntry.description && <Text dimColor>{selectedEntry.description}</Text>}
              <Box marginTop={1}>
                <Text color="cyan" wrap="wrap">{selectedEntry.command}</Text>
              </Box>
              {parsePlaceholders(selectedEntry.command).length > 0 && (
                <Box marginTop={1} flexDirection="column">
                  <Text dimColor>args:</Text>
                  {parsePlaceholders(selectedEntry.command).map((p) => (
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
      <Footer
        flash={props.flash}
        keys={[
          ['↵', 'run'],
          ['^Y', 'copy'],
          ['^A', 'add'],
          ['^E', 'edit'],
          ['^D', 'delete'],
          ['esc', 'quit'],
        ]}
      />
    </Box>
  );
}

// ---------- arg form ----------

function ArgsScreen(props: {
  name: string;
  entry: CommandEntry;
  lastArgs?: Record<string, string>;
  flash: string | null;
  onRun: (values: Record<string, string>) => void;
  onCopy: (values: Record<string, string>) => void;
  onBack: () => void;
}) {
  const placeholders = parsePlaceholders(props.entry.command);
  // pre-fill precedence: last value > default > empty (spec §2)
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(placeholders.map((p) => [p.name, props.lastArgs?.[p.name] ?? p.default ?? ''])),
  );
  const [focus, setFocus] = useState(0);

  useInput((input, key) => {
    if (key.escape) return props.onBack();
    if (key.return) return props.onRun(values);
    if (isCtrl(input, key, 'y')) return props.onCopy(values);
    if (key.tab || key.downArrow) return setFocus((f) => (f + 1) % placeholders.length);
    if (key.upArrow) return setFocus((f) => (f - 1 + placeholders.length) % placeholders.length);
    const name = placeholders[focus]!.name;
    if (isBackspace(input, key)) return setValues((v) => ({ ...v, [name]: v[name]!.slice(0, -1) }));
    if (isPrintable(input, key)) setValues((v) => ({ ...v, [name]: v[name] + input }));
  });

  return (
    <Box flexDirection="column" marginTop={1}>
      <Text>
        <Text bold>{props.name}</Text>
        <Text dimColor>  needs {placeholders.length} arg{placeholders.length > 1 ? 's' : ''}</Text>
      </Text>
      <Box flexDirection="column" marginTop={1}>
        {placeholders.map((p, i) => {
          const fromLast = props.lastArgs?.[p.name] !== undefined;
          return (
            <Field
              key={p.name}
              label={p.name}
              value={values[p.name]!}
              focused={i === focus}
              hint={fromLast ? '(last used)' : p.default !== undefined ? `(default: ${p.default})` : undefined}
            />
          );
        })}
      </Box>
      <Box flexDirection="column" marginTop={1} borderStyle="round" borderDimColor paddingX={1}>
        <Text dimColor>will run:</Text>
        <Text wrap="wrap">
          {renderSegments(props.entry.command, values).map((seg, i) =>
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
  original: string | null;
  entry: CommandEntry | null;
  flash: string | null;
  onSave: (name: string, entry: CommandEntry) => void;
  onInvalid: (msg: string) => void;
  isTaken: (name: string) => boolean;
  onBack: () => void;
}) {
  const [fields, setFields] = useState({
    name: props.original ?? '',
    command: props.entry?.command ?? '',
    description: props.entry?.description ?? '',
  });
  const [focus, setFocus] = useState(0);
  const order = ['name', 'command', 'description'] as const;

  useInput((input, key) => {
    if (key.escape) return props.onBack();
    if (key.return) {
      const name = fields.name.trim();
      if (!name) return props.onInvalid('name is required');
      if (!fields.command.trim()) return props.onInvalid('command is required');
      if (props.isTaken(name)) return props.onInvalid(`'${name}' already exists`);
      // preserve unknown extra fields when editing an existing entry
      const entry: CommandEntry = { ...props.entry, command: fields.command };
      if (fields.description.trim()) entry.description = fields.description.trim();
      else delete entry.description;
      return props.onSave(name, entry);
    }
    if (key.tab || key.downArrow) return setFocus((f) => (f + 1) % order.length);
    if (key.upArrow) return setFocus((f) => (f - 1 + order.length) % order.length);
    const field = order[focus]!;
    if (isBackspace(input, key)) return setFields((v) => ({ ...v, [field]: v[field].slice(0, -1) }));
    if (isPrintable(input, key)) setFields((v) => ({ ...v, [field]: v[field] + input }));
  });

  const placeholders = parsePlaceholders(fields.command);

  return (
    <Box flexDirection="column" marginTop={1}>
      <Text bold>{props.original ? `edit '${props.original}'` : 'new command'}</Text>
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

// ---------- delete confirm ----------

function DeleteScreen(props: { name: string; entry: CommandEntry; onConfirm: () => void; onBack: () => void }) {
  useInput((input, key) => {
    if (input === 'y' || input === 'Y') return props.onConfirm();
    if (input === 'n' || input === 'N' || key.escape || key.return) return props.onBack();
  });
  return (
    <Box flexDirection="column" marginTop={1}>
      <Text>
        delete <Text bold color="red">{props.name}</Text>?
      </Text>
      <Text dimColor wrap="truncate">  {props.entry.command}</Text>
      <Footer flash={null} keys={[['y', 'delete'], ['n / esc', 'keep']]} />
    </Box>
  );
}
