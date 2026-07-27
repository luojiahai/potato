// Package importer is a simple, local merge — not a sync tool. It never
// reasons about the incoming file's ids (identity stays internal): every
// incoming Command is added as a brand-new Command with a fresh id, and on a
// name Collision the incoming copy is renamed so both are kept. `--override`
// (handled in cmd/potato) is the other mode: replace the whole Library with
// the imported file as-is.
package importer

import (
	"fmt"

	"github.com/google/uuid"
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
// it renames to `name (N)` — lowest free N from 1 — keeping both.
// Deliberately not idempotent: re-merging the same file duplicates everything.
func Merge(ours, theirs library.Library) Result {
	commands := make([]library.Entry, len(ours.Commands))
	copy(commands, ours.Commands)

	taken := map[string]bool{}
	for _, entry := range commands {
		taken[entry.Name] = true
	}

	result := Result{Added: []string{}, Renamed: []Rename{}}
	for _, entry := range theirs.Commands {
		name := entry.Name
		if taken[name] {
			n := 1
			for taken[fmt.Sprintf("%s (%d)", entry.Name, n)] {
				n++
			}
			name = fmt.Sprintf("%s (%d)", entry.Name, n)
			result.Renamed = append(result.Renamed, Rename{From: entry.Name, To: name})
		} else {
			result.Added = append(result.Added, name)
		}
		taken[name] = true

		next := entry
		next.ID = uuid.NewString()
		next.Name = name
		commands = append(commands, next)
	}

	merged := ours
	merged.Commands = commands
	result.Merged = merged
	return result
}
