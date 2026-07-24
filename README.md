# 🥔 potato

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
- `potato import <file|->` — merge someone else's library (yours wins; `--theirs` to overwrite)
- `potato update` / `potato uninstall` — self-explanatory; uninstall keeps your data unless you `--purge`

Your library is one hand-editable JSON file, `~/.potato/commands.json` — copying it *is* exporting.

## Develop

```sh
bun install
bun start            # run the TUI from source
bun test             # test suite
bun run typecheck
bash scripts/build.sh 1.0.0   # compile all four release targets + SHA256SUMS
```

`bun start` runs the bare TUI — Enter prints the selection instead of pre-filling your prompt, because the pre-fill lives in the installed shell wrapper, and a child process can't touch its parent's prompt. To test the real hand-off, paste a dev wrapper into your zsh:

```zsh
pdev() {
  local out cmd
  out="$(mktemp)" || return 1
  bun run src/cli.tsx --out "$out"
  cmd="$(cat "$out")"; rm -f "$out"
  [ -n "$cmd" ] && print -z -- "$cmd"
}
```

Or exercise the full installed pipeline (compiled binary + generated glue), isolated from your real `~/.potato`:

```zsh
export POTATO_INSTALL=/tmp/potato-dev
mkdir -p $POTATO_INSTALL/bin
bun build --compile src/cli.tsx --outfile $POTATO_INSTALL/bin/potato
$POTATO_INSTALL/bin/potato init zsh > $POTATO_INSTALL/init.zsh
source $POTATO_INSTALL/init.zsh
potato   # the wrapper function, exactly as installed users get it
```

Clean up with `unset -f potato` and `unset POTATO_INSTALL`.

The spec this implements — and the decision trail behind it — lives in [docs/spec/potato-v1.md](docs/spec/potato-v1.md).
