# ADR-0001: Reject v1 Libraries rather than migrating them

Date: 2026-07-30
Status: Accepted

## Context

The Library file has had two shapes. v1 kept Commands in an object keyed by name, with a parallel `names` array carrying the file order. v2 (commit `971958a`) made them an array of entries identified by a stable UUID, which is what lets a rename keep its State and its file slot.

`internal/migrate` existed to close that gap: on every launch it read `commands.json`, and if it found v1 it minted an id per Command, rewrote the file as v2, and re-keyed `state.json` from names onto the new ids.

It was serving a file that cannot exist.

```
$ git tag --contains 971958a
v0.1.1  v0.1.2  v0.1.3  v0.2.0  v0.2.1  v0.2.2  v0.2.3
```

Every published release contains the v2 commit. The earliest release is v0.1.1 (24 July 2026); v2 landed before it, and there was never a v0.1.0. So **no released binary of potato has ever written a v1 Library.** One could only exist on a machine that ran potato from source inside the window between `a20802a` ("Implement potato v1 per spec") and `971958a` — a window that closed before the project's first release, and only then if that machine never ran a newer build, which would have migrated the file already.

Meanwhile the migration was not free:

- 437 lines (`migrate.go` plus its tests) for a code path with no reachable input.
- `library.NewError` was exported solely so the v1 reader could mint parser-shaped errors.
- `migrate.Load` became the front door for *steady-state* loading of both files, so `library.Load` had zero production callers and `state.Load` was reachable only through the migration package. Working out which `Load` a new call site should use meant grepping for callers.
- A partial-migration state was deliberately tolerated: the commands write was atomic and fail-loud, the State re-key rode along best-effort, so "v2 commands on disk, State still keyed by v1 names" was reachable and accepted.
- `potato import --override` wrote with `library.Save` without going through `migrate.Load`, so it replaced a v1 file with a v2 one while leaving State keyed by v1 names — the re-key never ran and those keys became permanent orphans.

## Decision

Delete `internal/migrate`. `library.Parse` stays version-strict and a v1 file fails loud with the message it already produced:

```
potato: commands.json: unsupported version 1 (expected 2)
```

`cmd/potato` loads with `library.Load` and `state.Load` directly. `Deps.Migrated` and the in-TUI upgrade toast go with it.

## Consequences

- A v1 `commands.json` is a hard failure. The file is left untouched, so nothing is destroyed and the user can convert or delete it themselves. Given the evidence above, the population this affects is approximately the author's own machine.
- Failing loud is the point: silently treating an unparseable Library as empty would be indistinguishable from data loss.
- `library.Load` becomes the front door and stops being dead code. The "always call `migrate.Load` first, and it loads State for you too" contract disappears rather than needing a name.
- The tolerated partial-migration failure mode and the `--override` orphan bug both cease to exist.
- Incoming import files are unaffected — they were always version-strict, and `potato import` still rejects a v1 file from someone else.

## Revisiting

Reopen this if a v2→v3 Library shape is ever needed. Note that the argument here is specifically that *v1 was never released*; it says nothing about whether a future version bump deserves a migration. A v3 would ship to users who are demonstrably on v2, which is the opposite situation.
