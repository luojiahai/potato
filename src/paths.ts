// Potato's entire footprint lives under ~/.potato (spec §6.2);
// POTATO_INSTALL overrides the root.

import { homedir } from 'node:os';
import { join } from 'node:path';

export const potatoDir = (): string => process.env.POTATO_INSTALL || join(homedir(), '.potato');
export const commandsPath = (): string => join(potatoDir(), 'commands.json');
export const statePath = (): string => join(potatoDir(), 'state.json');
export const binDir = (): string => join(potatoDir(), 'bin');
export const binPath = (): string => join(binDir(), 'potato');
export const initPath = (shell: 'zsh' | 'bash' | 'sh'): string => join(potatoDir(), `init.${shell}`);
