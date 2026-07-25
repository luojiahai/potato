# What Ghostty and Terminal.app emit for the movement keys

Research for [issue #20](https://github.com/luojiahai/potato/issues/20), part of the map in
[issue #19](https://github.com/luojiahai/potato/issues/19).

**Question.** What byte sequences do Ghostty and Terminal.app actually send for the movement keys
— and what of that survives Ink 7 plus potato's Bun stdin wrapper?

---

## How to read this document

There is a hard empirical ceiling on this ticket: the research agent cannot press a key in a real
Ghostty or Terminal.app window. So every claim below is tagged:

| Tag | Meaning |
| --- | --- |
| **[SRC]** | Read out of source, a shipped binary's own output, or a normative spec. Treat as settled. |
| **[SPEC]** | Follows necessarily from a spec (Kitty keyboard protocol / xterm modifier encoding) applied to a **[SRC]** fact. Settled unless the emulator is non-conformant. |
| **[CONFIRM]** | Not established from a primary source. **Needs a human keypress.** Run `scripts/keycapture.ts` (see §7). |

Nothing tagged **[CONFIRM]** should be treated as known. §6 lists every such row in one place.

Versions this was checked against:

- Ink **7.1.1** — `node_modules/ink/build/*.js` (the shipped build; `package.json` pins `ink: ^7`).
- Ghostty **1.3.1** stable — `/Applications/Ghostty.app/Contents/MacOS/ghostty +version`. **[SRC]**
- Bun 1.3.14 + Ink 7.1.1 per the header comment in `src/tui/stdin.ts`.
- Kitty keyboard protocol spec: <https://sw.kovidgoyal.net/kitty/keyboard-protocol/>
- macOS Terminal.app: no version pinned; **all Terminal.app rows are [CONFIRM]** (see §3).

---

## 1. Ink 7.1.1's decoder: what actually reaches `useInput`

All of §1 is **[SRC]** — read from `node_modules/ink/build/`.

### 1.1 The pipeline

`stdin` `'readable'` → `Ink.App.handleReadable` (`components/App.js:175`) → `inputParser.push(chunk)`
(`input-parser.js:169`) → one `emitInput(event)` per decoded event → `internal_eventEmitter.emit('input')`
→ `useInput`'s `handleData` (`hooks/use-input.js:39`) → `parseKeypress` (`parse-keypress.js:366`) →
the `key` object.

### 1.2 Legacy (non-Kitty) decoding — `parse-keypress.js`

Two regexes matter:

```js
const metaKeyCodeRe = /^(?:\x1b)([a-zA-Z0-9])$/;                                    // :4
const fnKeyRe = /^(?:\x1b+)(O|N|\[|\[\[)(?:(\d+)(?:;(\d+))?([~^$])|(?:1;)?(\d+)?([a-zA-Z]))/; // :5
```

`fnKeyRe` strips the `1;` and reads the modifier parameter, then (`parse-keypress.js:482`):

```js
const modifier = (parts[3] || parts[5] || 1) - 1;
key.ctrl  = !!(modifier & 4);
key.meta  = key.meta || !!(modifier & 10);
key.shift = !!(modifier & 1);
```

This is the xterm modifier encoding: parameter = `1 + (1·shift + 2·alt + 4·ctrl + 8·meta)`. The
mask `& 10` is `0b1010` = alt (2) **or** meta (8).

> **Consequence — the central collapse.** In the legacy path Ink has no `super`/`cmd` concept.
> `alt` (bit 2) and `meta` (bit 8) both land on the single boolean `key.meta`. Any two modified
> arrow presses whose xterm modifier parameter differs only in bits 2 vs 8 are **indistinguishable**
> to `useInput`. **[SRC]**

This was not only read but **executed** against Ink 7.1.1's shipped modules — feeding candidate
sequences straight into the real `parseKeypress`. Verbatim results:

| Fed in | `parseKeypress` returns |
| --- | --- |
| `\x1b[1;3D` (alt+←) | `{name: 'left', ctrl: false, meta: true, shift: false}` |
| `\x1b[1;9D` (super+←) | `{name: 'left', ctrl: false, meta: true, shift: false}` — **byte-identical output** |
| `\x1bb` | `{name: 'b', meta: true}` |
| `\x01` | `{name: 'a', ctrl: true}` |
| `\x15` | `{name: 'u', ctrl: true}` |
| `\x1b\x7f` | `{name: 'backspace', meta: true}` |
| `\r` | `{name: 'return'}` |
| `\n` | `{name: 'enter'}` — a *different* name; `useInput` has no `key.enter`, so `\n` arrives as `input === '\n'` with no flags |
| `\x1b[H` | `{name: 'home'}` |
| `\x1b[13;2u` | `{name: 'return', shift: true, isKittyProtocol: true, isPrintable: true, text: '\r', eventType: 'press'}` |
| `\x1b[27u` | `{name: 'escape', isKittyProtocol: true, eventType: 'press'}` |
| `\x1b[127;3u` | `{name: 'backspace', meta: true, isKittyProtocol: true}` |
| `\x1b[1;3:1A` (alt+↑, kitty) | `{name: 'up', meta: true, super: false, eventType: 'press'}` |
| `\x1b[1;9:1D` (super+←, kitty) | `{name: 'left', meta: false, **super: true**, eventType: 'press'}` |

The last two rows are the payoff: **with the `:eventType` sub-parameter present, cmd and opt are
fully distinguishable** (`super` vs `meta`); without it they are not. **[SRC]**

Also note `input` handling in `hooks/use-input.js:82-99`:

- `nonAlphanumericKeys` (`parse-keypress.js:92`) is every value in the `keyName` table plus
  `'backspace'`. For those, **`input` is forced to `''`** (`use-input.js:91-94`). Arrows, home, end,
  delete, tab, backspace all arrive with `input === ''`.
- `'return'` and `'escape'` are **not** in `keyName`, so they are not zeroed there — but
  `use-input.js:97-99` strips a leading `\x1b`, which reduces a lone `\x1b` to `''`.
- For `key.ctrl`, `input` is set to the *letter name* (`use-input.js:82-87`), so `0x01` arrives as
  `input === 'a', key.ctrl === true`.
- `\x1bb` (`metaKeyCodeRe`) gives `name: 'b', meta: true`; `input` starts as the raw sequence
  `'\x1bb'`, is **not** zeroed (no `'b'` in `keyName`), then the `\x1b` strip at `use-input.js:97`
  leaves **`input === 'b'` with `key.meta === true` and no arrow flag at all**. **[SRC]**

### 1.3 Kitty decoding — two separate parsers

- `kittyKeyRe = /^\x1b\[(\d+)(?:;(\d+)(?::(\d+))?(?:;([\d:]+))?)?u$/` — the `CSI codepoint ; mods u`
  form (`parse-keypress.js:125`). Modifier bits are read with the **Kitty** table
  (`kitty-keyboard.js:22`): `shift 1, alt 2, ctrl 4, super 8, hyper 16, meta 32, capsLock 64,
  numLock 128`, and `meta` is set from `(meta | alt)` (`parse-keypress.js:273`) while `super` gets
  its own field. `useInput` exposes `key.super`, `key.hyper`, `key.capsLock`, `key.numLock`,
  `key.eventType` (`use-input.js:59-63`). **[SRC]**
- `kittySpecialKeyRe = /^\x1b\[(\d+);(\d+):(\d+)([A-Za-z~])$/` — arrows/function keys
  (`parse-keypress.js:129`). **It requires the `:eventType` sub-parameter.** A bare `\x1b[1;9D`
  does not match and falls through to the legacy path, losing `super`. Ink's own comment gives
  `\x1b[1;1:1A` as the up-arrow-press example. **[SRC]**

> **Consequence.** To tell cmd+arrow apart from opt+arrow in Ink 7 you need the Kitty protocol with
> **both** `disambiguateEscapeCodes` (1) **and** `reportEventTypes` (2). Flag 1 alone leaves
> modified arrows in the `CSI 1 ; mods A` form, which Ink decodes through the legacy collapse. **[SRC]** for
> Ink's side; **[CONFIRM]** that Ghostty emits `:1` for press events when flag 2 is on.

### 1.4 Kitty is opt-in in Ink 7, and auto-detection cannot work in potato

`Ink.initKittyKeyboard` (`ink.js:799`):

```js
// Protocol is opt-in: if kittyKeyboard is not specified, do nothing
if (!this.options.kittyKeyboard) { return; }
```

potato's `render()` call in `src/cli.tsx` passes only `{stdin, exitOnCtrlC: true}` — **the Kitty
protocol is off in potato today.** **[SRC]**

Worse, `mode: 'auto'` *cannot* work through `bunSafeStdin`. `confirmKittySupport` (`ink.js:830-863`)
writes `CSI ? u` and waits for the reply via `stdin.on('data', onData)`, then returns leftovers with
`stdin.unshift(...)`. potato's fake stream is a bare `EventEmitter` that **only ever emits
`'readable'`** and has **no `unshift`** (`src/tui/stdin.ts:23-40`). The probe reply arrives on the
*real* stdin, is pumped into the queue as ordinary input, and the detector's 200 ms timer expires
having seen nothing. Auto-detect would silently never enable the protocol. **[SRC]**

Workable options, both source-verified:

1. `render(..., {kittyKeyboard: {mode: 'enabled', flags: ['disambiguateEscapeCodes', 'reportEventTypes']}})`
   — `ink.js:812-816` only checks `stdin.isTTY && stdout.isTTY` and writes `CSI > flags u`
   unconditionally. `fake.isTTY` is inherited from the real stdin (`stdin.ts:24`), so this works.
   Unsupporting terminals ignore the sequence. Teardown writes `CSI < u` at `ink.js:540`.
2. Teach `bunSafeStdin` to emit `'data'` (and implement `unshift`) so `mode: 'auto'` works.

---

## 2. Ghostty 1.3.1 at defaults

Everything in §2.1 comes from Ghostty's own binary: `ghostty +list-keybinds --default` (93 lines).
That is a **[SRC]** primary source — it is the shipped default table, not documentation about it.

### 2.1 The default bindings that touch our keys, verbatim

```
keybind = alt+arrow_left=esc:b
keybind = alt+arrow_right=esc:f
keybind = super+arrow_left=text:\\x01
keybind = super+arrow_right=text:\\x05
keybind = super+backspace=text:\\x15
keybind = super+arrow_up=jump_to_prompt:-1
keybind = super+arrow_down=jump_to_prompt:1
keybind = super+shift+arrow_up=jump_to_prompt:-1
keybind = super+shift+arrow_down=jump_to_prompt:1
keybind = super+alt+arrow_up=goto_split:up
keybind = super+alt+arrow_down=goto_split:down
keybind = super+alt+arrow_left=goto_split:left
keybind = super+alt+arrow_right=goto_split:right
keybind = super+ctrl+arrow_up=resize_split:up,10
keybind = super+ctrl+arrow_down=resize_split:down,10
keybind = super+ctrl+arrow_left=resize_split:left,10
keybind = super+ctrl+arrow_right=resize_split:right,10
keybind = shift+arrow_left=adjust_selection:left
keybind = shift+arrow_right=adjust_selection:right
keybind = shift+arrow_up=adjust_selection:up
keybind = shift+arrow_down=adjust_selection:down
keybind = super+home=scroll_to_top
keybind = super+end=scroll_to_bottom
keybind = shift+home=adjust_selection:home
keybind = shift+end=adjust_selection:end
keybind = super+enter=toggle_fullscreen
keybind = super+shift+enter=toggle_split_zoom
keybind = escape=end_search
```

`ghostty +list-keybinds --default` has **no** entry for `alt+arrow_up`, `alt+arrow_down`,
`alt+backspace`, `shift+enter`, plain `home`, plain `end`, or plain arrows — those are unbound and
fall through to Ghostty's key encoder. The `\\x01` double-backslash is the config file's own
escaping; the byte sent is `0x01`. **[SRC]**

The action syntax is documented at <https://ghostty.org/docs/config/keybind>: **[SRC]**

> `text:` — "Send a string. Uses Zig string literal syntax. i.e. `text:\x15` sends Ctrl-U."
> `csi:` — "Send a CSI sequence. i.e. `csi:A` sends 'cursor up'."
> `esc:` — "Send an escape sequence. i.e. `esc:d` deletes to the end of the word to the right."

So `esc:b` sends `ESC b` = `0x1b 0x62`, and `esc:f` sends `0x1b 0x66`. **[SRC]**

### 2.2 `macos-option-as-alt` — and why it does not matter for arrows

`ghostty +show-config --default --docs`, verbatim: **[SRC]**

> The default behavior (unset) will depend on your active keyboard layout. If your keyboard layout
> is one of the keyboard layouts listed below, then the default value is "true". Otherwise, the
> default value is "false". Keyboard layouts with a default value of "true" are:
> - U.S. Standard
> - U.S. International
>
> Note that if an *Option*-sequence doesn't produce a printable character, it will be treated as
> *Alt* regardless of this setting. (e.g. `alt+ctrl+a`).

Arrows, Backspace and Enter never produce a printable Option character, so **option is Alt for all
the keys on this ticket regardless of layout or of `macos-option-as-alt`.** The setting only
changes `option+letter`. The default printed value is empty (unset). **[SRC]**

There is no `~/.config/ghostty/config` and no
`~/Library/Application Support/com.mitchellh.ghostty/config` on this machine, so the defaults above
are what the dev machine actually runs. **[SRC]**

### 2.3 `escape=end_search` is not a hazard

`escape` is bound, but to `end_search`, which is only live while Ghostty's own search overlay is
open. With no overlay, `Escape` passes through to the app. **[CONFIRM]** — the scoping of
`end_search` is inferred from the action name and its neighbours (`start_search`, `navigate_search`),
not read out of Ghostty's source. It is trivially confirmable: potato quits on `esc` today, in
Ghostty, and that works.

---

## 3. Terminal.app — why every row is [CONFIRM]

Terminal.app's key mapping lives in its Settings → Profiles → Keyboard table and in the
`com.apple.Terminal` preferences domain, and Apple publishes no normative document that lists the
default mapping table byte-for-byte. `defaults read com.apple.Terminal` on this machine returns
nothing matching `metaKey`/`keyMap`/`Option` — i.e. **the profile has never been customised, so
factory defaults apply**, but the factory defaults themselves are not recoverable from the plist
(they live inside the app bundle's default profile). **[SRC]** for "unmodified", not for the values.

The following statements about Terminal.app *are* settled, on protocol grounds:

- Terminal.app reports `TERM=xterm-256color` and implements no Kitty keyboard protocol. **[CONFIRM]**
  — but the capture script probes this directly with `CSI ? u`, so one run settles it.
- Without the Kitty protocol (or xterm's `modifyOtherKeys`), **shift+Enter is physically
  inexpressible.** The Kitty spec is explicit about the class of ambiguity: **[SPEC]**

  > "Many of the legacy escape codes are ambiguous with multiple different key presses yielding the
  > same escape code(s), for example, ctrl+i is the same as tab, ctrl+m is the same as Enter,
  > ctrl+r is the same ctrl+shift+r, etc."

  Legacy encoding has one byte for Return (`0x0d`) and no modifier channel for it. So in
  Terminal.app, shift+Enter is `\r`, identical to Enter. This is the compat floor.
- Likewise `Esc`: **[SPEC]**

  > "No reliable way to distinguish single `Esc` key presses from the start of a escape sequence.
  > Currently, client programs use fragile timing related hacks for this, leading to bugs."

  Ink 7 uses exactly such a hack — see §5.

Everything else about Terminal.app (opt+arrow, cmd+arrow, opt+Backspace, cmd+Backspace, fn+arrow)
is **[CONFIRM]**. Do not design the keymap around a guess here; run the script.

---

## 4. The table

`input` and `key` are what potato's `useInput` callback receives. Modifier fields not listed are
`false`. Arrows/home/end/backspace always carry `input === ''` (§1.2).

### 4.1 Ghostty 1.3.1, defaults, Kitty protocol OFF (what potato does today)

| Physical key | Bytes on the wire | `useInput` receives | Tag |
| --- | --- | --- | --- |
| `←` / `→` | `\x1b[D` / `\x1b[C` (or `\x1bOD`/`\x1bOC` in DECCKM) | `''`, `{leftArrow}` / `{rightArrow}` | bytes **[SPEC]**, decode **[SRC]** |
| `↑` / `↓` | `\x1b[A` / `\x1b[B` | `''`, `{upArrow}` / `{downArrow}` | bytes **[SPEC]**, decode **[SRC]** |
| **opt+←** | `\x1b b` = `1b 62` (`alt+arrow_left=esc:b`) | **`'b'`, `{meta: true}` — no arrow flag** | **[SRC]** |
| **opt+→** | `\x1b f` = `1b 66` (`alt+arrow_right=esc:f`) | **`'f'`, `{meta: true}` — no arrow flag** | **[SRC]** |
| **cmd+←** | `\x01` (`super+arrow_left=text:\x01`) | **`'a'`, `{ctrl: true}`** — i.e. ctrl+a | **[SRC]** |
| **cmd+→** | `\x05` (`super+arrow_right=text:\x05`) | **`'e'`, `{ctrl: true}`** — i.e. ctrl+e | **[SRC]** |
| **opt+↑ / opt+↓** | unbound → encoder → `\x1b[1;3A` / `\x1b[1;3B` | `''`, `{upArrow, meta}` / `{downArrow, meta}` | binding absence **[SRC]**, bytes **[CONFIRM]** |
| **cmd+↑ / cmd+↓** | **swallowed** — `jump_to_prompt` | *nothing* | **[SRC]** |
| **shift+Enter** | unbound → `\r` | `'\r'`, `{return: true}` — same as Enter | binding absence **[SRC]**, bytes **[SPEC]** |
| Enter | `\r` | `'\r'`, `{return: true}` | **[SRC]** |
| cmd+Enter | **swallowed** — `toggle_fullscreen` | *nothing* | **[SRC]** |
| **opt+Backspace** | unbound → encoder → `\x1b\x7f` | `''`, `{backspace: true, meta: true}` | binding absence **[SRC]**, bytes **[CONFIRM]**, decode **[SRC]** (`parse-keypress.js:433`) |
| **cmd+Backspace** | `\x15` (`super+backspace=text:\x15`) | **`'u'`, `{ctrl: true}`** — i.e. ctrl+u | **[SRC]** |
| Backspace | `\x7f` | `''`, `{backspace: true}` | **[SRC]** |
| **fn+← / fn+→** (Home/End) | unbound → `\x1b[H` / `\x1b[F` (or `\x1bOH`/`\x1bOF`, or `\x1b[1~`/`\x1b[4~`) | `''`, `{home: true}` / `{end: true}` — Ink accepts all three spellings (`parse-keypress.js:43,52,54,57`) | binding absence **[SRC]**, decode **[SRC]**, which spelling **[CONFIRM]** |
| `Esc` | `\x1b` | `''`, `{escape: true}`, **delayed ~20 ms** (§5) | **[SRC]** |

> ### Two live collisions in potato today, from this table
>
> `src/tui/App.tsx:171` — `isCtrl(input, key, letter)` matches `key.ctrl && input === letter`.
> In `ListScreen` (`App.tsx:343-346`) that means, in Ghostty at defaults:
>
> - **cmd+← → `\x01` → `isCtrl(…,'a')` → `props.onAdd()`** — opens the *new command* screen.
> - **cmd+→ → `\x05` → `isCtrl(…,'e')` → `props.onEdit(...)`** — opens the *edit* screen.
>
> cmd+Backspace → `\x15` → ctrl+u is currently unhandled (no collision). Both collisions are
> **[SRC]**-grade: the byte comes from Ghostty's default table, the decode from Ink's source, the
> dispatch from potato's source. The keymap must either rebind these in Ghostty (which the compat
> floor rule forbids — potato cannot require config) or accept ctrl+a/ctrl+e as *aliases* for
> line-start/line-end and move add/edit off them.

### 4.2 Ghostty, Kitty protocol ON — `flags = disambiguateEscapeCodes | reportEventTypes` (3)

Only the rows that change. Ghostty's **default keybinds still win over the encoder**, so opt+←/→ and
cmd+←/→ and cmd+↑/↓ are *unchanged* by enabling the protocol — a `keybind` intercepts the key before
any encoding happens.

| Physical key | Bytes | `useInput` receives | Tag |
| --- | --- | --- | --- |
| **shift+Enter** | `\x1b[13;2u` | `'\r'`, `{return: true, shift: true, eventType: 'press'}` — **distinguishable** | **[SPEC]** for bytes, **[SRC]** for decode (`parse-keypress.js:307`, `use-input.js:66-71`) |
| `Esc` | `\x1b[27u` | `''`, `{escape: true}`, **no 20 ms delay** | **[SPEC]** bytes, **[SRC]** decode (`parse-keypress.js:167`) |
| opt+↑ / opt+↓ | `\x1b[1;3:1A` / `\x1b[1;3:1B` | `''`, `{upArrow, meta: true, eventType: 'press'}` | **[CONFIRM]** — the `:1` |
| opt+Backspace | `\x1b[127;3u` | `''`, `{backspace: true, meta: true}` | **[CONFIRM]** |
| Enter, Tab, Backspace **unmodified** | unchanged — `\r`, `\t`, `\x7f` | unchanged | **[SPEC]** — the spec is explicit: *"The only exceptions are the Enter, Tab and Backspace keys which still generate the same bytes as in legacy mode"* |

Spec basis for the arrow form, verbatim from the Kitty protocol: **[SPEC]**

> "With this flag turned on, all key events that do not generate text are represented in one of the
> following two forms: `CSI number; modifier u` [or] `CSI 1; modifier [~ABCDEFHPQS]`"

Arrows take the second form, so Ink's `kittySpecialKeyRe` (which needs `:eventType`) is the only
route to `key.super` on an arrow — hence flag 2 being mandatory, not optional. **[SRC]**

Kitty modifier parameter = `1 + bitmask`, bitmask `shift 1, alt 2, ctrl 4, super 8, hyper 16,
meta 32, caps_lock 64, num_lock 128`. **[SPEC]** Ghostty maps macOS ⌘ to `super`: **[CONFIRM]** —
strongly implied by `super+…` being the spelling of every ⌘ binding in `+list-keybinds`, but the
encoder's modifier bit was not read from Ghostty's source.

### 4.3 Terminal.app, factory defaults

| Physical key | Bytes on the wire | `useInput` receives | Tag |
| --- | --- | --- | --- |
| `←` `→` `↑` `↓` | `\x1b[D` `\x1b[C` `\x1b[A` `\x1b[B` | arrow flags, `input === ''` | bytes **[CONFIRM]**, decode **[SRC]** |
| **shift+Enter** | `\r` | `'\r'`, `{return: true}` — **indistinguishable from Enter** | **[SPEC]** (§3) |
| **opt+←/→** | unknown: plain `\x1b[D`, or `\x1b b`/`\x1b f`, or `\x1b[1;3D` | *depends* | **[CONFIRM]** |
| **opt+↑/↓** | unknown | *depends* | **[CONFIRM]** |
| **cmd+←/→/↑/↓** | unknown — likely swallowed as a menu shortcut or a no-op | probably *nothing* | **[CONFIRM]** |
| **opt+Backspace** | unknown: `\x7f` or `\x1b\x7f` | *depends* | **[CONFIRM]** |
| **cmd+Backspace** | unknown — likely swallowed | probably *nothing* | **[CONFIRM]** |
| **fn+←/→** | unknown; Terminal.app may bind Home/End to *scroll the buffer* rather than send bytes | *depends* | **[CONFIRM]** |
| `Esc` | `\x1b` | `''`, `{escape: true}`, delayed ~20 ms | bytes **[CONFIRM]**, decode **[SRC]** |
| "Use Option as Meta key" default | believed **off** | — | **[CONFIRM]** |

---

## 5. `src/tui/stdin.ts`: does anything get lost?

Answers to the ticket's question 4. All **[SRC]**.

### 5.1 A split escape sequence is reassembled — Ink 7.1.1 handles this properly

`createInputParser` keeps a `pending` string across `push()` calls (`input-parser.js:167-173`) and
parses `pending + chunk`. `parseEscapeSequence` returns the sentinel `'pending'` whenever the
sequence is incomplete (`input-parser.js:75-77`, `:36`, `:46`), so `\x1b` + `[1;` + `3D` arriving as
three `data` chunks reassembles into one `\x1b[1;3D` event. potato's `pump` pushing one chunk per
`data` event does not break this.

### 5.2 Coalesced keypresses are re-split — no loss

`parseKeypresses` loops over the buffer emitting one event per sequence (`input-parser.js:132-160`),
and `handleReadable` calls `emitInput` once per event (`App.js:181-194`). Two keys in one chunk fire
`useInput` twice. Held-down backspace is additionally split byte-by-byte by `splitBackspaceBytes`
(`input-parser.js:109`) — with a comment saying `\r` and `\t` are deliberately *not* split because
they occur in pasted text.

`handleReadable` drains with `while ((chunk = stdin.read()) !== null)`, so potato's `queue`
(`stdin.ts:21,35`) is fully consumed on every `'readable'` — even if several chunks accumulated.

Both behaviours were **executed** against Ink 7.1.1's real `createInputParser`:

| Pushed | Events out |
| --- | --- |
| `'\x1b'` then `'[1;'` then `'3D'` (three chunks) | `[]`, `[]`, `['\x1b[1;3D']` — reassembled |
| `'\x1b[Dab\x1b[C'` (one chunk) | `['\x1b[D', 'ab', '\x1b[C']` — re-split |
| `'\x1b'` alone | `[]`, then `hasPendingEscape() === true`, then `flushPendingEscape() === '\x1b'` |

Note the middle row: the two literal characters `ab` arrive as **one event with a two-character
`input`**. Any cursor/insert model must handle multi-character `input`, not assume one keypress =
one character. **[SRC]**

### 5.3 Lone `Esc` vs the `\x1b` prefix — a 20 ms race, and it is real

An `\x1b` at the end of the buffer is held as `pending`, not emitted. `hasPendingEscape()`
(`input-parser.js:174`) then makes `App` arm a timer (`App.js:44-45`):

```js
// Small delay to let chunked escape sequences complete before flushing as literal input.
const pendingInputFlushDelayMilliseconds = 20;
```

After 20 ms the pending bytes are flushed as one event (`App.js:164-174`), so a lone `Esc` decodes
to `{escape: true}` **20 ms late**, and potato quits / backs out (`App.tsx:342,471,557`).

The residual risk: if the terminal or pty splits `\x1b` from the rest of a movement sequence by more
than 20 ms, Ink flushes `\x1b` alone — **potato quits** — and the remainder (`[D`, or `b` from
Ghostty's `esc:b`) arrives as a separate event. `[D` is plain text that `isPrintable`
(`App.tsx:162`) accepts, so it would be *typed into the search query*. In practice a keypress
arrives in one pty read, so this is a tail risk, not an everyday one — but it is exactly the
"fragile timing related hack" the Kitty spec names, and enabling `disambiguateEscapeCodes` removes
it: `Esc` becomes `\x1b[27u`, a complete sequence with no pending state.

Note this also means **Ghostty's `esc:b` for opt+← is on the wrong side of that race**: it is a
two-byte sequence starting with `\x1b`, so opt+← in Ghostty is subject to the same 20 ms window as
`Esc`, whereas a `CSI`-form arrow is not.

### 5.4 Three potato-specific hazards in `bunSafeStdin`

1. **`setEncoding` is a no-op, so UTF-8 can be corrupted before Ink sees it.** Ink calls
   `stdin.setEncoding('utf8')` (`App.js:217`); `stdin.ts:25` makes that a no-op and `pump` instead
   does `chunk.toString('utf8')` per chunk (`stdin.ts:38`). A multi-byte codepoint split across two
   `data` chunks is therefore decoded independently and becomes U+FFFD — Ink's `pending` buffer
   cannot repair it because the damage happens upstream. Node's real `setEncoding` uses a
   `StringDecoder` that holds partial sequences. This only bites non-ASCII input: accented
   characters, emoji, and the `⌥`-produced Unicode you get when `macos-option-as-alt` is `false`.
2. **A screen swap discards a pending partial escape.** `setRawMode` is a deliberate no-op
   (`stdin.ts:30`), but Ink's ref-counting still runs: when the last `useInput` unmounts,
   `handleSetRawMode` → `clearInputState()` → `inputParser.reset()` **and**
   `detachReadableListener()` (`App.js:129-133,236-248`). `reset()` clears `pending`. potato swaps
   screens on `Enter`/`Esc`/`^A`/`^E`/`^D`, i.e. exactly one React commit after an input event, so a
   half-arrived escape sequence at that instant is dropped. Bytes already in potato's `queue`
   survive (only `pending` is cleared) but are delivered late — the next `'readable'` emission,
   i.e. the next keypress.
3. **Kitty auto-detection is inert** — see §1.4. `mode: 'auto'` will never enable the protocol
   through the fake stream; only `mode: 'enabled'` works.

---

## 6. Verdict: what is and is not receivable

### 6.1 NOT receivable (design around these)

| Key | Why | Tag |
| --- | --- | --- |
| **cmd+↑ / cmd+↓ in Ghostty** | Ghostty's default `super+arrow_up/down=jump_to_prompt` consumes them. The app never sees a byte. | **[SRC]** |
| **cmd+Enter in Ghostty** | default `super+enter=toggle_fullscreen`. | **[SRC]** |
| **cmd+shift+Enter in Ghostty** | default `super+shift+enter=toggle_split_zoom`. | **[SRC]** |
| **shift+Enter anywhere without the Kitty protocol** | legacy encoding has no modifier channel for Return. Identical to `\r`. | **[SPEC]** |
| **cmd+← / cmd+→ *as arrows* in Ghostty** | Ghostty sends `\x01`/`\x05`. Receivable as *bytes*, but they arrive as ctrl+a/ctrl+e with no arrow flag — you cannot recover "cmd + Left" from them. | **[SRC]** |
| **opt+← / opt+→ *as arrows* in Ghostty** | Ghostty sends `\x1bb`/`\x1bf`. Arrives as `meta` + `input 'b'`/`'f'`, no arrow flag. | **[SRC]** |
| **cmd vs opt on any arrow, without Kitty flags 1+2** | Ink's legacy path folds alt(2) and meta(8) into one `key.meta`. | **[SRC]** |

### 6.2 Receivable, and how

- **Word motion (opt+←/→) in Ghostty at defaults: yes** — but as `key.meta && input === 'b' | 'f'`,
  not as an arrow event. This is the readline/emacs convention Ghostty deliberately ships, and it
  matches what `bash`/`zsh` expect. A keymap keyed on `{meta, input: 'b'/'f'}` gets word motion for
  free in Ghostty with no config. **[SRC]**
- **Line bounds (cmd+←/→) in Ghostty at defaults: yes** — but as `ctrl+a` / `ctrl+e`, again the
  readline convention. Accepting `ctrl+a`/`ctrl+e` as line-start/line-end is the compat-floor-safe
  way to make cmd+arrow work, and it requires moving potato's `^A` (add) and `^E` (edit) off those
  bytes. **[SRC]**
- **`cmd+Backspace` in Ghostty: yes**, as `ctrl+u` (kill line). Free, no collision today. **[SRC]**
- **opt+Backspace: probably yes**, as `{backspace, meta}` from `\x1b\x7f`. Ink decodes that form
  explicitly. **[CONFIRM]** on the bytes.
- **fn+←/→ (Home/End): probably yes**, as `key.home` / `key.end`. Ink 7 exposes both and accepts all
  three legacy spellings. **[CONFIRM]** on which spelling and on Terminal.app not scrolling instead.
- **shift+Enter: only with the Kitty protocol**, as `{return, shift}` from `\x1b[13;2u`, and only in
  Ghostty. Requires `render(..., {kittyKeyboard: {mode: 'enabled', flags: [...]}})` because
  auto-detect is inert (§1.4). **Terminal.app can never have it** — so multi-line authoring needs a
  non-shift+Enter fallback (e.g. `ctrl+j`, which is `\n` and decodes to `name: 'enter'`, distinct
  from `name: 'return'` — `parse-keypress.js:414-423`; note `useInput` has **no** `key.enter` field,
  only `key.return`, so `\n` arrives as `input === '\n'` with *no* flags set). **[SRC]**

### 6.3 Rows awaiting human confirmation

Run `scripts/keycapture.ts` (§7) once in Ghostty and once in Terminal.app. These are the open rows:

**Ghostty (6 rows)**

1. opt+↑ and opt+↓ — expected `\x1b[1;3A` / `\x1b[1;3B`.
2. opt+Backspace — expected `\x1b\x7f`.
3. fn+← / fn+→ — which of `\x1b[H`/`\x1bOH`/`\x1b[1~` and `\x1b[F`/`\x1bOF`/`\x1b[4~`.
4. Whether plain arrows arrive as `\x1b[D` (normal) or `\x1bOD` (DECCKM).
5. Under Kitty flags 1+2: whether Ghostty emits the `:1` press sub-parameter on modified arrows
   (`\x1b[1;3:1A`), i.e. whether `key.super` / `key.eventType` are populated at all.
6. Whether ⌘ maps to Kitty `super` (bit 8) rather than `meta` (bit 32) in Ghostty's encoder.

**Terminal.app (all 9 rows of §4.3, plus)**

7. Whether Terminal.app answers the `CSI ? u` Kitty probe (the script reports this).
8. The factory default of "Use Option as Meta key".
9. Whether cmd+arrow / cmd+Backspace are swallowed by a menu shortcut or simply do nothing.

**Ghostty, minor**

10. That `escape=end_search` really is overlay-scoped and `Esc` reaches the app (§2.3).

---

## 7. The capture script

`scripts/keycapture.ts`, run with `bun scripts/keycapture.ts`. It puts stdin in raw mode itself
(the same "raw mode first, read via `data`" order `src/tui/stdin.ts` uses), prints for each chunk:

- the raw bytes as hex and as a C-escaped string,
- the events Ink's own `createInputParser` splits it into,
- the `keypress` object Ink's own `parseKeypress` produces,
- the `key` object `useInput` would build from it — reusing Ink's real modules, not a reimplementation,
- and a verdict line saying whether potato's `isPrintable` / `isBackspace` / `isCtrl` would fire.

`--kitty` enables the Kitty protocol (`CSI > 3 u`, i.e. `disambiguateEscapeCodes | reportEventTypes`)
so the §4.2 rows can be checked in the same sitting. On start it probes with `CSI ? u` and reports
whether the terminal replied, which settles open row 7.

### Keys to press, in order

The script prints this list and numbers each capture, so the transcript lines up. Press exactly
these, one at a time:

```
 1. Left            2. Right           3. Up              4. Down
 5. opt+Left        6. opt+Right       7. opt+Up          8. opt+Down
 9. cmd+Left       10. cmd+Right      11. cmd+Up         12. cmd+Down
13. Enter          14. shift+Enter    15. ctrl+J
16. Backspace      17. opt+Backspace  18. cmd+Backspace
19. fn+Left        20. fn+Right
21. Escape         22. Escape (again, to confirm the 20 ms flush)
23. ctrl+A         24. ctrl+E         25. ctrl+U
26. the letter b   27. opt+b
```

Rows 23–25 exist to prove the §4.1 collision: 9/10/18 must produce *the same* output as 23/24/25 in
Ghostty. Row 27 checks `macos-option-as-alt`. Then `ctrl+C` (or `q`) to exit.

Do a second pass with `bun scripts/keycapture.ts --kitty` in Ghostty for rows 5–14 and 17–22.
