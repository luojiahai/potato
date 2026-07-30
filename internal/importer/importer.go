// Package importer is a simple, local merge — not a sync tool. It never
// reasons about the incoming file's ids (identity stays internal): every
// incoming Command is added as a brand-new Command with a fresh id, and on a
// name Collision the incoming copy is renamed so both are kept. `--override`
// (handled in cmd/potato) is the other mode: replace the whole Library with
// the imported file as-is.
package importer

import (
	"fmt"

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
//
// It fails rather than skipping a Command the Library refuses. `theirs` came
// from library.Parse, so its Commands are already in the normalised form Add
// accepts and FreeName has just resolved the only other thing Add refuses —
// which makes a refusal here a bug in one of those three, not something a user
// can hand us. But "nothing is lost" is what `--merge` promises (CONTEXT.md,
// Import), and a merge that quietly dropped one and reported success would break
// that promise in the one direction the user cannot see. Failing means the caller
// dies before writing, so the Library on disk keeps every Command it had.
func Merge(ours, theirs library.Library) (Result, error) {
	merged := ours
	result := Result{Added: []string{}, Renamed: []Rename{}}

	for _, command := range theirs.Commands {
		name := library.FreeName(merged, command.Name)

		draft := library.Draft{Name: name, Template: command.Template, Extra: command.Extra}
		if command.Description != nil {
			draft.Description = *command.Description
		}
		next, err := library.Add(merged, draft)
		if err != nil {
			return Result{}, fmt.Errorf("cannot import %q: %w", command.Name, err)
		}
		merged = next

		// Reported after the Add, so a Command that was not taken cannot be
		// reported as taken.
		if name != command.Name {
			result.Renamed = append(result.Renamed, Rename{From: command.Name, To: name})
		} else {
			result.Added = append(result.Added, name)
		}
	}

	result.Merged = merged
	return result, nil
}
