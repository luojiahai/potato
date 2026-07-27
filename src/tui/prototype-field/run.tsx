// PROTOTYPE — throwaway. Entry point: `bun run prototype:field`.
import React from 'react';
import { render } from 'ink';
import { Proto } from './proto';

render(<Proto />, { stdin: process.stdin, exitOnCtrlC: true });
