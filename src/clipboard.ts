// Copy (spec §4.2): spawn the native clipboard tool if present AND always
// emit OSC 52 — the only mechanism that works over SSH and inside tmux.

import { spawnSync } from 'node:child_process';

const NATIVE_TOOLS: string[][] = [['pbcopy'], ['wl-copy'], ['xclip', '-selection', 'clipboard']];

export function copyToClipboard(text: string): void {
  for (const [cmd, ...args] of NATIVE_TOOLS) {
    const result = spawnSync(cmd!, args, { input: text, stdio: ['pipe', 'ignore', 'ignore'] });
    if (!result.error && result.status === 0) break;
  }
  process.stdout.write(`\x1b]52;c;${Buffer.from(text).toString('base64')}\x07`);
}
