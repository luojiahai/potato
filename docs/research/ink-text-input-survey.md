# Does a maintained Ink text input already solve the cursor?

Research for [#21](https://github.com/luojiahai/potato/issues/21) (map: [#19](https://github.com/luojiahai/potato/issues/19)).

**Answer: no. Nothing is adoptable as a drop-in.** One package — `ink-multiline-input` — is adoptable as a *renderer* behind potato's own cursor and keymap. The other two are not viable at any level short of a rewrite.

All claims below were verified against installed source in a scratch install of `ink@7.1.1` + `react@19.2.8`, or run as live probes through `ink-testing-library@4`. Dependency changes were reverted before commit; no candidate is in `package.json` on this branch.

---

## The constraints that decide it

Three facts about potato's environment do most of the deciding.

### 1. Raw mode is a non-issue — none of the candidates disqualify here

This was the constraint I expected to be fatal, and it isn't. Ink 7's `useInput` toggles raw mode itself:

```js
// node_modules/ink/build/hooks/use-input.js:29-38
const {setRawMode, internal_exitOnCtrlC, internal_eventEmitter} = useStdinContext();
useEffect(() => {
  if (options.isActive === false) return;
  setRawMode(true);
  return () => { setRawMode(false); };
}, [options.isActive, setRawMode]);
```

That `setRawMode` comes from `useStdinContext()`, which resolves to the stream potato handed Ink — so it lands on `src/tui/stdin.ts:30`, `fake.setRawMode = () => fake`, a no-op. Ink's `App.js` calls `stdin.setRawMode(...)` (lines 136, 226) on the same fake object.

**All three candidates handle input exclusively through Ink's `useInput`.** None calls `setRawMode` directly, none imports `useStdin`, none reads `process.stdin`. So potato's caller-owned raw mode and fake `EventEmitter` stream are transparent to them. The disqualifiers listed in the ticket don't fire.

### 2. Every mounted handler receives every keypress — so "screen bindings still win" is the wrong question

`useInput` subscribes each handler to a shared emitter:

```js
// node_modules/ink/build/hooks/use-input.js:119
internal_eventEmitter.on('input', handleData);
```

There is no capture, no propagation, no `stopPropagation`. A child text input and potato's screen-level `useInput` both fire on the same keystroke. So potato's `^Y`/`^A`/`^E`/`^D` bindings are never *blocked* by a text input — the damage is additive: **the screen action fires *and* the field mutates.**

### 3. Ink 7 delivers ctrl+letter as a bare letter, which every candidate then inserts

```js
// node_modules/ink/build/parse-keypress.js:448-450
// ctrl+letter
key.name = String.fromCharCode(s.charCodeAt(0) + 'a'.charCodeAt(0) - 1);
key.ctrl = true;
```

```js
// node_modules/ink/build/hooks/use-input.js:82-87
else if (keypress.ctrl) {
  input = keypress.name ?? '';
}
```

`nonAlphanumericKeys` (`parse-keypress.js:92`) contains only named keys (`up`, `escape`, `tab`, `backspace`, …), never single letters — so the blanking at `use-input.js:91-94` does not apply. **`^Y` arrives at every handler as `input === 'y'`, `key.ctrl === true`.**

All three candidates guard only `ctrl+c` and then fall through to an unconditional insert branch. potato's own `isPrintable` (`src/tui/App.tsx:162-168`) already gets this right by excluding `key.ctrl`; the libraries do not.

Probe result, with a screen-level handler mounted alongside each input, sending `^Y` then `^A` against a starting value of `hello`:

| Component | value after | screen handler |
|---|---|---|
| `ink-text-input` | `helloya` | fired |
| `@inkjs/ui` `TextInput` | `helloya` | fired |
| `ink-multiline-input` `MultilineInput` | `helloy` | fired |

Escape and tab were clean in all three (early-returned or ignored); the value was untouched and the screen handler fired.

---

## `ink-text-input@6.0.0`

Source read: `node_modules/ink-text-input/build/index.js` (113 lines, the whole component).

| | |
|---|---|
| Last publish | **2024-05-14** (registry `time` map; latest of 6.0.0) |
| Peer deps | `{"ink": ">=5", "react": ">=18"}` |
| Runtime deps | `chalk@^5.3.0`, `type-fest@^4.18.2` (types-only, no runtime cost) |
| Dev deps | pins `ink@^5.0.0`, `react@^18.3.1`, `@types/react@^18.3.2` |

**Ink 7 / React 19:** satisfied by range only. The peer range `>=5` admits Ink 7, but the package has never been built or tested against Ink 7 or React 19 — its own devDeps are Ink 5 / React 18, and there has been no release since 2024-05.

**Cursor index:** internal only, and not reachable.

```js
// build/index.js:5-9
const [state, setState] = useState({
  cursorOffset: (originalValue || '').length,
  cursorWidth: 0,
});
const {cursorOffset, cursorWidth} = state;
```

The full prop surface is `value, placeholder, focus, mask, highlightPastedText, showCursor, onChange, onSubmit`. There is no cursor prop, no `onCursorChange`, no ref. The parent sees a string and nothing else.

**Keymap extension:** impossible. One hardcoded `useInput` body (`build/index.js:48-106`), zero configuration props. Early-returns cover `upArrow`, `downArrow`, `ctrl+c`, `tab`, `shift+tab`; `return` calls `onSubmit`. Everything else — including escape and every ctrl combo other than `^C` — reaches the insert branch at line 83. Word and line motions cannot be added without editing the file.

This is a known, unfixed defect upstream: open issue [#75 "Ctrl keys are consumed even when they shouldn't"](https://github.com/vadimdemedes/ink-text-input/issues/75) (2023-01) and [#77 "Ignore escape sequences"](https://github.com/vadimdemedes/ink-text-input/issues/77) (2022-06). Also open: [#81](https://github.com/vadimdemedes/ink-text-input/issues/81) (cursor movement wrong at string bounds), [#73](https://github.com/vadimdemedes/ink-text-input/issues/73) (no `option`/meta shortcuts — i.e. exactly the opt+arrow motions the map wants).

**Multi-line:** none. `key.return` returns early to `onSubmit` (lines 56-61) and `\n` is never inserted. Probe: sending `\r` then `X` against `hello` yields `helloX` — no newline. The `Field` renderer is a single `<Text>`.

**Under potato's stdin:** fine. Ink `useInput` only.

> ### Verdict: **not viable**
> No cursor index is exposed, the keymap has no extension point, and there is no multi-line. Three of the four things the spec needs are absent, and the ctrl-insert bug corrupts a field on `^Y`/`^A`/`^E`/`^D`. A "fork" here means rewriting all 113 lines — that is building it yourself, not adopting.

---

## `@inkjs/ui@2.0.0` — `TextInput`

Source read: `node_modules/@inkjs/ui/build/components/text-input/{text-input,use-text-input,use-text-input-state}.js`.

| | |
|---|---|
| Last publish | **2024-05-22** (only 3 releases ever: 0.0.1, 1.0.0, 2.0.0) |
| Peer deps | `{"ink": ">=5"}` — **no `react` peer dep declared at all** |
| Runtime deps | `chalk@^5.3.0`, `cli-spinners@^3.0.0`, `deepmerge@^4.3.1`, `figures@^6.1.0` |
| Dev deps | pins `ink@^5.0.0`, `react@^18.3.1` |

**Ink 7 / React 19:** by range for Ink; for React, unconstrained and unstated. Never tested against either. Four runtime dependencies arrive for a whole UI kit when potato wants one component — `cli-spinners` and `figures` are pure dead weight in a compiled binary.

**Cursor index:** it exists internally, and is deliberately not public.

```js
// use-text-input-state.js:39-43
const [state, dispatch] = useReducer(reducer, {
  previousValue: defaultValue,
  value: defaultValue,
  cursorOffset: defaultValue.length,
});
```

The reducer's only actions are `move-cursor-left`, `move-cursor-right`, `insert`, `delete` (lines 2-37) — all ±1. There is no absolute "set cursor to index N" action, so even reaching the reducer would not give you line-bounds motion in one step.

And you cannot reach it. `useTextInputState` and `useTextInput` are excluded from the public surface:

```js
// build/components/text-input/index.js
export * from './text-input.js';   // the component only
```

`package.json` `exports` is `{"types": "./build/index.d.ts", "default": "./build/index.js"}` — a single `.` entry, so a deep import of `@inkjs/ui/build/components/text-input/use-text-input-state.js` is blocked by the exports map.

**A second, structural mismatch — the component is uncontrolled.** The prop surface is `isDisabled, placeholder, defaultValue, suggestions, onChange, onSubmit`. There is **no `value` prop**; the string lives inside the reducer. potato's screens own their values: `ArgsScreen` pre-fills from `lastArgs ?? default ?? ''` (`App.tsx:465-467`), `EditScreen` seeds from the library entry and validates live (`App.tsx:538-555`), and both feed the "will run" / "template" preview panels from parent state. An uncontrolled input can only be observed via `onChange` and can never be reset or re-seeded without a remount keyed on new props.

**Keymap extension:** impossible. Hardcoded in `use-text-input.js:40-64`, same guard set as `ink-text-input` (`upArrow`, `downArrow`, `ctrl+c`, `tab`, `shift+tab`), same unconditional `state.insert(input)` fallthrough at line 62. `theme.ts` themes styles only — `styles.value()` is spread onto a `<Text>`; it carries no behaviour.

**Multi-line:** none. No newline handling anywhere, and no textarea component in the package — the components directory is badge, confirm-input, unordered-list, multi-select, progress-bar, select, spinner, text-input, theme, ordered-list, password-input, status-message, alert, email-input. Probe: `\r` then `X` against `hello` yields `helloX`.

**Focus manager:** `TextInput` does *not* use `useFocus`; it gates on its own `isDisabled` prop (`{isActive: !isDisabled}`). Worth noting separately that Ink's `App.js:156-158` resets focus on Escape when focus is enabled — but potato does not use Ink's focus manager, so this is inert here.

> ### Verdict: **not viable**
> Cursor state is walled off behind the package's `exports` map, the reducer offers only ±1 motions even if you breached it, the keymap has no extension point, there is no multi-line, and the component is uncontrolled where potato needs controlled. It also drags in three runtime dependencies potato has no use for.

---

## `ink-multiline-input@0.1.0`

Not named in the ticket; found by scanning npm for `keywords:ink` + input/textarea/multiline. It is the only live candidate and the only one that clears the hard requirements. Source read: `node_modules/ink-multiline-input/dist/index.js`.

| | |
|---|---|
| Publisher | ByteLandTechnology ([repo](https://github.com/ByteLandTechnology/ink-multiline-input)) |
| Releases | `0.0.1` 2025-12-12, `0.1.0` **2026-01-03** — two, ever |
| Peer deps | `{"ink": ">=6", "react": ">=19"}` — **the only candidate that declares React 19** |
| Runtime deps | **none** |

It ships two components, and the distinction is the whole finding.

### `ControlledMultilineInput` — a pure renderer that takes a cursor index as a prop

Its own doc comment (`dist/index.d.ts:64-71`): *"This component is responsible only for displaying the input content and cursor. It doesn't handle any input logic itself."* It calls no `useInput` at all. Props include both `value: string` and `cursorIndex?: number`, plus `rows`, `maxRows`, `placeholder`, `mask`, `showCursor`, `focus`, `tabSize`, `highlight`, `textStyle`, `highlightStyle`.

Probe — rendering `"ab\ncd"` at three externally supplied cursor indices:

```
cursorIndex=0 ->  "[7m [27mab\ncd\n\n"
cursorIndex=3 ->  "ab\n[7m [27mcd\n\n"
cursorIndex=5 ->  "ab\ncd[7m [27m\n\n"
```

The cursor lands where told, including across the newline. This is exactly the shape potato needs if it owns the cursor: a renderer that accepts `(value, cursorIndex)` and draws them.

It also answers part of the map's open rendering question. Overflow is handled by measuring content with Ink's `measureElement` and scrolling via a negative margin inside a clipped box (`dist/index.js:207-232`), with `rows`/`maxRows` bounding the viewport (lines 159-167) and the scroll offset chasing the cursor line (lines 168-188).

### `MultilineInput` — stateful, and *not* adoptable as-is

Real multi-line editing. `keyBindings.newline` defaults to `key.return` and inserts `\n` at the cursor (lines 264-269); `keyBindings.submit` defaults to `ctrl+return`. Up/down arrows do column-preserving line motion (lines 285-352). Probe: `\r` then `X` against `hello` gives `"hello\nX"` — a genuine newline, unlike the other two.

But two limits:

1. **The cursor is internal here.** `const [cursorIndex, setCursorIndex] = useState2(value.length)` (line 251), and it is passed to the renderer *after* the prop spread (`{...controlledProps, value, cursorIndex, ...}`, lines 393-402), so an externally supplied `cursorIndex` is overridden. The stateful variant gives you no cursor control.
2. **The default keymap has the same ctrl-insert bug.** Line 271 guards only `tab`, `shift+tab`, `ctrl+c`; ctrl combos fall to the insert branch at line 375. Probe: `^Y` → `"helloy"`.
3. `keyBindings` covers **only** `submit` and `newline` predicates. No hook for word or line motions.

### The escape hatch that changes the verdict

`MultilineInput` accepts a `useCustomInput` prop that replaces Ink's `useInput` wholesale:

```js
// dist/index.js:248
useCustomInput = (inputHandler, isActive) => useInput(inputHandler, {isActive}),
```

Probe — supplying a custom hook that drops ctrl combos before the component sees them, with potato's screen handler also mounted:

```
DEFAULT after ^Y -> "helloy" log=screen:^Y     // corrupted
CUSTOM  after ^Y -> "hello"  log=screen:^Y     // clean, screen binding still fires
```

So the ctrl collision is fixable from the outside, without a fork. That is the one candidate-side escape hatch found in this whole survey.

**Under potato's stdin:** safe. No `setRawMode`, no `process.stdin`, no `useStdin` — and with `useCustomInput` you control the subscription entirely.

**Risks, stated plainly:**

- **v0.1.0, two releases, ~7 months old, single unknown publisher.** For a dependency in a shipped binary this is the real cost, and it is not small.
- **Layout friction.** It renders into its own `<Box height={visibleRows} overflow="hidden" flexDirection="column">`. potato's `Field` (`App.tsx:96-111`) puts a width-14 label box beside an inline `<Text>` value. Dropping a column-flex Box in there will need layout work.
- React 19 support is a declared peer range, not a tested-of-record claim.
- Unknown: no open-issue history to speak of on a repo this young, so unproven edge cases (wide chars, IME, paste) should be assumed unverified.

> ### Verdict: **adoptable with a wrapper — and only the `ControlledMultilineInput` half**
> Use `ControlledMultilineInput` as a dumb renderer for `(value, cursorIndex)` and keep the cursor and the whole keymap in potato. `MultilineInput` is **not** adoptable: its cursor is unreachable, its keymap extends only to submit/newline, and its default bindings corrupt the field on potato's `^Y`/`^A`/`^E`/`^D`. If the v0.1.0 dependency risk is unacceptable, its renderer is ~200 lines and the scrolling approach is reimplementable — see below.

---

## Ruled out quickly

From an npm scan of `keywords:ink` and searches for ink + input/textarea/multiline/editor:

- `ink-text-input-2@1.0.0` (2024-02) — stale one-off republish of `ink-text-input`; inherits every limitation above.
- `ink-text-input-improved@1.0.1` (2025-05), `@cjser/ink-text-input@6.0.0-cjser.2` (2026-05), `@inkkit/ink-text-input@5.0.2`, `@leondreamed/…`, `@lvi/…`, `@jamiedixon/…`, `@cpmech/…` — personal forks of `ink-text-input`. Same architecture, same missing cursor/keymap/multi-line; adds a single-maintainer fork to the dependency graph. No reason to prefer any over the original.
- `ink-combobox@0.2.1` (2026-07) — autocomplete select, not a text editing surface.
- `ink-confirm-input`, `ink-password-input`, `ink-quicksearch-input`, `ink-select-input`, `ink-multi-select`, `ink-fuzzy-select` — not general text inputs; most are Ink 3/4-era.
- `@matthesketh/ink-input-dispatcher@0.1.0` (2026-04) — input routing, not an input component. Not evaluated further; potato's screen-level `useInput` already routes.
- `ink-mde` — **naming collision, not relevant.** A CodeMirror-based markdown editor for the browser; nothing to do with Ink the TUI library.

---

## Side finding for the map: cursor position *is* assertable in tests

The map lists "how cursor position gets asserted in tests" as unspecified. All three candidates draw the cursor as SGR 7 (inverse), and that survives into `ink-testing-library` frames **when colour is forced**:

```
FORCE_COLOR=1 -> "hell[7mo[27m"     // cursor visible at index 4
(unset)       -> "hello"                        // chalk degrades to level 0, cursor gone
```

So whichever way build-vs-adopt goes, a frame assertion on `[7m` works — but `tests/tui.test.tsx` will need `FORCE_COLOR=1` in its environment, or the cursor is invisible to the test. Worth folding into the rendering ticket.

---

## Verdicts

| Candidate | Verdict | Reason |
|---|---|---|
| `ink-text-input@6.0.0` | **not viable** | No cursor exposed, no keymap extension point, no multi-line; ctrl-insert corrupts the field on `^Y`/`^A`/`^E`/`^D`. Forking means rewriting all 113 lines. |
| `@inkjs/ui@2.0.0` `TextInput` | **not viable** | Cursor state sealed behind the `exports` map, ±1 motions only, no keymap hook, no multi-line, and uncontrolled where potato needs controlled. Plus 3 unwanted runtime deps. |
| `ink-multiline-input@0.1.0` | **adoptable with a wrapper** — renderer only | `ControlledMultilineInput` takes an external `cursorIndex` and does real multi-line rendering with viewport scrolling, zero deps, `ink>=6`/`react>=19`. `MultilineInput` itself is not adoptable (internal cursor, submit/newline-only keybindings, ctrl-insert bug). |

**This points to build, not adopt.** The cursor model, the keymap, and ctrl/meta handling all have to be potato's own in every scenario — no candidate exposes a settable cursor index or a keymap seam wide enough for opt+arrow word motion and cmd+arrow line motion. The only thing genuinely worth adopting is a *renderer*, and that is the smallest part of the job.

### What a hand-built cursor can borrow from their source

- **The fake-cursor rendering trick**, from `ink-text-input` (`build/index.js:30-47`): walk the string, wrap the char at `cursorOffset` in inverse, and append an inverse space when the cursor sits at `value.length` (otherwise a cursor at end-of-string has nothing to invert). potato's `Field` already draws a fixed `▌`; this is the generalisation. `@inkjs/ui` uses the identical approach in `use-text-input.js:14-39`.
- **The reducer shape**, from `@inkjs/ui` (`use-text-input-state.js:2-43`): `{value, previousValue, cursorOffset}` with an `onChange` effect gated on `value !== previousValue`. A clean base to extend with the absolute `set-cursor` action and the word/line motions both libraries lack.
- **Column-preserving vertical motion**, from `ink-multiline-input` (`dist/index.js:285-352`): split on `\n`, find the cursor's line and column, clamp the column to the target line's length, recompute the flat offset. This is the cmd/opt+up/down logic the map needs, already written.
- **Viewport scrolling for the command field**, from `ink-multiline-input` (`dist/index.js:159-232`): `measureElement` for content height, `marginTop={-scrollOffset}` inside `overflow="hidden"`, `rows`/`maxRows` to bound it. Directly relevant to the map's open "how does multi-line render when it outgrows the panel" question.
- **The negative example worth encoding as a test**: all three insert a literal letter on `^Y`. potato's `isPrintable` (`App.tsx:162-168`) already excludes `key.ctrl` and `key.meta` and filters `[\x00-\x1f\x7f]` — that guard is the thing to keep, and a regression test sending `^Y`/`^A`/`^E`/`^D` at every field would pin it.

---

## Note on location

The repo had no existing convention for research notes — `CLAUDE.md` points at `CONTEXT.md` + `docs/adr/` for domain docs, but `docs/adr/` does not exist yet and these findings are a survey, not a decision record. Filed under `docs/research/` as the sensible neighbour to `docs/agents/`. Move it if the ADR directory lands and this should live there instead.
