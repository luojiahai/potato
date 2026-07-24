// PROTOTYPE — during shell hand-off stdout is piped (command substitution), so
// chalk's stdout-based detection would strip all color even though the TUI
// renders on stderr, which is still a terminal. Must run before ink/chalk load.
if (process.stderr.isTTY && process.env.FORCE_COLOR === undefined) {
  process.env.FORCE_COLOR = '3';
}
