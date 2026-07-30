# ADR-0002: `library.Remove` does not prune State

Date: 2026-07-30
Status: Accepted

## Context

Command mutation now lives behind `library`: `Add`, `Update`, `Remove`. The obvious next question is whether `Remove` should also drop the deleted Command's `state.json` entry, since that entry is keyed by the id `Remove` was just handed and nothing else will ever look at it again.

It was a real bug that it went unpruned. `listScreen.delete` removed the Command from the Library and never touched State, so `~/.potato/state.json` accumulated an entry per deleted Command for as long as potato stayed installed. The only pruning anywhere in the codebase lived inside the v1→v2 re-key, which has since been deleted (ADR-0001) — so before this change, orphans were created on every delete and collected never.

The temptation is to make it impossible by construction: have `Remove` take both files, or introduce a module that owns the Library and the State together and pairs every mutation.

## Decision

`library.Remove` touches the Library and nothing else. Callers pair it with `state.Forget`.

In the TUI that pairing happens in `listScreen.delete`, which is the one place that knows a Command is being destroyed rather than merely edited.

## Rationale

The two files have different owners, lifetimes and audiences, and CONTEXT.md says so:

- **Library** — `~/.potato/commands.json`. The user's actual data. Portable: copying the file is sharing the Library. Written atomically, fail-loud, never silently discarded.
- **State** — `~/.potato/state.json`. A disposable per-Command cache of last-used time and last argument values. Explicitly *"Safe to delete; never shared or imported."* An unreadable State resets to empty without complaint.

A module owning both would have to pick one of those postures for the pair, and neither fits: making State writes authoritative would mean a failed cache write could block a Library edit, and making Library writes disposable is out of the question. It would also drag the load path in with it — one module owning both mutations naturally becomes the module that opens both files, which is a larger change than this one and re-raises a question ADR-0001 just settled.

So the coupling stays where the knowledge is. `Remove` and `Forget` are two calls because they are two files.

## Consequences

- No module owns both files. Any future path that deletes a Command has to remember both calls; the doc comment on `library.Remove` points at `state.Forget`, and `TestDeleteAlsoForgetsTheCommandsState` pins the TUI's pairing.
- The failure modes stay independent, which is the point: a failed State write is reported but does not undo the delete, and a failed Library write leaves the cache entry alone.
- Orphans remain *possible* rather than impossible. They are cheap — a few dead keys in a file that is safe to delete — and `state.Load` already tolerates anything it cannot read.

## Revisiting

Reopen this if a third caller needs to delete a Command, or if the load path is ever consolidated into a module that owns both files for other reasons. Two callers would make the pairing a real seam rather than a single site's responsibility; one caller does not.
