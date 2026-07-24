// The potato TUI (spec §3): split-pane list with fuzzy search, single-form
// arg screen with live preview, in-app CRUD. Layout and flow follow the
// prototype accepted in the wayfinder effort (variant C); visual chrome is
// the framed-panel redesign: every screen is built from titled round-border
// panels, with match highlighting, arg-count badges, and last-used times.

import React, { useState } from 'react';
import { Box, Text, useApp, useInput, useStdout } from 'ink';
import type { CommandEntry, Library } from '../library';
import { recordUse, type State } from '../state';
import { nameMatchIndices, searchCommands } from '../search';
import { parsePlaceholders, renderCommand, renderSegments, templateSegments } from '../placeholders';

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

// Brand golds from the banner gradient: the muted bottom row for frames, the
// brighter middle row for controls (footer keys, selection pointer) — chrome
// stays warm, cyan stays reserved for command content.
const FRAME_COLOR = '#d78700';
const ACCENT_COLOR = '#ffaf5f';

// Round-border panel with the title overlaid on the top border
// (marginTop -1 draws the title over the border row).
function Panel(props: {
  title?: string;
  titleColor?: string;
  borderColor?: string;
  width?: number;
  flexGrow?: number;
  children: React.ReactNode;
}) {
  return (
    <Box
      borderStyle="round"
      borderColor={props.borderColor ?? FRAME_COLOR}
      flexDirection="column"
      paddingX={1}
      width={props.width}
      flexGrow={props.flexGrow}
    >
      {props.title !== undefined && (
        <Box marginTop={-1}>
          <Text bold color={props.titleColor} wrap="truncate">
            {' '}{props.title}{' '}
          </Text>
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
        <Text backgroundColor="yellow" color="black">
          {' '}{flash}{' '}
        </Text>
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

// ANSI Shadow wordmark (hermes-agent banner style), assembled per letter so
// the columns stay aligned. Rows are colored with a top-to-bottom gradient.
const GLYPHS: Record<string, string[]> = {
  p: ['██████╗ ', '██╔══██╗', '██████╔╝', '██╔═══╝ ', '██║     ', '╚═╝     '],
  o: [' ██████╗ ', '██╔═══██╗', '██║   ██║', '██║   ██║', '╚██████╔╝', ' ╚═════╝ '],
  t: ['████████╗', '╚══██╔══╝', '   ██║   ', '   ██║   ', '   ██║   ', '   ╚═╝   '],
  a: [' █████╗ ', '██╔══██╗', '███████║', '██╔══██║', '██║  ██║', '╚═╝  ╚═╝'],
};
const BANNER = GLYPHS['p']!.map((_, row) =>
  ['p', 'o', 't', 'a', 't', 'o'].map((c) => GLYPHS[c]![row]).join(''),
);
export const BANNER_WIDTH = Math.max(...BANNER.map((l) => l.length));
const BANNER_GRADIENT = ['#ffd75f', '#ffd75f', '#ffaf5f', '#ffaf5f', '#d78700', '#d78700'];

function Banner() {
  return (
    <Box flexDirection="column" paddingX={1}>
      {BANNER.map((line, i) => (
        <Text key={i} color={BANNER_GRADIENT[i]}>{line}</Text>
      ))}
    </Box>
  );
}

function timeAgo(iso: string, now: Date): string | null {
  const ms = now.getTime() - Date.parse(iso);
  if (!Number.isFinite(ms)) return null;
  const minutes = Math.floor(ms / 60_000);
  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
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
  const columns = stdout?.columns ?? 80;
  // hide the banner on short or narrow terminals to keep the screens usable
  // (app paddingX 1 + banner paddingX 1 on both sides = 4 extra columns)
  const showBanner = rows >= 19 && columns >= BANNER_WIDTH + 4;

  return (
    <Box flexDirection="column" paddingX={1} height={rows}>
      {showBanner && <Banner />}
      {screen.kind === 'list' && (
        <ListScreen
          library={library}
          state={state}
          rows={rows}
          banner={showBanner}
          flash={flash}
          now={deps.now}
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
  banner: boolean;
  flash: string | null;
  now: () => Date;
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

  // search panel (3) + list panel borders (2) + footer (2) + banner
  const chrome = 7 + (props.banner ? BANNER.length : 0);
  const visible = Math.max(2, props.rows - chrome);
  const start = Math.max(0, Math.min(selIdx - visible + 1, results.length - visible));
  const window = results.slice(start, start + visible);
  const selectedEntry = selected ? props.library.commands[selected] : undefined;
  const lastUsedAt = selected ? props.state[selected]?.lastUsedAt : undefined;
  const used = lastUsedAt ? timeAgo(lastUsedAt, props.now()) : null;

  return (
    <Box flexDirection="column" flexGrow={1}>
      <Panel {...(props.banner ? {} : { title: 'potato', titleColor: 'yellow' })}>
        <Box>
          <Text bold color="cyan">/ </Text>
          <Text>{query}</Text>
          <Text color="cyan">▌</Text>
          <Box flexGrow={1} />
          <Text dimColor>
            {query === '' && '(recently used first)  '}
            {results.length}/{total}
          </Text>
        </Box>
      </Panel>
      <Box flexGrow={1}>
        <Panel title="commands" width={30}>
          {results.length === 0 && <Text dimColor>no matches</Text>}
          {window.map((name, i) => {
            const isSel = start + i === selIdx;
            const matches = nameMatchIndices(query, name);
            const argCount = parsePlaceholders(props.library.commands[name]!.command).length;
            return (
              <Box key={name}>
                <Text color={ACCENT_COLOR}>{isSel ? '❯ ' : '  '}</Text>
                <Text wrap="truncate">
                  <Text bold inverse={isSel}>
                    {matches
                      ? name.split('').map((ch, ci) => (
                          <Text key={ci} color={matches.has(ci) ? 'yellow' : undefined}>{ch}</Text>
                        ))
                      : name}
                  </Text>
                  {argCount > 0 && <Text dimColor color="cyan"> ⌁{argCount}</Text>}
                </Text>
              </Box>
            );
          })}
        </Panel>
        {selected && selectedEntry ? (
          <Panel title={selected} flexGrow={1}>
            {selectedEntry.description && <Text dimColor>{selectedEntry.description}</Text>}
            <Box marginTop={selectedEntry.description ? 1 : 0}>
              <Text color="cyan" wrap="wrap">
                <Text dimColor>$ </Text>
                {selectedEntry.command}
              </Text>
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
            {used && (
              <Box marginTop={1}>
                <Text dimColor>used {used}</Text>
              </Box>
            )}
          </Panel>
        ) : (
          <Panel flexGrow={1}>
            <Text dimColor>nothing selected</Text>
          </Panel>
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
    <Box flexDirection="column" flexGrow={1}>
      <Panel title={props.name}>
        <Text dimColor>
          needs {placeholders.length} arg{placeholders.length > 1 ? 's' : ''}
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
      </Panel>
      <Panel title="will run">
        <Text wrap="wrap">
          <Text dimColor>$ </Text>
          {renderSegments(props.entry.command, values).map((seg, i) =>
            seg.substituted ? (
              <Text key={i} bold color="cyan">{seg.text}</Text>
            ) : (
              <Text key={i}>{seg.text}</Text>
            ),
          )}
        </Text>
      </Panel>
      <Box flexGrow={1} />
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

  const name = fields.name.trim();
  const taken = name !== '' && props.isTaken(name);
  const problem = !name
    ? 'name is required'
    : taken
      ? `'${name}' already exists`
      : !fields.command.trim()
        ? 'command is required'
        : null;

  useInput((input, key) => {
    if (key.escape) return props.onBack();
    if (key.return) {
      if (problem) return props.onInvalid(problem);
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
    <Box flexDirection="column" flexGrow={1}>
      <Panel title={props.original ? `edit '${props.original}'` : 'new command'}>
        <Field label="name" value={fields.name} focused={focus === 0} />
        <Field label="command" value={fields.command} focused={focus === 1} />
        <Field label="description" value={fields.description} focused={focus === 2} hint="(optional)" />
        {taken && (
          <Box marginTop={1}>
            <Text color="red">⚠ '{name}' already exists</Text>
          </Box>
        )}
      </Panel>
      <Panel title="template">
        {fields.command.trim() === '' ? (
          <Text dimColor>type a command — {'{{name}}'} or {'{{name=default}}'} become args</Text>
        ) : (
          <Text wrap="wrap">
            <Text dimColor>$ </Text>
            {templateSegments(fields.command).map((seg, i) =>
              seg.placeholder ? (
                <Text key={i} bold color="yellow">{seg.text}</Text>
              ) : (
                <Text key={i} color="cyan">{seg.text}</Text>
              ),
            )}
          </Text>
        )}
        {placeholders.length > 0 && (
          <Box marginTop={1} flexDirection="column">
            <Text dimColor>args:</Text>
            {placeholders.map((p) => (
              <Text key={p.name}>
                {'  '}<Text color="yellow">{p.name}</Text>
                {p.default !== undefined && <Text dimColor> = {p.default}</Text>}
              </Text>
            ))}
          </Box>
        )}
      </Panel>
      <Box flexGrow={1} />
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
    <Box flexDirection="column" flexGrow={1}>
      <Panel title={`delete '${props.name}'?`} titleColor="red" borderColor="red">
        <Text dimColor wrap="truncate">$ {props.entry.command}</Text>
      </Panel>
      <Box flexGrow={1} />
      <Footer flash={null} keys={[['y', 'delete'], ['n / esc', 'keep']]} />
    </Box>
  );
}
