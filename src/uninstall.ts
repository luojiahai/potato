// potato uninstall (spec §8): remove the rc line, delete the binary and
// generated init files, keep user data (--purge wipes everything).

import { existsSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { binDir, commandsPath, initPath, potatoDir, statePath } from './paths';

// Drop any line that sources a potato init file; everything else unchanged.
export function removeInitLines(content: string, needles: string[] = ['.potato/init.']): string {
  return content
    .split('\n')
    .filter((line) => !needles.some((n) => line.includes(n)))
    .join('\n');
}

export function runUninstall(args: string[]): void {
  const purge = args.includes('--purge');
  const needles = ['.potato/init.', join(potatoDir(), 'init.')];

  for (const rc of [join(homedir(), '.zshrc'), join(homedir(), '.bashrc')]) {
    if (!existsSync(rc)) continue;
    const content = readFileSync(rc, 'utf8');
    const cleaned = removeInitLines(content, needles);
    if (cleaned !== content) {
      writeFileSync(rc, cleaned);
      console.log(`removed potato line from ${rc}`);
    }
  }

  if (purge) {
    rmSync(potatoDir(), { recursive: true, force: true });
    console.log(`removed ${potatoDir()}`);
    return;
  }

  rmSync(binDir(), { recursive: true, force: true });
  for (const shell of ['zsh', 'bash', 'sh'] as const) rmSync(initPath(shell), { force: true });
  console.log('potato uninstalled.');
  console.log(`your data is kept at ${commandsPath()} and ${statePath()}`);
  console.log('run with --purge to remove it too.');
}
