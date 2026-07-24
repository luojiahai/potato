// potato CLI (spec §5): the TUI by default (--out <file> carries the
// selection back to the shell wrapper), plus import / init / update /
// uninstall subcommands.

import React from 'react';
import { render } from 'ink';
import { readFileSync, writeFileSync } from 'node:fs';
import { App, type AppDeps } from './tui/App';
import { bunSafeStdin } from './tui/stdin';
import { copyToClipboard } from './clipboard';
import { mergeLibraries } from './import';
import { loadLibrary, parseLibrary, saveLibrary } from './library';
import { commandsPath, binPath, potatoDir, statePath } from './paths';
import { initScript } from './shell';
import { loadState, saveState } from './state';
import { runUninstall } from './uninstall';
import { runUpdate } from './update';
import { VERSION } from './version';

const USAGE = `potato ${VERSION} — save, find, and hand off long terminal commands

usage:
  potato                       open the TUI (Enter = run, Ctrl-Y = copy)
  potato --out <file>          TUI; write the selection to <file> (shell glue)
  potato import <file|-> [--theirs]
                               merge another library in (ours wins; --theirs overwrites)
  potato update                update to the latest release
  potato uninstall [--purge]   remove potato (keep data; --purge wipes it)
  potato init <zsh|bash|sh>    print shell integration (used by the installer)
  potato --version             print the version
`;

function die(message: string): never {
  console.error(`potato: ${message}`);
  process.exit(1);
}

async function runTui(outFile: string | null): Promise<void> {
  if (!process.stdin.isTTY) die('the potato TUI needs a terminal');
  const library = loadLibrary(commandsPath());
  const state = loadState(statePath());
  let handoff: string | null = null;

  const deps: AppDeps = {
    library,
    state,
    saveLibrary: (lib) => saveLibrary(commandsPath(), lib),
    saveState: (s) => saveState(statePath(), s),
    copy: (text) => copyToClipboard(text),
    now: () => new Date(),
  };

  // Wrap stdin (which enables raw mode) BEFORE the first stdout write: Bun's
  // lazy tty init on stdout leaves stdin cooked if raw mode comes second.
  const { stdin, cleanup } = bunSafeStdin();
  const useAltScreen = process.stdout.isTTY;
  if (useAltScreen) process.stdout.write('\x1b[?1049h'); // no scrollback litter
  const app = render(<App deps={deps} onHandoff={(command) => (handoff = command)} />, {
    stdin,
    exitOnCtrlC: true,
  });
  await app.waitUntilExit();
  cleanup();
  if (useAltScreen) process.stdout.write('\x1b[?1049l');

  if (outFile !== null) {
    writeFileSync(outFile, handoff ?? ''); // empty file = cancelled (spec §4.1)
  } else if (handoff !== null) {
    // run outside the shell wrapper: can't pre-fill the prompt, so print
    console.log(handoff);
  }
}

function runImport(args: string[]): void {
  const theirsFlag = args.includes('--theirs');
  const file = args.find((a) => a === '-' || !a.startsWith('--'));
  if (!file) die('usage: potato import <file|-> [--theirs]');

  const source = file === '-' ? 'stdin' : file;
  let text: string;
  try {
    text = file === '-' ? readFileSync(0, 'utf8') : readFileSync(file, 'utf8');
  } catch (e) {
    die(`cannot read ${source}: ${(e as Error).message}`);
  }

  const theirs = parseLibrary(text, source);
  const ours = loadLibrary(commandsPath());
  const { merged, added, skipped, overwritten } = mergeLibraries(ours, theirs, { theirs: theirsFlag });
  saveLibrary(commandsPath(), merged);

  if (added.length) console.log(`added: ${added.join(', ')}`);
  if (overwritten.length) console.log(`overwritten: ${overwritten.join(', ')}`);
  if (skipped.length) {
    console.log(`skipped (already yours, differs): ${skipped.join(', ')}`);
    console.log('re-run with --theirs to overwrite');
  }
  if (!added.length && !overwritten.length && !skipped.length)
    console.log('nothing to import — your Library already has all of these');
}

function runInit(args: string[]): void {
  const shell = args[0];
  if (shell !== 'zsh' && shell !== 'bash' && shell !== 'sh')
    die('usage: potato init <zsh|bash|sh>');
  process.stdout.write(initScript(shell, binPath(), potatoDir()));
}

async function main(): Promise<void> {
  const [cmd, ...rest] = process.argv.slice(2);
  try {
    switch (cmd) {
      case undefined:
        return await runTui(null);
      case '--out': {
        if (!rest[0]) die('--out needs a file path');
        return await runTui(rest[0]);
      }
      case 'import':
        return runImport(rest);
      case 'init':
        return runInit(rest);
      case 'update':
        return await runUpdate();
      case 'uninstall':
        return runUninstall(rest);
      case '--version':
      case '-v':
        return console.log(VERSION);
      case '--help':
      case '-h':
        return void process.stdout.write(USAGE);
      default:
        console.error(`potato: unknown command '${cmd}'\n`);
        process.stdout.write(USAGE);
        process.exit(1);
    }
  } catch (e) {
    if (e instanceof Error) die(e.message);
    throw e;
  }
}

await main();
