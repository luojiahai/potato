# <img src="docs/media/brand/potato-app-icon.svg" alt="" width="32" height="32"> Potato

Potato keeps the long terminal commands you can never remember, finds them by fuzzy search, and hands them back. It never runs anything itself. Enter drops the finished command at your shell prompt so you can read it before you commit to it, and Ctrl-Y puts it on your clipboard instead.

![The potato list screen, showing a search field, seven saved commands, and a detail strip for the selected one](docs/media/list.png)

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/luojiahai/potato/main/install.sh | bash && source ~/.potato/init.sh
```

macOS and Linux on x64 or arm64, with zsh and bash 4 or newer. There is no sudo and no PATH edit: everything lands under `~/.potato`, and the installer adds one line to your shell rc. To pin a release instead of taking the latest, append `-s -- 1.0.0` to the `bash`. Older bash prints the command instead of pre-filling it, since the readline hook the pre-fill needs is not there.

## Use

Run `potato`. Type to narrow the list, then press Enter. The command arrives pre-filled at your prompt, where a second Enter runs it. Nothing executes until you say so.

Potato draws a fixed block of rows under the prompt you launched it from rather than taking over the screen, and erases them on the way out. What is left in your scrollback is the command, not the picker you found it with.

Ctrl-Y copies rather than hands off. It writes through a system clipboard tool when there is one and always emits OSC 52 as well, so it still works over SSH and inside tmux. Tmux needs `set -s set-clipboard on` in `~/.tmux.conf` to pass it through.

Search is a fuzzy subsequence match across each command's name, description, and text, ranked in that order — a name hit outranks a description hit, which outranks one in the command itself — and the letters that matched are picked out in the name. An empty query lists what you used most recently, and anything never used follows in file order.

A row carries the command's name, a dim preview of the command itself, and at the right `⌁N` for the number of placeholders it will ask you to fill in, then how long ago you last used it. The strip below the list opens the selected one out in full.

### Keys

The list screen has two keyboard zones and Tab moves between them. The search field keeps every readline key, including Ctrl-A and Ctrl-E, which is also what your terminal sends for Cmd-Left and Cmd-Right. That is why the verbs live in the other zone, where a bare letter is free to mean something.

| Search field | | Command list | |
|---|---|---|---|
| `↵` | run | `↵` | run |
| `^Y` | copy | `y` | copy |
| `↑` `↓` | move the selection | `↑` `↓` / `k` `j` | move the selection |
| `tab` | go to the list | `a` `e` `d` | add, edit, delete |
| `esc` | quit | `tab` / `esc` | back to the search field |

Adding and editing happen in the app. Nothing sends you out to `$EDITOR`. Ctrl-C leaves from anywhere without handing anything over, and a delete asks first, on the row itself, with the strip still showing what you are about to lose.

### Placeholders

Commands are templates. Write `ssh {{host=prod-1}} 'deploy.sh'` and potato asks for `host` before handing anything over, showing the rendered result as you type. Each field starts from the value you used last time for that command, falling back to the default in the template.

![The argument form, with two placeholders filled from last-used and default values, above a live preview of the command that will run](docs/media/arguments.png)

A command that already contains newlines, whether hand-written into the JSON or imported, is stored and handed off exactly as it is; the list preview flattens it to one row. The add and edit fields themselves are single-line, and Enter there saves.

### Your library

Your commands live in one hand-editable JSON file, `~/.potato/commands.json`. Copying that file is how you share or back up a library; there is no separate export. Beside it, `~/.potato/state.json` caches last-used times and last argument values. It is disposable, and it never travels with the library.

```json
{
  "version": 2,
  "commands": [
    {
      "id": "0f6d2a1e-8c3b-4a5f-9d70-2b1c4e8a6f3d",
      "name": "deploy web",
      "description": "Roll out the web tier",
      "command": "ssh {{host=prod-1}} 'cd /srv/web && git pull && systemctl restart web'"
    }
  ]
}
```

`id` is what potato tracks a command by, so a rename keeps its history; it only has to be a non-empty string, unique in the file. `name` must be unique too — the edit form refuses a clash rather than shadowing the command you already had. `description` is optional, and any field potato does not know about is carried through untouched. A file potato cannot parse stops it with the reason rather than being overwritten, and the writes it does make go through a temp file and a rename, so a failed one leaves the library you had.

```sh
potato import <file|->              # merge another library into yours. On a name
                                    # clash both survive, and the incoming one
                                    # becomes `name (N)`
potato import <file|-> --override   # replace your library with the file instead
potato update                       # fetch and install the latest release
potato uninstall [--purge]          # remove potato, keeping your data unless --purge
```

## Develop

Go 1.26.5 or newer. The TUI is built on [Bubble Tea](https://github.com/charmbracelet/bubbletea).

```sh
go run ./cmd/potato     # run the TUI from source
go test ./...           # test suite
go vet ./... && gofmt -l .
bash scripts/build.sh 1.0.0   # compile all four release targets and SHA256SUMS
```

Run from source and Enter prints the selection rather than pre-filling your prompt. The pre-fill lives in the shell wrapper that the installer generates, and no child process can write to its parent's prompt. To exercise the real hand-off, paste this into your zsh:

```zsh
pdev() {
  local out cmd
  out="$(mktemp)" || return 1
  go run ./cmd/potato --out "$out"
  cmd="$(cat "$out")"; rm -f "$out"
  [ -n "$cmd" ] && print -z -- "$cmd"
}
```

Screen layouts are pinned by the golden frames in `testdata/frames`, and the generated shell glue by `testdata/init`. Regenerate the frames with `go test ./internal/tui -update-frames` after a deliberate visual change, then read the diff. To try a real install without touching your own `~/.potato`, point `POTATO_INSTALL` at a throwaway directory; `internal/shell` documents the rest.
