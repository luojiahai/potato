# Packaging potato for `curl | bash` install without Node preinstalled

Research for [issue #2](https://github.com/luojiahai/potato/issues/2) (part of map [#1](https://github.com/luojiahai/potato/issues/1)).

**Question:** How should potato — a TypeScript + Ink TUI app — be packaged so `curl -fsSL .../install.sh | bash` installs a working binary on macOS and Linux (x64 + arm64), with no Node runtime assumed?

**Date:** 2026-07-24. Versions used in the hands-on experiment: Bun 1.3.14, ink 7.1.1, react 19.2.8, Node v24.18.0, on macOS arm64.

---

## TL;DR recommendation

**Compile with `bun build --compile` (cross-compiling all four targets from a single CI job), publish the binaries + a `SHA256SUMS` file as GitHub Release assets, and ship a starship-style `install.sh` that detects platform/arch, downloads the right asset from `releases/latest/download/`, verifies the sha256, and installs to `~/.potato/bin` (overridable), warning if that dir is not on `PATH`.**

This was verified empirically: a real Ink 7 app compiles and runs correctly as a Bun single-file executable, and all four targets (macOS/Linux × x64/arm64) cross-compile from one macOS machine in ~1s each. Node SEA cannot cross-compile and needs a per-target build matrix; Deno compile is unproven for Ink and adds risk for no benefit. This is also the path Claude Code — the most prominent Ink app in existence — ships through: platform binaries behind a `curl | bash` bootstrap with sha256 verification.

---

## Option 1: Bun single-file executables (`bun build --compile`) — recommended

### What the docs say

- `bun build --compile` bundles the app **and a copy of the Bun runtime** into one executable; everything is always bundled ([Bun docs: single-file executables](https://bun.com/docs/bundler/executables)).
- Cross-compilation is first-class via `--target`: `bun-darwin-x64`, `bun-darwin-arm64`, `bun-linux-x64`, `bun-linux-arm64`, plus `-musl` variants for Alpine and `-baseline` variants for CPUs without AVX2 — all buildable **from any host OS** ([Bun docs: cross-compile to production](https://bun.com/docs/bundler/executables)).
- `--minify` and `--sourcemap` are recommended for compiled executables; `--bytecode` optionally speeds startup ([same page](https://bun.com/docs/bundler/executables)).

### Does Ink/React/yoga actually work? Yes — verified by experiment

Historically this was broken: `bun build --compile` failed on Ink apps with `Cannot find module ./yoga.wasm` because older yoga loaded its WASM via a runtime-resolved file path ([oven-sh/bun#6567](https://github.com/oven-sh/bun/issues/6567), [oven-sh/bun#13552](https://github.com/oven-sh/bun/issues/13552), [oven-sh/bun#2034](https://github.com/oven-sh/bun/issues/2034)). Two things have since changed:

1. **yoga-layout no longer ships a loose `.wasm` file.** Ink 7.1.1 depends on `yoga-layout ~3.2.1` ([ink package.json](https://github.com/vadimdemedes/ink/blob/master/package.json)), and yoga-layout's loader imports the WASM as a **base64-embedded ESM JS module** (`import loadYogaImpl from '../binaries/yoga-wasm-base64-esm.js'` in [facebook/yoga javascript/src/load.ts](https://github.com/facebook/yoga/blob/main/javascript/src/load.ts)) — so there is no external file for the bundler to lose.
2. Bun v1.1.25 (2024-08-21) additionally fixed runtime-known relative-path resolution of embedded files (including `.wasm`) in standalone executables ([Bun v1.1.25 release notes](https://bun.com/blog/bun-v1.1.25)).

**Experiment (2026-07-24, Bun 1.3.14, ink 7.1.1, react 19.2.8):** an Ink app using `render`, `<Box borderStyle="round">`, `<Text>`, `useState`/`useEffect` timers was compiled with `bun build --compile --minify` and run as a standalone binary on macOS arm64. It rendered the bordered layout (i.e. yoga's WASM layout engine executed inside the binary), updated state live, and exited cleanly.

**One real caveat found:** Ink's reconciler conditionally loads React DevTools at module top level (`if (process.env['DEV'] === 'true') { ... await import('./devtools.js') }` — [ink reconciler.ts](https://github.com/vadimdemedes/ink/blob/master/src/reconciler.ts), documented in [Ink's readme](https://github.com/vadimdemedes/ink#readme) as the opt-in `react-devtools-core` integration). Bun's bundler hoists that dynamic import statically, so the build fails with `Could not resolve: "react-devtools-core"` (it's an optional peer dep that isn't installed), and `--external react-devtools-core` merely defers the same failure to startup. **Working fix (verified):** alias `react-devtools-core` to a one-line stub via `tsconfig.json` `paths`:

```jsonc
// tsconfig.json
{ "compilerOptions": { "paths": { "react-devtools-core": ["./stubs/react-devtools-core.ts"] } } }
```
```ts
// stubs/react-devtools-core.ts
export default { connectToDevTools() {} };
```

(Bun's bundler resolves imports through tsconfig `paths` — [Bun bundler docs](https://bun.com/docs/bundler).) With the stub, the build is clean and the binary works.

### Cross-compilation — verified

All four targets built from one macOS arm64 machine (no other hardware involved), ~1s per target:

| Target | Command | Size | `file` output |
|---|---|---|---|
| macOS arm64 | (native) | 61 MB (23 MB gzip) | Mach-O arm64 |
| macOS x64 | `--target=bun-darwin-x64` | 66 MB | Mach-O x86_64 |
| Linux x64 | `--target=bun-linux-x64` | 91 MB (34 MB gzip) | ELF x86-64, glibc |
| Linux arm64 | `--target=bun-linux-arm64` | 90 MB | ELF aarch64, glibc |

Sizes are dominated by the embedded Bun runtime (the standalone `bun` binary itself is ~60 MB on macOS arm64; measured locally). `-musl` targets exist if Alpine support is ever wanted ([Bun docs](https://bun.com/docs/bundler/executables)).

### Real-world precedent

Ink's own readme lists **Claude Code** (Anthropic) and **Gemini CLI** (Google) as its flagship users ([ink readme](https://github.com/vadimdemedes/ink#readme)). Claude Code ships exactly this shape: a `curl -fsSL https://claude.ai/install.sh | bash` bootstrap that downloads a prebuilt platform binary (`darwin-arm64`, `linux-x64`, `linux-x64-musl`, …) and verifies its sha256 against a manifest ([claude.ai/install.sh](https://claude.ai/install.sh), which redirects to [downloads.claude.ai/claude-code-releases/bootstrap.sh](https://downloads.claude.ai/claude-code-releases/bootstrap.sh)). An Ink TUI distributed as a no-runtime-required native binary is a proven production pattern.

## Option 2: Node Single Executable Applications (SEA)

Per the [official Node.js SEA docs](https://nodejs.org/api/single-executable-applications.html):

- **Status:** Stability 1.1 — *Active development* (experimental). The streamlined `node --build-sea` flow only landed in v25.5.0; before that it's the manual blob + `postject` injection + `codesign` dance.
- **Cross-compilation: effectively unsupported.** The docs warn that cross-platform SEA generation requires disabling `useCodeCache`/`useSnapshot` to avoid "incompatible executables", and the process works by copying and mutating the *host* `node` binary — so producing 4 targets means 4 native build environments (a full CI matrix: macOS x64, macOS arm64, Linux x64, Linux arm64), each followed by platform-specific signature surgery (`codesign --sign -` on macOS). Notably, the docs list macOS x64 as *skipped* in Node's own SEA CI.
- **Bundling friction for Ink specifically:** the injected script must be a single pre-bundled file, and by default can only `require` builtins. Ink 7 is ESM-only with **top-level await** in its reconciler ([ink reconciler.ts](https://github.com/vadimdemedes/ink/blob/master/src/reconciler.ts)), so it cannot be bundled down to the well-trodden CommonJS SEA path; it needs the newer `"mainFormat": "module"` support, which is the least-exercised corner of an already-experimental feature.
- **Size: worst of all options.** The SEA is a copy of the whole `node` binary plus the blob — `node` v24.18.0 alone is **115 MB** on macOS arm64 (measured locally), vs 61 MB for the equivalent complete Bun executable.

Verdict: strictly more CI complexity (4-runner matrix + postject + codesign), larger binaries, experimental status, and an awkward ESM story. No advantage for potato.

## Option 3: Deno compile

Per the [official `deno compile` docs](https://docs.deno.com/runtime/reference/cli/compile/):

- Cross-compilation is good: `--target` covers all four needed triples (`x86_64/aarch64-apple-darwin`, `x86_64/aarch64-unknown-linux-gnu`) from any host.
- npm packages are supported; by default the **entire resolved `node_modules` tree is embedded**, with an experimental `--bundle` flag to tree-shake. Non-statically-analyzable dynamic imports are dropped unless `--include`d — Ink's conditional devtools import is exactly that shape.
- **Ink on Deno is unproven.** Ink declares `engines: node >= 22` and is developed/tested against Node ([ink package.json](https://github.com/vadimdemedes/ink/blob/master/package.json)); nothing in Ink's readme mentions Deno ([ink readme](https://github.com/vadimdemedes/ink#readme)). A raw-mode TUI leans on the hairiest part of Deno's Node compat (stdin raw mode / TTY streams), an area with a long tail of upstream issues in Deno's own tracker (e.g. [denoland/deno#21930](https://github.com/denoland/deno/issues/21930), [denoland/deno#27260](https://github.com/denoland/deno/issues/27260)).

Verdict: viable cross-compile story, but it would make potato a compatibility pioneer for its whole UI layer. Only worth revisiting if Bun ever regresses.

## Option 4: GitHub Releases + install script

This is not an alternative to options 1–3 — it's the **delivery half** of the answer regardless of which compiler produces the binaries. Conventions observed in the real scripts (primary sources: [bun.sh/install](https://bun.sh/install), [deno.land/install.sh](https://deno.land/install.sh), [starship.rs/install.sh](https://starship.rs/install.sh), [Claude Code bootstrap.sh](https://downloads.claude.ai/claude-code-releases/bootstrap.sh)):

| Convention | bun.sh/install | deno.land/install.sh | starship.rs/install.sh | Claude Code bootstrap.sh |
|---|---|---|---|---|
| Install dir | `~/.bun/bin` (env `BUN_INSTALL`) | `~/.deno/bin` (env `DENO_INSTALL`) | `/usr/local/bin` (flag `-b`, sudo only if unwritable) | `~/.claude/downloads`, then binary self-installs |
| Platform/arch detection | `uname -ms` case-map to target triple | `uname -sm` case-map | `uname -s` / `uname -m`, normalized | OS + arch map, e.g. `darwin-arm64` |
| Rosetta 2 handling | Yes — `sysctl -n sysctl.proc_translated` → prefer arm64 | No | No | Yes |
| musl detection | `/etc/alpine-release` → `-musl` target | No | Uses musl-static Linux builds | `ldd` / `libc.musl-*` probe → `-musl` |
| Checksum verification | **No** (HTTPS only) | **No** | **No** | **Yes** — sha256 from a manifest, fail-and-delete on mismatch |
| Download URL | `github.com/<org>/<repo>/releases/latest/download/bun-<target>.zip` (or `/download/<tag>/…` for pinned version) | `dl.deno.land/release/<version>/deno-<target>.zip` | `…/releases/latest/download/starship-<triple>.<ext>` | first-party CDN + `manifest.json` |
| PATH handling | Appends `export PATH` to shell rc (zsh/bash/fish), prints manual fallback | Optional rc edit, `--no-modify-path` opt-out | **Warns only**, never edits rc files | Delegated to the binary's `install` subcommand |
| Error handling | `set -euo pipefail`, checks `curl`/`unzip` upfront | `set -e`, requires `unzip` or `7z` | TTY-gated confirm prompts, `-f/-y` to skip | sudo refusal, `stty sane` restore on signal death |
| Version pinning | Optional arg (`bun-v1.x`) | Optional arg | `-v` flag | latest-version endpoint |

Notable: of the four, only Claude Code verifies checksums — the newest script has the strongest convention, and it costs ~10 lines. Starship's "warn, don't edit rc files" PATH stance is the least invasive; bun's rc-editing is the most convenient. Both are defensible; editing rc files should at least be opt-out-able.

## Recommended concrete setup for potato

1. **Build:** `bun build --compile --minify --sourcemap --target=bun-<os>-<arch> src/cli.tsx --outfile dist/potato-<os>-<arch>`, with the `react-devtools-core` tsconfig-paths stub described above. Four targets from one runner.
2. **CI (single job, e.g. `ubuntu-latest` or macOS):** on tag push — install Bun, loop over the 4 targets, `gzip`/`tar.gz` each (91 MB → 34 MB for linux-x64), generate `SHA256SUMS` (`shasum -a 256`), upload all assets to the GitHub Release (`gh release create`). No matrix, no codesigning step required to run (unsigned binaries run fine when installed to the user's home dir; Gatekeeper quarantine applies to browser downloads, not `curl`).
3. **`install.sh`** (committed to the repo, served raw): `set -euo pipefail`; `uname -sm` case-map to `darwin-x64|darwin-arm64|linux-x64|linux-arm64` with Rosetta detection (`sysctl -n sysctl.proc_translated`); download `https://github.com/luojiahai/potato/releases/latest/download/potato-<target>.tar.gz` (accept an optional version arg for pinning); **verify sha256 against the released `SHA256SUMS`**; install to `~/.potato/bin/potato` (override via `POTATO_INSTALL`); warn + print snippet if not on `PATH` (starship-style), only edit rc files if we later decide bun-style convenience is worth it.
4. **Size expectation:** ~60–90 MB installed, ~23–34 MB download per platform. That is the going rate for runtime-embedded JS CLIs (Claude Code ships the same way); if that ever becomes unacceptable, the escape hatches are Deno compile with `--bundle` (risky for Ink) or leaving the TUI world.

### Why not the alternatives, in one line each

- **Node SEA:** experimental, no cross-compilation (4-runner matrix + postject + codesign), 115 MB+ binaries, awkward ESM/TLA path for Ink. ([Node SEA docs](https://nodejs.org/api/single-executable-applications.html))
- **Deno compile:** fine compiler, but Ink-on-Deno is untested territory (raw-mode stdin compat risk) for zero payoff over Bun. ([deno compile docs](https://docs.deno.com/runtime/reference/cli/compile/))
- **npm-based install:** violates the premise — assumes a Node runtime.

## Sources

- Bun single-file executables: https://bun.com/docs/bundler/executables
- Bun v1.1.25 release notes (embedded-file resolution in executables): https://bun.com/blog/bun-v1.1.25
- Bun issues on the historical ink/yoga wasm failure: https://github.com/oven-sh/bun/issues/6567, https://github.com/oven-sh/bun/issues/13552, https://github.com/oven-sh/bun/issues/2034
- Node.js SEA docs: https://nodejs.org/api/single-executable-applications.html
- Deno compile docs: https://docs.deno.com/runtime/reference/cli/compile/
- Deno stdin/raw-mode issue tracker examples: https://github.com/denoland/deno/issues/21930, https://github.com/denoland/deno/issues/27260
- ink (deps, readme, reconciler devtools import): https://github.com/vadimdemedes/ink
- yoga-layout base64-embedded wasm loader: https://github.com/facebook/yoga/blob/main/javascript/src/load.ts
- Install scripts read in full: https://bun.sh/install, https://deno.land/install.sh, https://starship.rs/install.sh, https://claude.ai/install.sh (→ https://downloads.claude.ai/claude-code-releases/bootstrap.sh)
- Local experiment: Bun 1.3.14, ink 7.1.1, react 19.2.8, Node v24.18.0, macOS arm64, 2026-07-24 (sizes measured with `ls -lh`, formats with `file`, compression with `gzip -9`).
