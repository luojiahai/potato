// Package importer is a simple, local merge — not a sync tool. It never
// reasons about the incoming file's ids (identity stays internal): every
// incoming Command is added as a brand-new Command with a fresh id, and on a
// name Collision the incoming copy is renamed so both are kept. `--override`
// (handled in cmd/potato) is the other mode: replace the whole Library with
// the imported file as-is.
package importer

import (
	"github.com/luojiahai/potato/internal/library"
)

// Rename records one Collision: the incoming name and what it became.
type Rename struct {
	From string
	To   string
}

type Result struct {
	Merged  library.Library
	Added   []string // incoming names taken as-is (no collision)
	Renamed []Rename
}

// Merge adds every incoming Command with a fresh id, appending in incoming
// order. On a name Collision (case-sensitive, against the running merged set)
// the incoming copy takes `name (N)` — lowest free N from 1 — keeping both.
// Deliberately not idempotent: re-merging the same file duplicates everything.
//
// The Collision is resolved against the *running* merged Library rather than
// against `ours`, so two incoming Commands that collide with each other take
// successive suffixes instead of both taking `(1)`. That is what makes
// library.FreeName the right question to ask, and asking it per Command rather
// than seeding a set up front is what keeps this loop free of its own copy of
// the uniqueness rule.
func Merge(ours, theirs library.Library) Result {
	merged := ours
	result := Result{Added: []string{}, Renamed: []Rename{}}

	for _, entry := range theirs.Commands {
		name := library.FreeName(merged, entry.Name)

		draft := library.Draft{Name: name, Command: entry.Command, Extra: entry.Extra}
		if entry.Description != nil {
			draft.Description = *entry.Description
		}
		next, err := library.Add(merged, draft)
		if err != nil {
			// Unreachable: `theirs` came from library.Parse, so every Command in
			// it has a non-empty name and command, and FreeName just resolved
			// the only other thing Add refuses. Skipping rather than failing
			// keeps Merge total — and the report is written after the Add, so a
			// Command that was not taken is not reported as taken.
			continue
		}
		merged = next

		if name != entry.Name {
			result.Renamed = append(result.Renamed, Rename{From: entry.Name, To: name})
		} else {
			result.Added = append(result.Added, name)
		}
	}

	result.Merged = merged
	return result
}
