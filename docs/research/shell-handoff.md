# Command hand-off: run in parent shell + copy to clipboard

Research for [issue #3](https://github.com/luojiahai/potato/issues/3), part of the map in
[issue #1](https://github.com/luojiahai/potato/issues/1).

**Question.** Potato (an Ink/React TUI picker) never executes commands itself. On Enter the
rendered command must reach the parent shell, ideally pre-filled at the prompt so the user can
review and press Enter; on Ctrl-Y it must reach the clipboard. What are the concrete mechanisms?

All claims below were checked against primary sources (official manuals, man pages, and the
source code of fzf, zoxide, atuin, and mcfly). Each claim cites its source.

---

## 1. Getting a command into the parent shell's prompt

A child process cannot write into its parent shell's input buffer directly; every tool in this
space solves it with a small shell function installed in the user's rc file that runs the picker,
captures its final output, and feeds that output to the shell's line editor. The shell-specific
half is tiny; the mechanisms differ per shell.

### 1.1 zsh: `print -z` (simple) or a zle widget setting `BUFFER` (better)

- `print -z` is documented as: "Push the arguments onto the editing buffer stack, separated by
  spaces." The buffer stack is popped into the next prompt, i.e. the text appears pre-filled and
  editable. Source: zsh manual, Shell Builtin Commands —
  <https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html>. (`print -s` similarly
  pushes into history instead of stdout — same page.)
- A zle widget is what fzf and atuin actually use, because it works mid-line and controls the
  cursor. fzf's `fzf-history-widget` captures the selection via command substitution, then sets
  `BUFFER="..."` and `CURSOR=${#BUFFER}`, registers with `zle -N fzf-history-widget`, and binds
  Ctrl-R in the emacs, vicmd and viins keymaps with `bindkey`. Source: fzf
  `shell/key-bindings.zsh` —
  <https://github.com/junegunn/fzf/blob/master/shell/key-bindings.zsh>.
- atuin's zsh widget does the same: `output=$(__atuin_search_cmd $*)`, then `RBUFFER=""` and
  `LBUFFER=$output`; it registers widgets with `zle -N atuin-search _atuin_search`. It also shows
  how "insert vs run immediately" can be signalled in-band: if the output starts with the prefix
  `__atuin_accept__:`, the widget strips the prefix and calls `zle accept-line` to execute at
  once. Source: atuin `crates/atuin/src/shell/atuin.zsh` —
  <https://github.com/atuinsh/atuin/blob/main/crates/atuin/src/shell/atuin.zsh>.

Verdict: `print -z` is fine for a plain `potato` command typed by the user; a zle widget with
`BUFFER`/`CURSOR` is the right mechanism for a keybinding, and is what the ecosystem converged on.

### 1.2 bash: `bind -x` + `READLINE_LINE` / `READLINE_POINT`

- The bash manual defines `READLINE_LINE` as "The contents of the Readline line buffer, for use
  with 'bind -x'" and `READLINE_POINT` as "The position of the insertion point in the Readline
  line buffer, for use with 'bind -x'". Source: Bash Reference Manual, Bash Variables —
  <https://www.gnu.org/software/bash/manual/html_node/Bash-Variables.html>.
- For `bind -x keyseq:shell-command` the manual states: "When shell-command is executed, the
  shell sets the READLINE_LINE variable to the contents of the Readline line buffer … If the
  executed command changes the value of any of READLINE_LINE, READLINE_POINT, or READLINE_MARK,
  those new values will be reflected in the editing state." So a function bound with `bind -x`
  can rewrite the prompt line; a function invoked as a normal command cannot. Source: Bash
  Reference Manual, Bash Builtins —
  <https://www.gnu.org/software/bash/manual/html_node/Bash-Builtins.html>.
- fzf's `__fzf_history__` does exactly this: it sets `READLINE_LINE` to the selected entry and
  `READLINE_POINT` to the end, bound via `bind -m emacs-standard -x '"\C-r": __fzf_history__'`
  (and likewise for vi-command/vi-insert). For bash < 4 (no usable `bind -x` for this), fzf falls
  back to a macro hack built from key sequences and command substitution. Source: fzf
  `shell/key-bindings.bash` —
  <https://github.com/junegunn/fzf/blob/master/shell/key-bindings.bash>.
- atuin's bash half is the same shape: `READLINE_LINE=$__atuin_output;
  READLINE_POINT=${#READLINE_LINE}`, with the same `__atuin_accept__:` prefix protocol for
  run-immediately. Source: atuin `crates/atuin/src/shell/atuin.bash` —
  <https://github.com/atuinsh/atuin/blob/main/crates/atuin/src/shell/atuin.bash>.
- mcfly shows an alternative capture channel: it passes a temp-file path to the picker
  (`mcfly search -o "$MCFLY_OUTPUT"`), then parses mode + command out of that file and assigns
  `READLINE_LINE`/`READLINE_POINT`; line 1 of the file carries a `mode` ("display" vs "run") that
  decides whether to auto-accept. Source: mcfly `mcfly.bash` —
  <https://github.com/cantino/mcfly/blob/master/mcfly.bash>.

### 1.3 `eval "$(potato --pick)"` — rejected for Enter

An eval-style wrapper executes whatever the picker prints, immediately, with no review step and
no history entry of the edited form. That is the right shape for tools whose output is a command
to run on the user's behalf (zoxide's `__zoxide_zi` runs `__zoxide_cd "$(zoxide query -i)"` —
source: zoxide `templates/zsh.txt`,
<https://github.com/ajeetdsouza/zoxide/blob/main/templates/zsh.txt>), but it contradicts potato's
"never executes commands itself; user reviews at the prompt" requirement. Use eval only for
*installing* the integration (§3), never for delivering the selection.

## 2. TUI on the terminal, result on stdout

The picker must be able to draw its UI even while its stdout is being captured by the wrapper's
command substitution. Two proven patterns:

- **fzf: TUI on `/dev/tty`, result on stdout.** The man page describes fzf as an interactive
  filter that reads the list from stdin and writes the selected item to stdout, and documents
  that the finder UI is drawn on the current TTY device, "defaulting to `/dev/tty`"
  (`--tty-default=DEVICE_NAME` / `--no-tty-default` exist to override). Source: fzf man page —
  <https://github.com/junegunn/fzf/blob/master/man/man1/fzf.1>. In the source, `openTtyIn`/
  `openTtyOut` open the TTY device (falling back to `DefaultTtyDevice`) separately from stdio.
  Source: fzf `src/tui/light_unix.go` —
  <https://github.com/junegunn/fzf/blob/master/src/tui/light_unix.go>.
- **atuin: stream swap in the wrapper.** atuin's widgets run
  `atuin search … -i 3>&1 1>&2 2>&3 3>&-` inside a command substitution — the classic idiom that
  swaps fd 1 and fd 2, so the TUI stream reaches the terminal while the other stream (carrying
  only the final selection) is captured into `$output`. Sources: atuin `atuin.zsh` and
  `atuin.bash` (URLs above).
- **mcfly: temp file.** UI owns the terminal; the result goes through `-o <tempfile>` (URL
  above). Works everywhere but leaves files to clean up and needs `mktemp` portability care.

### Ink specifics

Ink's `render(tree, options)` accepts the streams explicitly: `stdout` ("Output stream where the
app will be rendered", default `process.stdout`) and `stdin` ("Input stream where app will listen
for input", default `process.stdin`), plus `stderr` and `patchConsole`. Source: Ink readme —
<https://github.com/vadimdemedes/ink#rendertree-options>. So potato can reserve the real
`process.stdout` for the final selection and render the UI elsewhere. Two options:

1. **fzf-style (recommended):** open `/dev/tty` and pass
   `new tty.ReadStream(fs.openSync('/dev/tty', 'r'))` as `stdin` and
   `new tty.WriteStream(fs.openSync('/dev/tty', 'w'))` as `stdout` to `render()`. Node documents
   both constructors (`new tty.ReadStream(fd[, options])`, `new tty.WriteStream(fd)`); the docs
   note manual instances are rarely needed — this is one of the rare legitimate cases, and it is
   exactly what fzf does at the process level. Source: Node.js tty docs —
   <https://nodejs.org/api/tty.html>. This also frees stdin for a future piped-input mode.
2. **stderr-render:** pass `stdout: process.stderr` to `render()` and let the wrapper's `2>` go
   to the terminal. Simpler, but breaks if the user redirects stderr, and Ink's non-TTY fallback
   (only the final frame is written when the output stream is not a TTY — Ink readme, above)
   kicks in when stderr is piped.

Either way: on Enter, potato writes the rendered command + `\n` to the real `process.stdout` and
exits 0; on cancel it prints nothing and exits non-zero (fzf uses 130 for Ctrl-C; potato should
match so wrappers can `|| return`).

## 3. Installed wrapper functions and rc wiring

zoxide and atuin both ship a `<tool> init <shell>` subcommand that prints the shell script, and
tell users to add one line to their rc file:

- zoxide: "Add this to the **end** of your config file (usually `~/.zshrc`):
  `eval "$(zoxide init zsh)"`" (same for bash/`~/.bashrc`), with the zsh caveat that the line
  must come *after* `compinit` for completions. Source: zoxide README —
  <https://github.com/ajeetdsouza/zoxide#installation>. The script itself is generated from
  per-shell templates (`templates/zsh.txt`, `templates/bash.txt`) —
  <https://github.com/ajeetdsouza/zoxide/tree/main/templates>.
- atuin: `eval "$(atuin init zsh)"` / `eval "$(atuin init bash)"`; bash additionally needs
  bash-preexec or ble.sh for its hooks (not needed for a pure `bind -x` widget like potato's).
  Source: atuin README — <https://github.com/atuinsh/atuin#shell-plugin>.

Recommended `potato init zsh` output (shape, following fzf/atuin):

```zsh
potato-widget() {
  local out
  out=$(command potato pick --query "$BUFFER") || { zle reset-prompt; return }
  BUFFER=$out
  CURSOR=${#BUFFER}
  zle reset-prompt
}
zle -N potato-widget
bindkey -M emacs '^[p' potato-widget   # Alt-P; make configurable
bindkey -M viins '^[p' potato-widget
bindkey -M vicmd '^[p' potato-widget

potato() {  # plain command form: pre-fill next prompt
  local out
  out=$(command potato pick "$@") || return
  print -z -- "$out"
}
```

Recommended `potato init bash` output (bash >= 4):

```bash
__potato_widget() {
  local out
  out=$(command potato pick --query "$READLINE_LINE") || return
  READLINE_LINE=$out
  READLINE_POINT=${#READLINE_LINE}
}
bind -m emacs-standard -x '"\ep": __potato_widget'
bind -m vi-command     -x '"\ep": __potato_widget'
bind -m vi-insert      -x '"\ep": __potato_widget'
```

The install script's only rc-file job is appending the guarded line
`eval "$(potato init zsh)"` / `eval "$(potato init bash)"` to `~/.zshrc` / `~/.bashrc` if absent
(zoxide's documented convention, above). Keeping the logic behind `init` means fixes ship with
the binary, not with rc-file edits.

## 4. Clipboard mechanics (Ctrl-Y)

### 4.1 OSC 52

- Spec: xterm's ctlseqs defines OSC `Ps=52` "Manipulate Selection Data" with `Pt = Pc ; Pd`,
  where `Pc` names selections (`c` = clipboard, `p` = primary, `q` = secondary, `s` = select,
  `0-7` = cut buffers) and `Pd` is "a string encoded in base64 (RFC-4648)"; `?` queries, other
  non-base64 clears. In xterm itself the control is gated by the `allowWindowOps` resource.
  Source: XTerm Control Sequences —
  <https://invisible-island.net/xterm/ctlseqs/ctlseqs.html>.
  Concretely potato would write `\x1b]52;c;<base64(text)>\x07` to `/dev/tty`.
- Works over SSH: the tmux wiki notes the point of OSC 52 is that the escape travels with normal
  terminal output, so "it works over an ssh(1) connection even if X11 forwarding is not
  configured". Source: tmux wiki, Clipboard —
  <https://github.com/tmux/tmux/wiki/Clipboard>.
- tmux: governed by `set-clipboard` — `on` "both makes tmux set the clipboard for the outside
  terminal, and allows applications inside tmux to set tmux's clipboard" (i.e. an app's OSC 52 is
  forwarded; no manual passthrough wrapping needed); `external` (the default since tmux 2.6)
  lets only tmux itself set it; `off` disables. tmux also needs `Ms` in `terminal-overrides` for
  unusual outer terminals. Users who want Ctrl-Y to work inside tmux need
  `set -s set-clipboard on`. Source: tmux wiki, Clipboard (URL above).
- Terminal support (per the tmux wiki's matrix, same URL): supported by xterm (disabled by
  default; needs the `disallowedWindowOps`/`allowWindowOps` resource tweak), iTerm2 (opt-in
  preference), kitty, st; historically **not** supported by VTE terminals (GNOME Terminal, XFCE
  Terminal) or rxvt-unicode. VTE's tracking issue for OSC 52 write support is
  <https://gitlab.gnome.org/GNOME/vte/-/issues/2495> (write-only support landed in modern VTE;
  treat VTE-based terminals as "recent versions only"). Modern terminals (WezTerm, Alacritty,
  foot, Windows Terminal, Ghostty) document OSC 52 support in their own docs. Net: OSC 52 is the
  only mechanism that works over SSH, but it cannot be the *only* mechanism.
- Payload limits exist per terminal (the tmux wiki notes st's historical length cap). Keep
  copies to shell-command size (bytes to low KB) and this is a non-issue.

### 4.2 OS clipboard tools (fallbacks)

- macOS: `pbcopy` "takes the standard input and places it in the specified pasteboard"
  (general pasteboard by default). Source: `PBCOPY(1)` man page, macOS.
- Wayland: `wl-copy` from wl-clipboard ("command-line copy/paste utilities for Wayland").
  Source: <https://github.com/bugaevc/wl-clipboard>.
- X11: `xclip` ("command line interface to the X11 clipboard"; use `-selection clipboard`).
  Source: <https://github.com/astrand/xclip>.
- WSL: `clip.exe` is available from within WSL.

### 4.3 Recommended clipboard strategy

On Ctrl-Y, inside potato (no shell involvement needed):

1. Spawn the first available native tool — `pbcopy` (darwin), `wl-copy` (if
   `WAYLAND_DISPLAY`), `xclip -selection clipboard` (if `DISPLAY`), `clip.exe` (WSL) — and pipe
   the rendered command to it.
2. Always *also* emit OSC 52 to `/dev/tty` (base64, `c` selection). Terminals that don't support
   it ignore the sequence, and it is the only path that works over plain SSH.
3. Show a status-line confirmation; document `set -s set-clipboard on` for tmux users.

Don't shell out to the wrapper for clipboard — the TUI already owns `/dev/tty` and can do both
steps itself.

## 5. Recommended design for potato

1. **Binary contract:** `potato pick` renders the Ink TUI on `/dev/tty` (Ink `render()` with
   `tty.ReadStream`/`tty.WriteStream` on `/dev/tty` as `stdin`/`stdout`), and reserves the real
   stdout for exactly one thing: the final rendered command, printed on Enter, exit 0. Cancel
   prints nothing, exit 130. (fzf pattern; Ink and Node APIs support it directly — §2.)
2. **Shell integration:** `potato init zsh|bash` prints the integration script; install docs and
   the install script add `eval "$(potato init <shell>)"` at the end of the rc file (zoxide
   pattern — §3). The zsh script installs a zle widget (`BUFFER`/`CURSOR`, `zle reset-prompt`)
   plus a `print -z` wrapper for plain invocation; the bash script installs a `bind -x` widget
   (`READLINE_LINE`/`READLINE_POINT`), bash >= 4 only (§1).
3. **Enter never executes.** The command lands pre-filled at the prompt; the user reviews and
   presses Enter themselves. If a run-immediately mode is ever wanted, use atuin's in-band prefix
   protocol (`__potato_accept__:` + `zle accept-line` / bound accept key) rather than eval (§1.3).
4. **Ctrl-Y inside the TUI:** native clipboard tool if present, plus unconditional OSC 52 to
   `/dev/tty`; document tmux `set-clipboard on` (§4).
