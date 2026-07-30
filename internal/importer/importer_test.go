package importer

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/luojiahai/potato/internal/library"
)

// Import v2: a simple local merge. Every incoming Command is added as a new
// Command with a fresh id, appended in incoming order; on a name Collision the
// incoming copy is renamed `name (N)` (case-sensitive) so both are kept.
// Nothing is skipped or overwritten. Ids never match across files.

func ours() library.Library {
	return library.Library{Version: 2, Commands: []library.Entry{
		{ID: "o1", Name: "alpha", Command: "echo ours-alpha"},
		{ID: "o2", Name: "beta", Command: "echo beta"},
	}}
}

var uuidish = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-`)

func names(lib library.Library) []string {
	out := make([]string, 0, len(lib.Commands))
	for _, entry := range lib.Commands {
		out = append(out, entry.Name)
	}
	return out
}

func find(lib library.Library, name string) *library.Entry {
	for i := range lib.Commands {
		if lib.Commands[i].Name == name {
			return &lib.Commands[i]
		}
	}
	return nil
}

func TestMergeAddsUnknownNames(t *testing.T) {
	theirs := library.Library{Version: 2, Commands: []library.Entry{
		{ID: "t1", Name: "zeta", Command: "echo z"},
		{ID: "t2", Name: "gamma", Command: "echo g"},
	}}
	result := Merge(ours(), theirs)

	want := []string{"alpha", "beta", "zeta", "gamma"}
	got := names(result.Merged)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if len(result.Added) != 2 || result.Added[0] != "zeta" || result.Added[1] != "gamma" {
		t.Errorf("added = %v", result.Added)
	}
	if len(result.Renamed) != 0 {
		t.Errorf("renamed = %v", result.Renamed)
	}

	zeta := find(result.Merged, "zeta")
	if zeta.ID == "t1" {
		t.Error("the incoming id was kept; it must be replaced")
	}
	if !uuidish.MatchString(zeta.ID) {
		t.Errorf("id %q is not a UUID", zeta.ID)
	}
}

func TestMergeRenamesOnCollision(t *testing.T) {
	theirs := library.Library{Version: 2, Commands: []library.Entry{
		{ID: "t1", Name: "alpha", Command: "echo theirs-alpha"},
	}}
	result := Merge(ours(), theirs)

	if len(result.Renamed) != 1 || result.Renamed[0] != (Rename{From: "alpha", To: "alpha (1)"}) {
		t.Errorf("renamed = %v", result.Renamed)
	}
	if len(result.Added) != 0 {
		t.Errorf("added = %v", result.Added)
	}
	if got := find(result.Merged, "alpha"); got == nil || got.Command != "echo ours-alpha" {
		t.Error("ours was not left untouched")
	}
	if got := find(result.Merged, "alpha (1)"); got == nil || got.Command != "echo theirs-alpha" {
		t.Error("theirs did not come in renamed")
	}
}

func TestMergePicksLowestFreeSuffix(t *testing.T) {
	base := library.Library{Version: 2, Commands: []library.Entry{
		{ID: "o1", Name: "ship", Command: "a"},
		{ID: "o2", Name: "ship (1)", Command: "b"},
	}}
	theirs := library.Library{Version: 2, Commands: []library.Entry{{ID: "t1", Name: "ship", Command: "c"}}}
	result := Merge(base, theirs)
	if len(result.Renamed) != 1 || result.Renamed[0].To != "ship (2)" {
		t.Errorf("renamed = %v, want ship → ship (2)", result.Renamed)
	}
}

// An incoming Command's description and unknown fields have to survive being
// re-homed. Merge hands the Library a Draft rather than an Entry, so both have
// to be carried across explicitly — a conversion that dropped either would lose
// data from a file written by a newer potato.
func TestMergeCarriesDescriptionsAndUnknownFields(t *testing.T) {
	description := "what it is for"
	theirs := library.Library{Version: 2, Commands: []library.Entry{{
		ID: "t1", Name: "zeta", Command: "echo z",
		Description: &description,
		Extra:       map[string]json.RawMessage{"note": json.RawMessage(`"from the future"`)},
	}}}
	result := Merge(ours(), theirs)

	zeta := find(result.Merged, "zeta")
	if zeta == nil {
		t.Fatal("the incoming Command is missing")
	}
	if zeta.Description == nil || *zeta.Description != description {
		t.Errorf("description = %v, want %q", zeta.Description, description)
	}
	if string(zeta.Extra["note"]) != `"from the future"` {
		t.Errorf("unknown field lost: %v", zeta.Extra)
	}
	// The merged Command must not share the map with the file it came from.
	theirs.Commands[0].Extra["note"] = json.RawMessage(`"mutated"`)
	if string(zeta.Extra["note"]) != `"from the future"` {
		t.Error("Extra is aliased to the incoming Library")
	}
}

// Two incoming Commands colliding with each other take successive suffixes.
// This is what makes asking FreeName per Command — against the running merged
// Library — the right shape, rather than seeding a name set up front.
func TestMergeResolvesCollisionsAmongTheIncomingCommands(t *testing.T) {
	theirs := library.Library{Version: 2, Commands: []library.Entry{
		{ID: "t1", Name: "alpha", Command: "echo one"},
		{ID: "t2", Name: "alpha", Command: "echo two"},
	}}
	result := Merge(ours(), theirs)

	if len(result.Renamed) != 2 {
		t.Fatalf("renamed = %v, want two renames", result.Renamed)
	}
	if result.Renamed[0].To != "alpha (1)" || result.Renamed[1].To != "alpha (2)" {
		t.Errorf("renamed = %v, want successive suffixes", result.Renamed)
	}
	if got := find(result.Merged, "alpha (1)"); got == nil || got.Command != "echo one" {
		t.Errorf("alpha (1) = %+v", got)
	}
	if got := find(result.Merged, "alpha (2)"); got == nil || got.Command != "echo two" {
		t.Errorf("alpha (2) = %+v", got)
	}
}

// Merge does not mutate the Library it was given.
func TestMergeLeavesOursAlone(t *testing.T) {
	base := ours()
	theirs := library.Library{Version: 2, Commands: []library.Entry{{ID: "t1", Name: "zeta", Command: "echo z"}}}
	Merge(base, theirs)
	if len(base.Commands) != 2 {
		t.Errorf("ours grew to %d commands", len(base.Commands))
	}
}

// Deliberately not idempotent: re-merging the same file duplicates everything.
func TestMergeIsNotIdempotent(t *testing.T) {
	theirs := library.Library{Version: 2, Commands: []library.Entry{{ID: "t1", Name: "zeta", Command: "echo z"}}}
	once := Merge(ours(), theirs)
	twice := Merge(once.Merged, theirs)
	if len(twice.Merged.Commands) != 4 {
		t.Errorf("got %d commands, want 4: %v", len(twice.Merged.Commands), names(twice.Merged))
	}
}
