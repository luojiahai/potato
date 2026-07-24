// Bun workarounds for Ink's stdin handling, both verified against Bun 1.3.14
// + Ink 7.1.1 in a pty:
//
// 1. Ink consumes stdin via 'readable' events, but Bun's native TTY stream
//    delivers those late and cooked — raw mode never engages if enabled after
//    the stream starts flowing, and Bun's PassThrough re-emits 'readable' one
//    write behind (each keystroke rendered only when the next arrived).
// 2. Flowing-mode 'data' reads on the real stdin work correctly.
//
// So: enable raw mode FIRST, read via 'data', and feed Ink a minimal fake
// stream (EventEmitter + read()) whose 'readable' emission we control.

import { EventEmitter } from 'node:events';

export interface WrappedStdin {
  stdin: NodeJS.ReadStream;
  cleanup(): void;
}

export function bunSafeStdin(real: NodeJS.ReadStream = process.stdin): WrappedStdin {
  const queue: string[] = [];

  const fake = new EventEmitter() as EventEmitter & Record<string, unknown>;
  fake.isTTY = real.isTTY;
  fake.setEncoding = () => fake;
  // We own raw mode for the whole session (enabled below, disabled in
  // cleanup). Ink toggles it off/on when screens swap useInput mounts, and
  // Bun ignores a re-enable once the stream is flowing — the terminal would
  // fall back to cooked mode mid-session. So Ink's toggles are no-ops here.
  fake.setRawMode = () => fake;
  fake.ref = () => real.ref?.();
  fake.unref = () => real.unref?.();
  fake.resume = () => fake;
  fake.pause = () => fake;
  fake.read = () => (queue.length > 0 ? queue.shift()! : null);

  const pump = (chunk: Buffer | string) => {
    queue.push(typeof chunk === 'string' ? chunk : chunk.toString('utf8'));
    fake.emit('readable');
  };

  // Raw mode must engage BEFORE the stream starts flowing (and before any
  // stdout tty write): Bun ignores setRawMode() once its read loop runs.
  real.setRawMode?.(true);
  real.on('data', pump);

  return {
    stdin: fake as unknown as NodeJS.ReadStream,
    cleanup() {
      real.off('data', pump);
      try {
        real.setRawMode?.(false);
      } catch {
        // stdin may already be closed
      }
      real.pause();
      real.unref?.();
    },
  };
}
