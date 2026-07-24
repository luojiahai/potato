# PROTOTYPE — potato TUI layout & interaction flow

**Throwaway code.** Answers wayfinder ticket
[#6 TUI layout & interaction flow prototype](https://github.com/luojiahai/potato/issues/6):
what does potato look and feel like? This is an asset to react to, not the implementation.

## Run it

```sh
cd prototype-tui
bun install
bun start
```

## What's in it

All state is in memory — nothing touches `~/.potato`. Seeded with realistic commands,
some with `{{arg}}` placeholders.

- **List + fuzzy search** — type to filter (name > description > command), empty query = MRU order.
- **Three list layout variants**, cycled with `Ctrl-V`:
  - **A — compact**: fzf-style single-line rows, maximum density.
  - **B — two-line**: name + description, command underneath.
  - **C — split**: name list on the left, detail pane (description, full command, args) on the right.
- **Persistent footer bar** — Enter = run, Ctrl-Y = copy, plus CRUD keys.
- **Arg-prompt flow** — Enter on a command with `{{placeholders}}` opens a single form screen
  with live preview; pre-fill is last-value > default. Enter runs, Ctrl-Y copies, from any field.
- **Add / edit / delete** — Ctrl-A / Ctrl-E / Ctrl-D from the list.

"Run" exits and prints the command it *would* pre-fill into your shell (the real hand-off is
already decided — zle/READLINE widgets). "Copy" emits a real OSC 52 sequence, so it may
actually reach your clipboard in a supporting terminal.
