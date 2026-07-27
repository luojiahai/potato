# 🥔 Potato

A TUI for saving, fuzzy-finding, and handing off long terminal commands. Potato never executes anything — it renders a command and hands it to your shell prompt (Enter) or your clipboard (Ctrl-Y).

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/luojiahai/potato/main/install.sh | bash && source ~/.potato/init.sh
```

macOS and Linux (x64/arm64), zsh and bash. No sudo, no PATH edits — everything lives under `~/.potato`.

## Use

Type `potato`. Fuzzy-find a command, press **Enter** — the rendered command lands pre-filled at your prompt for review; press Enter again to actually run it. **Ctrl-Y** copies it instead (works over SSH and in tmux via OSC 52 — in tmux, enable it with `set -s set-clipboard on` in `~/.tmux.conf`).

Commands are templates: `ssh {{host=prod-1}} 'deploy.sh'` prompts for `host` (pre-filled with your last value, then the default) with a live preview before hand-off.

- **Ctrl-A** add · **Ctrl-E** edit · **Ctrl-D** delete, all in-app
- `potato import <file|->` — merge someone else's library in; on a name clash both are kept (the incoming one renamed `name (N)`). `--override` replaces yours wholesale instead.
- `potato update` / `potato uninstall` — self-explanatory; uninstall keeps your data unless you `--purge`

Your library is one hand-editable JSON file, `~/.potato/commands.json` — copying it *is* exporting.

## Develop

Go 1.26+. The TUI is [Bubble Tea](https://github.com/charmbracelet/bubbletea) — see [ADR-0001](docs/adr/0001-bubble-tea-over-ink.md).

```sh
go run ./cmd/potato     # run the TUI from source
go test ./...           # test suite
go vet ./... && gofmt -l .
bash scripts/build.sh 1.0.0   # compile all four release targets + SHA256SUMS
```

`go run ./cmd/potato` runs the bare TUI — Enter prints the selection instead of pre-filling your prompt, because the pre-fill lives in the installed shell wrapper, and a child process can't touch its parent's prompt. To test the real hand-off, paste a dev wrapper into your zsh:

```zsh
pdev() {
  local out cmd
  out="$(mktemp)" || return 1
  go run ./cmd/potato --out "$out"
  cmd="$(cat "$out")"; rm -f "$out"
  [ -n "$cmd" ] && print -z -- "$cmd"
}
```

Or exercise the full installed pipeline (compiled binary + generated glue), isolated from your real `~/.potato`:

```zsh
export POTATO_INSTALL=/tmp/potato-dev
mkdir -p $POTATO_INSTALL/bin
go build -o $POTATO_INSTALL/bin/potato ./cmd/potato
$POTATO_INSTALL/bin/potato init zsh > $POTATO_INSTALL/init.zsh
source $POTATO_INSTALL/init.zsh
potato   # the wrapper function, exactly as installed users get it
```

Clean up with `unset -f potato` and `unset POTATO_INSTALL`.
