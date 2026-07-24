// No-op stand-in for Ink's optional react-devtools-core import, which breaks
// `bun build --compile`. Wired in via tsconfig `paths` (spec §6.1).
export function initialize(): void {}
export function connectToDevTools(): void {}
export default { initialize, connectToDevTools };
