# Potato v1 — Build Spec

Potato is a TUI for saving, fuzzy-finding, and handing off long terminal commands. It never executes anything — it renders a command and hands it to the parent shell's prompt (Enter) or the clipboard (Ctrl-Y).

This spec is the destination of the [potato v1 wayfinder map](https://github.com/luojiahai/potato/issues/1). Every section links the ticket that decided it; the tickets hold the full rationale and rejected alternatives. Vocabulary follows `CONTEXT.md` (Command, Library, Placeholder, Hand-off, State, Import, Collision).

**Stack:** TypeScript + Ink 7 (React TUI), compiled to standalone binaries with Bun. No Node runtime assumed on user machines.

---

## 1. Data model

_Decided in [Command data model & JSON schema](https://github.com/luojiahai/potato/issues/5) and [Arg placeholder syntax & prompting UX](https://github.com/luojiahai/potato/issues/4)._

### 1.1 The Library — `~/.potato/commands.json`

The user's shareable, hand-editable Library:

```json
{
  "version": 1,
  "commands": {
    "deploy prod": {
      "command": "ssh {{host=prod-1}} 'deploy.sh'",
      "description": "Roll out to production"
    }
  }
}
```

- **Identity: name as key.** `commands` is an object keyed by unique Command name; the name is the command-id everywhere (including State). Renaming a Command orphans its State entry — accepted; State is just a pre-fill cache.
- **Name rules:** any non-empty string after trimming, case-sensitive, no length cap or charset restriction. The list view truncates for display.
- **Entry fields:** `command` (required — the template string with inline Placeholders) and `description` (optional). Nothing else; no timestamps.
- **Version envelope:** top-level `{ "version": 1, ... }`. Unknown or missing version → fail loud. No migration machinery in v1; the field is the hook that makes future migrations deterministic.

**Robustness — fail loud.** Any parse or validation error (bad JSON, missing/empty `command`, bad version) refuses to start, reporting file + reason. Potato never writes to a file it couldn't parse. Unknown extra fields are tolerated and preserved on save. Saves rewrite pretty-printed (2-space indent), preserving key order, with new entries appended — file order is meaningful (it's the never-used sort order, under the hand-editor's control).

### 1.2 State — `~/.potato/state.json`

Disposable per-Command cache, never shared or imported:

```json
{
  "deploy prod": {
    "lastUsedAt": "2026-07-24T09:12:00Z",
    "args": { "host": "prod-2" }
  }
}
```

`lastUsedAt` drives MRU ordering; `args` holds the last value supplied for each Placeholder. Unreadable State → silently reset to `{}`.

## 2. Placeholders

_Decided in [Arg placeholder syntax & prompting UX](https://github.com/luojiahai/potato/issues/4)._

- Syntax: `{{name}}` or `{{name=default}}`, where `name` matches `[A-Za-z0-9_-]+`.
- Anything else — `{print $1}`, `${HOME}`, `{{ spaced }}`, `{1}` — stays literal. No escape syntax in v1.
- A repeated name prompts once and fills every occurrence; on conflicting defaults, the first wins.
- **Substitution is verbatim**, character-for-character. Template authors add their own quoting (`git commit -m "{{msg}}"`). Empty values are allowed and render as empty string — the live preview and the shell-prompt pre-fill are the review points, so there is no validation state.
- Per-Placeholder metadata is exactly: static default (inline in the template) and remembered last value (in State). Pre-fill precedence: **last value > default > empty**.

## 3. TUI

_Decided in [TUI layout & interaction flow prototype](https://github.com/luojiahai/potato/issues/6); reference implementation on branch [`prototype/tui-layout`](https://github.com/luojiahai/potato/tree/prototype/tui-layout), directory `prototype-tui/` (layout variant C)._

### 3.1 List screen — split pane

- **Left:** Command name list with fuzzy-search input.
- **Right:** detail pane for the highlighted Command — description, full command template, Placeholders with defaults.
- **Persistent footer bar:** `Enter` run · `Ctrl-Y` copy · `Ctrl-A` add · `Ctrl-E` edit · `Ctrl-D` delete.

**Fuzzy search** matches over name + description + command text; name hits weighted highest, then description, then command. **Empty query:** most-recently-used first (via `lastUsedAt` in State), never-used Commands follow in file order.

### 3.2 Arg form

Selecting a Command whose template has Placeholders (via Enter *or* Ctrl-Y) opens a single form screen:

- One field per Placeholder, pre-filled per the precedence in §2.
- Tab/↑↓ move between fields; Esc returns to the list.
- Live-rendered preview of the command as you type, substituted values highlighted.
- From any field: **Enter = run** the rendered command (Hand-off, §4), **Ctrl-Y = copy** the rendered command. Copy never yields a raw template with holes — Hand-off is always a runnable command.

### 3.3 CRUD

In-app add (`Ctrl-A`), edit (`Ctrl-E`), delete (`Ctrl-D` with a confirm screen), operating on the Library file per §1.1's save rules.

### 3.4 Implementation notes (validated in the prototype)

- Ink 7.1.1 renders and takes raw-mode input correctly under Bun in a pty.
- Enter must arrive as its own keypress event; chunked (paste-like) input does not trigger run — fine for interactive use.
- With the temp-file Hand-off (§4), the binary renders on stdout as a completely normal Ink app — no stderr split, no forced-color workaround. (The prototype's `FORCE_COLOR` / stderr-render findings applied only to the superseded stdout-split design.)

## 4. Hand-off

_Decided in [Command hand-off: run in parent shell + copy to clipboard](https://github.com/luojiahai/potato/issues/3), as amended by [Install script UX & lifecycle](https://github.com/luojiahai/potato/issues/9)._

Potato's only output path. A child process can never mutate its parent shell, so Hand-off is mediated by an installed wrapper function (§6.2).

### 4.1 Enter — pre-fill the parent prompt

**Temp-file protocol:** the wrapper function `mktemp`s a file and invokes the binary with `--out <file>`. The binary runs the TUI normally on stdout; on exit it writes the selected rendered command to the file — **empty file = cancelled**. The wrapper reads the file, deletes it, and if non-empty pre-fills the prompt:

- **zsh:** `print -z -- "$cmd"` (pushes onto the editing buffer stack; next prompt arrives pre-filled).
- **bash:** a `bind -x` widget setting `READLINE_LINE` / `READLINE_POINT` (bash ≥ 4).

Enter never executes — the command lands pre-filled for review; the user presses Enter again in their own shell to run it.

### 4.2 Ctrl-Y — copy

Handled inside the TUI, from the list or the arg form; always copies the rendered command:

- Spawn the native clipboard tool if present (`pbcopy` / `wl-copy` / `xclip`), **and**
- always emit OSC 52 (`ESC ] 52 ; c ; <base64> BEL`) to the terminal — the only mechanism that works over SSH and inside tmux (document `set -s set-clipboard on` for tmux).

## 5. CLI surface

- `potato` — the TUI (§3). With `--out <file>`, writes the selection per §4.1 (how the wrapper invokes it).
- `potato import <file>` — §7. `-` reads stdin.
- `potato update` — §8.
- `potato uninstall` — §8.
- `potato init zsh|bash` — plumbing that prints the shell glue; invoked only by install.sh and `potato update`, never by users or rc files.

## 6. Packaging, release, and install

### 6.1 Packaging & release

_Decided in [Packaging potato for curl|bash install without Node preinstalled](https://github.com/luojiahai/potato/issues/2); full findings in [docs/research/packaging-curl-bash-install.md](https://github.com/luojiahai/potato/blob/research/packaging/docs/research/packaging-curl-bash-install.md)._

- **`bun build --compile`** produces standalone binaries; Ink 7 + React 19 verified working. One required workaround: stub Ink's optional `react-devtools-core` import via a tsconfig `paths` alias.
- All four targets — macOS/Linux × x64/arm64 — cross-compile from one machine via `--target` (61–91 MB raw, 23–34 MB gzipped).
- **Release:** GitHub Releases. CI on tag push: install Bun, loop the 4 targets, tar.gz each as `potato-<target>.tar.gz`, `shasum -a 256 > SHA256SUMS`, `gh release create`. No matrix, no signing.

### 6.2 Install script

_Decided in [Install script UX & lifecycle](https://github.com/luojiahai/potato/issues/9)._

**Advertised install command** (zero manual activation — the trailing `source` runs in the user's own shell, gated by `&&`):

```sh
curl -fsSL https://…/install.sh | bash && source ~/.potato/init.sh
```

**install.sh behavior** (starship-style: `set -euo pipefail`, `uname -sm` case-map with Rosetta 2 detection, optional version arg, download from `releases/latest/download/`, **sha256-verified against `SHA256SUMS`**):

1. **Location:** binary at `~/.potato/bin/potato`; potato's entire footprint lives under `~/.potato`. `POTATO_INSTALL` overrides. No sudo, ever.
2. **Shell glue — no PATH edit.** Generate static init files from the binary: `potato init zsh > ~/.potato/init.zsh`, likewise `init.bash`, plus a ~5-line `~/.potato/init.sh` dispatcher that sources the right one via `$ZSH_VERSION`/`$BASH_VERSION`. The glue defines a wrapper **function named `potato`** invoking the binary by absolute path — the function *is* the command, which is why PATH is never touched — plus the pre-fill mechanics of §4.1. Static files also avoid spawning the 60–90 MB binary on every shell startup, which an `eval "$(potato init …)"` line would incur.
3. **rc edit:** append exactly one line — `source ~/.potato/init.zsh` (or `.bash`) — to the login shell's rc, targeting `$SHELL` only: zsh → `~/.zshrc`, bash → `~/.bashrc` (created if missing). Unsupported shells: binary + init files install, no rc edited, the line printed for manual placement. macOS + bash: messaging notes `~/.bash_profile` must source `~/.bashrc`; we never edit `.bash_profile`.
4. **Idempotency:** re-runs always overwrite the binary and regenerate init files; the rc line is appended only if a grep for `.potato/init.` finds nothing; `commands.json` / `state.json` are never touched.
5. **Messaging:** platform detected, version installed, files written, then "If potato isn't available in this shell, run: `source ~/.potato/init.sh`". Re-runs where the rc line already existed print just the version change.

## 7. Import

_Decided in [Import/export semantics](https://github.com/luojiahai/potato/issues/8)._

**There is no export command** — the Library file is the full-fidelity portable artifact; copying it *is* exporting.

`potato import <file>` (`-` for stdin, enabling `curl -fsSL <url> | potato import -`) is a one-shot CLI verb; no TUI surface. **Merge semantics, ours wins:**

- Incoming names not in the Library are **added**, appended in incoming-file order.
- **Collisions** that differ are skipped and reported by name; identical entries are silent no-ops. `--theirs` flips collisions to overwrite.
- Report (added / skipped names) prints to stdout; exit 0 on success even with skips.
- **Validation:** the same fail-loud loader as §1.1 — bad JSON, bad/missing/future version, or any invalid entry aborts the whole import with file + reason. All-or-nothing; no partial merges. Unknown extra fields on incoming entries are tolerated and preserved. If the user's own Library doesn't parse, import refuses to run.
- **State is untouched.** A `--theirs` overwrite may strand stale arg values; the disposable-cache rule (§1.2) already tolerates that.

## 8. Update & uninstall

_Decided in [Install script UX & lifecycle](https://github.com/luojiahai/potato/issues/9)._

- **`potato update`** — CLI-only, latest-only (pinning stays install.sh's job via its version arg). Verifies sha256 against `SHA256SUMS` with install.sh's rigor, atomic rename over the running binary's realpath (respects `POTATO_INSTALL`), regenerates init files after every swap, never touches the rc. "Already up to date" no-op. Re-running install.sh remains a valid fallback.
- **`potato uninstall`** — removes the rc line (from `.zshrc`/`.bashrc` where found), deletes `~/.potato/bin/` and the generated init files, **keeps user data** and prints where it lives. `--purge` does the full `rm -rf ~/.potato`.

## 9. Out of scope for v1

- Grouping/tags — flat searchable list only.
- Windows support.
- In-app subprocess execution — ruled out in favour of uniform Hand-off.
- Full shell-integration surface (custom keybindings, completions) — only the minimal wrapper function ships.
