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
	return library.Library{Version: 2, Commands: []library.Command{
		{ID: "o1", Name: "alpha", Template: "echo ours-alpha"},
		{ID: "o2", Name: "beta", Template: "echo beta"},
	}}
}

var uuidish = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-`)

func names(lib library.Library) []string {
	out := make([]string, 0, len(lib.Commands))
	for _, command := range lib.Commands {
		out = append(out, command.Name)
	}
	return out
}

// mustMerge is Merge for the cases that must not fail — which is every merge of
// two Libraries the Library itself would accept.
func mustMerge(t *testing.T, ours, theirs library.Library) Result {
	t.Helper()
	result, err := Merge(ours, theirs)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	return result
}

func find(lib library.Library, name string) *library.Command {
	for i := range lib.Commands {
		if lib.Commands[i].Name == name {
			return &lib.Commands[i]
		}
	}
	return nil
}

func TestMergeAddsUnknownNames(t *testing.T) {
	theirs := library.Library{Version: 2, Commands: []library.Command{
		{ID: "t1", Name: "zeta", Template: "echo z"},
		{ID: "t2", Name: "gamma", Template: "echo g"},
	}}
	result := mustMerge(t, ours(), theirs)

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
	theirs := library.Library{Version: 2, Commands: []library.Command{
		{ID: "t1", Name: "alpha", Template: "echo theirs-alpha"},
	}}
	result := mustMerge(t, ours(), theirs)

	if len(result.Renamed) != 1 || result.Renamed[0] != (Rename{From: "alpha", To: "alpha (1)"}) {
		t.Errorf("renamed = %v", result.Renamed)
	}
	if len(result.Added) != 0 {
		t.Errorf("added = %v", result.Added)
	}
	if got := find(result.Merged, "alpha"); got == nil || got.Template != "echo ours-alpha" {
		t.Error("ours was not left untouched")
	}
	if got := find(result.Merged, "alpha (1)"); got == nil || got.Template != "echo theirs-alpha" {
		t.Error("theirs did not come in renamed")
	}
}

func TestMergePicksLowestFreeSuffix(t *testing.T) {
	base := library.Library{Version: 2, Commands: []library.Command{
		{ID: "o1", Name: "ship", Template: "a"},
		{ID: "o2", Name: "ship (1)", Template: "b"},
	}}
	theirs := library.Library{Version: 2, Commands: []library.Command{{ID: "t1", Name: "ship", Template: "c"}}}
	result := mustMerge(t, base, theirs)
	if len(result.Renamed) != 1 || result.Renamed[0].To != "ship (2)" {
		t.Errorf("renamed = %v, want ship → ship (2)", result.Renamed)
	}
}

// An incoming Command's description and unknown fields have to survive being
// re-homed. Merge hands the Library a Draft rather than a Command, so both have
// to be carried across explicitly — a conversion that dropped either would lose
// data from a file written by a newer potato.
func TestMergeCarriesDescriptionsAndUnknownFields(t *testing.T) {
	description := "what it is for"
	theirs := library.Library{Version: 2, Commands: []library.Command{{
		ID: "t1", Name: "zeta", Template: "echo z",
		Description: &description,
		Extra:       map[string]json.RawMessage{"note": json.RawMessage(`"from the future"`)},
	}}}
	result := mustMerge(t, ours(), theirs)

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
	theirs := library.Library{Version: 2, Commands: []library.Command{
		{ID: "t1", Name: "alpha", Template: "echo one"},
		{ID: "t2", Name: "alpha", Template: "echo two"},
	}}
	result := mustMerge(t, ours(), theirs)

	if len(result.Renamed) != 2 {
		t.Fatalf("renamed = %v, want two renames", result.Renamed)
	}
	if result.Renamed[0].To != "alpha (1)" || result.Renamed[1].To != "alpha (2)" {
		t.Errorf("renamed = %v, want successive suffixes", result.Renamed)
	}
	if got := find(result.Merged, "alpha (1)"); got == nil || got.Template != "echo one" {
		t.Errorf("alpha (1) = %+v", got)
	}
	if got := find(result.Merged, "alpha (2)"); got == nil || got.Template != "echo two" {
		t.Errorf("alpha (2) = %+v", got)
	}
}

// Merge does not mutate the Library it was given.
func TestMergeLeavesOursAlone(t *testing.T) {
	base := ours()
	theirs := library.Library{Version: 2, Commands: []library.Command{{ID: "t1", Name: "zeta", Template: "echo z"}}}
	mustMerge(t, base, theirs)
	if len(base.Commands) != 2 {
		t.Errorf("ours grew to %d commands", len(base.Commands))
	}
}

// A padded incoming name must not be able to smuggle a Collision past FreeName.
// Parse stores names trimmed, so `  alpha  ` arrives as `alpha`, FreeName sees
// the Collision and both are kept. Were the name stored as written, FreeName
// would have found it free, Add would have trimmed it onto a name that is taken
// and refused it — one Command lost from a merge that reported success, against
// CONTEXT.md's promise for `--merge` that nothing is lost.
func TestMergeKeepsACommandWhoseNameIsPaddedInTheFile(t *testing.T) {
	theirs, err := library.Parse(`{"version":2,"commands":[{"id":"t1","name":"  alpha  ","command":"echo padded"}]}`, "theirs.json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	result := mustMerge(t, ours(), theirs)

	if len(result.Merged.Commands) != 3 {
		t.Fatalf("got %d commands, want 3: %v", len(result.Merged.Commands), names(result.Merged))
	}
	if len(result.Renamed) != 1 || result.Renamed[0] != (Rename{From: "alpha", To: "alpha (1)"}) {
		t.Errorf("renamed = %v, want alpha → alpha (1)", result.Renamed)
	}
	if got := find(result.Merged, "alpha (1)"); got == nil || got.Template != "echo padded" {
		t.Errorf("alpha (1) = %+v", got)
	}
}

// Merge fails rather than dropping. Reaching this takes a hand-built Library —
// Parse refuses a whitespace-only template — which is the point: a Command the
// Library will not accept stops the merge, so the caller dies before writing
// instead of saving a Library that quietly lost one.
func TestMergeFailsRatherThanDroppingACommand(t *testing.T) {
	theirs := library.Library{Version: 2, Commands: []library.Command{
		{ID: "t1", Name: "zeta", Template: "echo z"},
		{ID: "t2", Name: "hollow", Template: "   "},
	}}
	if _, err := Merge(ours(), theirs); err == nil {
		t.Fatal("Merge reported success on a Command the Library refuses")
	}
}

// Deliberately not idempotent: re-merging the same file duplicates everything.
func TestMergeIsNotIdempotent(t *testing.T) {
	theirs := library.Library{Version: 2, Commands: []library.Command{{ID: "t1", Name: "zeta", Template: "echo z"}}}
	once := mustMerge(t, ours(), theirs)
	twice := mustMerge(t, once.Merged, theirs)
	if len(twice.Merged.Commands) != 4 {
		t.Errorf("got %d commands, want 4: %v", len(twice.Merged.Commands), names(twice.Merged))
	}
}
