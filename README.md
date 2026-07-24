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

The spec this implements — and the decision trail behind it — lives in [docs/spec/potato-v1.md](docs/spec/potato-v1.md).
