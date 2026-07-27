# ADR-0001: Bubble Tea over Ink

**Status:** accepted · **Date:** 2026-07-27

## Context

potato was a TypeScript TUI on Bun, React 19 and Ink 7 — about 1,300 lines of
`src/` plus a 960-line test suite, compiled to a single binary with
`bun build --compile`.

Four things pushed against that stack at once:

1. **Text input.** [Map #34](https://github.com/luojiahai/potato/issues/34)
   spent eighteen decision tickets designing a cursor, a macOS movement set and
   multi-line Commands, producing [PRD #56](https://github.com/luojiahai/potato/issues/56).
   Its largest section specifies four modules — `keys.ts`, `text.ts`,
   `wrap.ts`, `display.ts` — that exist to give Ink something it does not have.
   Along the way the map had to reverse-engineer Ink's input model out of
   `node_modules`: no key propagation, a 20 ms escape flush, `input` blanked for
   non-alphanumeric keys.
2. **Runtime.** A Bun-compiled potato is 50–90 MB and pays a Node-shaped cold
   start on a command invoked many times a day.
3. **The stack.** A React reconciler and a hand-written fake stdin stream
   (`src/tui/stdin.ts`, sixty lines of Bun raw-mode workarounds) to run a
   fuzzy-finder.
4. **Ecosystem.** Charm is where terminal-UI craft is being done.

## Decision

Rewrite potato in Go on **Bubble Tea v2**, dropping Ink, React, TypeScript and
Bun entirely.

- `charm.land/bubbletea/v2` · `charm.land/lipgloss/v2` · `charm.land/bubbles/v2`
  · `github.com/google/uuid`. Go 1.26; CI pinned to 1.26.5.
- `cmd/potato` plus `internal/` packages mirroring the old modules
  (`import` → `importer`, forced by the keyword).
- `bubbles/textinput` on all five fields, with the search field's `LineStart`
  and `LineEnd` unbound so `^A` and `^E` still mean *add* and *edit* on the list
  screen. That is PRD #56 §4.1's resolution, arriving early and for free.
- Everything else is hand-rendered from Lipgloss primitives.

### The bar: strict parity with the Ink build

The port reproduces `main` as it stood, including its known bugs
([#49](https://github.com/luojiahai/potato/issues/49),
[#52](https://github.com/luojiahai/potato/issues/52),
[#55](https://github.com/luojiahai/potato/issues/55)) — those are fixed
separately, not smuggled into a rewrite.

Parity is enforced, not asserted: fourteen frames were captured from the Ink
build before it was deleted (`testdata/parity/`), and `internal/tui`'s tests
assert the Go `View()` reproduces them.

## Consequences

### Accepted differences

Adopting `textinput` and Bubble Tea's message loop changes four visible things:

- fields gain a real, blinking cursor, replacing the decorative `▌` drawn at
  the end of the focused value;
- fields gain word motion, `^U`, `^W`, `^K` and `ctrl+v` paste;
- `^A`/`^E` mean line-start and line-end *inside* the arg and edit screens
  (they remain add/edit on the list screen);
- the layout reflows on terminal resize, which the Ink build never did — it
  read `stdout.columns` once with no listener.

The parity harness normalises exactly one cell for this: `▌` in the goldens is
compared against the column the real cursor now occupies. Everything else must
match byte for byte.

### Two frames the port deliberately does not reproduce

At `19×80` and `24×50` Ink's flexbox ran out of room and rendered something
broken — the wordmark clipped to five rows with the detail panel's title and
last line drawn into its own borders; the footer's key hints squeezed onto two
rows with `↵ run` truncated to `↵ ru`. Reproducing them means emulating Yoga's
flex-shrink over inline text. The Go build renders both correctly instead.
These are listed in `inkDegenerate` in `internal/tui/parity_test.go`, which
fails if the difference ever disappears.

Note this incidentally fixes #55's *symptom* at 19 rows while faithfully
reproducing its *cause* at 24 rows, where the selection window is computed one
row larger than the panel that clips it.

### Storage

Go's `encoding/json` differs from `JSON.stringify` in ways that reach the
user's file, so `internal/library` writes JSON by hand:

- **HTML escaping is off.** `json.Marshal` writes `&&` as `\u0026\u0026`, and
  `>` as `\u003e`, which would rewrite most Libraries into unreadable escapes
  on the first save.
- **Unknown fields are preserved** at both the Library and the entry level via
  `Extra map[string]json.RawMessage`. A plain struct would silently delete
  someone's forward-compatible field — the one loss that cannot be undone.
- **Key order is normalised** to `id, name, description, command` then unknown
  keys sorted, where `JSON.stringify` emitted the order it parsed. Order is
  promised nowhere; the cost is a one-time diff on first save.

`state.json`'s map keys are likewise sorted rather than insertion-ordered.

### Release compatibility — do not change these names

Installed v0.1.x binaries self-update by resolving
`potato-{darwin,linux}-{x64,arm64}.tar.gz` against `SHA256SUMS`, and
`install.sh` derives the same four names from `uname`. Go's arch is `amd64`;
it is published as `x64`. **Renaming an asset strands every installed binary.**
`install.sh` needed no edit at all.

`potato init` output is likewise frozen: the installer regenerates the shell
glue from the binary on every install *and* every update, so drift would
silently rewrite every user's rc hook. `testdata/init/` holds the three scripts
captured from the Ink build, and a test diffs them.

### The rest

- `src/tui/stdin.ts` and the `react-devtools-core` stub cease to exist —
  Bubble Tea owns raw mode.
- `potato update` no longer shells out to `tar`; it uses `archive/tar`.
- The binary drops from ~50–90 MB to roughly 8 MB.
- `CONTEXT.md` needed no edit: it never named the stack.

## Alternatives rejected

- **Stay on Ink and implement PRD #56 there.** It means building the four text
  modules and a caret overlay — the largest and hardest work in the PRD, and
  precisely what `bubbles/textarea` and Lipgloss provide.
- **`bubbles/list`** for the list screen. It brings its own filtering,
  pagination, status bar and help, all of which would be overridden back to
  potato's per-character match highlighting, `⌁{n}` badge, `❯` pointer,
  inverse-video row and MRU ordering.
- **`sahilm/fuzzy`** (already in the module graph via `bubbles`). potato's
  scorer is bespoke — `+3` for a consecutive run, a short-target bias, then
  `×100`/`×10`/`×1` by field — and any other ranking visibly reorders the list.
- **`atotto/clipboard`** (likewise transitively present). It does not emit
  OSC 52, which is the path the README advertises for SSH and tmux.
- **goreleaser.** It wants to own archive naming and layout, and both
  `install.sh` and `internal/update` depend on that layout exactly.

## Supersedes

PRD [#56](https://github.com/luojiahai/potato/issues/56) is **half superseded**
and has not been edited. Its policy survives: §2 vocabulary, §4.1 the `^A`/`^E`
resolution, §4.3 the whitespace-only word rule, §5 the Continuation semantics,
§7 storage, §8.3–§8.6 rendering and budgets, §9 hand-off, §11 out-of-scope.

Its Ink-bound sections do not: **§3** (the `apply` reducer, `'pass'`, `caps`,
the four modules — Bubble Tea *is* that reducer), **§4's predicate column**
(`key.meta && input === 'b'` becomes `tea.KeyPressMsg` matching), **§6's
mechanism** (`usePaste` becomes Bubble Tea's native bracketed paste; the filter
table survives), **§8.1–§8.2's measurement** (`string-width@8.2.0` and the
`Bun.stringWidth` rejection become "agree with Lipgloss"), **§12's test plan**,
and **§15's order of work**.

One section flips outright: §11 rules the Kitty keyboard protocol out of scope
*because Ink cannot decode it*. Bubble Tea v2 negotiates keyboard enhancements
natively, which may make §13's headline accepted limit — no word-delete on a
stock Mac — not a limit at all. That reopens §10's terminal-parity table.

Anyone picking up #56 should re-derive those sections before writing code.
