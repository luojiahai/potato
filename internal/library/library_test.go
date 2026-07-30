package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Library v2: an array of UUID-identified entries { id, name, description?,
// command }; name is unique and case-sensitive, id is loosely validated
// (non-empty + unique). Version-strict — v1 and unknown versions fail loud.
// Unknown fields tolerated and preserved; saves are 2-space pretty, array
// order kept.

const valid = `{"version":2,"commands":[{"id":"b3f1c2a4-5d6e-4f80-9a1b-2c3d4e5f6a7b",` +
	`"name":"deploy prod","description":"Roll out to production","command":"ssh {{host=prod-1}} 'deploy.sh'"}]}`

func TestParseValid(t *testing.T) {
	lib, err := Parse(valid, "commands.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entry := lib.Commands[0]
	if entry.Name != "deploy prod" {
		t.Errorf("name = %q", entry.Name)
	}
	if entry.Command != "ssh {{host=prod-1}} 'deploy.sh'" {
		t.Errorf("command = %q", entry.Command)
	}
	if entry.Description == nil || *entry.Description != "Roll out to production" {
		t.Errorf("description = %v", entry.Description)
	}
}

func TestParseAllowsNonUUIDID(t *testing.T) {
	lib, err := Parse(`{"version":2,"commands":[{"id":"not-a-uuid","name":"x","command":"ls"}]}`, "f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lib.Commands[0].ID != "not-a-uuid" {
		t.Errorf("id = %q", lib.Commands[0].ID)
	}
}

func TestParseFailsLoud(t *testing.T) {
	cases := []struct{ label, text string }{
		{"bad JSON", "{ nope"},
		{"missing version", `{"commands": []}`},
		{"v1 is rejected version-strict", `{"version": 1, "commands": {}}`},
		{"future version", `{"version": 3, "commands": []}`},
		{"commands not an array", `{"version": 2, "commands": {}}`},
		{"entry missing id", `{"version": 2, "commands": [{"name": "x", "command": "ls"}]}`},
		{"entry with empty id", `{"version": 2, "commands": [{"id": "", "name": "x", "command": "ls"}]}`},
		{"duplicate id", `{"version": 2, "commands": [{"id":"a","name":"x","command":"ls"},{"id":"a","name":"y","command":"pwd"}]}`},
		{"duplicate name", `{"version": 2, "commands": [{"id":"a","name":"x","command":"ls"},{"id":"b","name":"x","command":"pwd"}]}`},
		{"name empty after trimming", `{"version": 2, "commands": [{"id":"a","name":"  ","command":"ls"}]}`},
		{"entry missing command", `{"version": 2, "commands": [{"id":"a","name":"x"}]}`},
		{"entry with empty command", `{"version": 2, "commands": [{"id":"a","name":"x","command":""}]}`},
		{"non-string description", `{"version": 2, "commands": [{"id":"a","name":"x","command":"ls","description":3}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			_, err := Parse(tc.text, "commands.json")
			if err == nil {
				t.Fatal("expected a failure")
			}
			if _, ok := err.(*Error); !ok {
				t.Errorf("error is %T, want *library.Error", err)
			}
			if !strings.Contains(err.Error(), "commands.json") {
				t.Errorf("error does not name the source: %v", err)
			}
		})
	}
}

func TestParseNamesAreCaseSensitive(t *testing.T) {
	lib, err := Parse(`{"version":2,"commands":[{"id":"a","name":"deploy","command":"ls"},{"id":"b","name":"Deploy","command":"pwd"}]}`, "f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lib.Commands) != 2 {
		t.Errorf("got %d commands, want 2", len(lib.Commands))
	}
}

func TestSerializeRoundTripsUnknownFields(t *testing.T) {
	text := `{"version":2,"color":"blue","commands":[{"id":"a","name":"x","command":"ls","note":"keep me"}]}`
	lib, err := Parse(text, "f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out struct {
		Color    string `json:"color"`
		Commands []struct {
			Note string `json:"note"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(Serialize(lib)), &out); err != nil {
		t.Fatalf("serialized output is not JSON: %v", err)
	}
	if out.Color != "blue" {
		t.Errorf("top-level unknown field lost: %q", out.Color)
	}
	if out.Commands[0].Note != "keep me" {
		t.Errorf("entry unknown field lost: %q", out.Commands[0].Note)
	}
}

func TestSerializePrettyPrintsAndKeepsOrder(t *testing.T) {
	lib, err := Parse(`{"version":2,"commands":[{"id":"a","name":"zeta","command":"z"},{"id":"b","name":"alpha","command":"a"}]}`, "f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := Serialize(lib)
	if !strings.Contains(out, `  "commands"`) {
		t.Error("not indented with two spaces")
	}
	if strings.Index(out, "zeta") > strings.Index(out, "alpha") {
		t.Error("array order not preserved")
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("missing trailing newline")
	}
}

// Go's encoding/json escapes &, < and > by default, which would rewrite most
// shell commands into unreadable escapes on the first save.
func TestSerializeDoesNotEscapeShellCharacters(t *testing.T) {
	lib, err := Parse(`{"version":2,"commands":[{"id":"a","name":"x","command":"a && b > c < d"}]}`, "f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := Serialize(lib)
	if !strings.Contains(out, `a && b > c < d`) {
		t.Errorf("shell characters were escaped:\n%s", out)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	lib, err := Load(filepath.Join(t.TempDir(), "commands.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lib.Version != 2 || len(lib.Commands) != 0 {
		t.Errorf("got %+v, want an empty v2 library", lib)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "commands.json")
	lib, err := Load(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lib = add(t, lib, Draft{Name: "first", Command: "ls"})
	lib = add(t, lib, Draft{Name: "second", Command: "pwd"})
	if err := Save(file, lib); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(file)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var names []string
	for _, entry := range loaded.Commands {
		names = append(names, entry.Name)
	}
	if strings.Join(names, ",") != "first,second" {
		t.Errorf("got %v, want [first second]", names)
	}
}

// ---------- the write interface ----------

func add(t *testing.T, lib Library, d Draft) Library {
	t.Helper()
	next, err := Add(lib, d)
	if err != nil {
		t.Fatalf("Add(%q): %v", d.Name, err)
	}
	return next
}

func TestAddMintsAnIDAndAppends(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "first", Command: "ls"})
	lib = add(t, lib, Draft{Name: "second", Command: "pwd"})

	if len(lib.Commands) != 2 {
		t.Fatalf("got %d commands, want 2", len(lib.Commands))
	}
	if lib.Commands[0].Name != "first" || lib.Commands[1].Name != "second" {
		t.Errorf("new Commands did not append in order: %+v", lib.Commands)
	}
	for _, entry := range lib.Commands {
		if _, err := uuid.Parse(entry.ID); err != nil {
			t.Errorf("id %q is not a UUID: %v", entry.ID, err)
		}
	}
	if lib.Commands[0].ID == lib.Commands[1].ID {
		t.Error("two Commands were minted the same id")
	}
}

func TestAddDoesNotMutateTheLibraryItWasGiven(t *testing.T) {
	before := add(t, Empty(), Draft{Name: "first", Command: "ls"})
	after := add(t, before, Draft{Name: "second", Command: "pwd"})

	if len(before.Commands) != 1 {
		t.Errorf("the original grew to %d commands", len(before.Commands))
	}
	if len(after.Commands) != 2 {
		t.Errorf("the result has %d commands, want 2", len(after.Commands))
	}
}

func TestAddTrimsAndDropsAnEmptyDescription(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "  spaced  ", Command: "ls", Description: "   "})
	entry := lib.Commands[0]
	if entry.Name != "spaced" {
		t.Errorf("name = %q, want it trimmed", entry.Name)
	}
	if entry.Description != nil {
		t.Errorf("description = %q, want absent", *entry.Description)
	}

	lib = add(t, lib, Draft{Name: "described", Command: "ls", Description: "  why  "})
	if got := lib.Commands[1].Description; got == nil || *got != "why" {
		t.Errorf("description = %v, want a trimmed %q", got, "why")
	}
}

func TestAddCarriesAndClonesExtra(t *testing.T) {
	extra := map[string]json.RawMessage{"note": json.RawMessage(`"keep me"`)}
	lib := add(t, Empty(), Draft{Name: "x", Command: "ls", Extra: extra})

	if string(lib.Commands[0].Extra["note"]) != `"keep me"` {
		t.Errorf("Extra was not carried: %v", lib.Commands[0].Extra)
	}
	// The Draft's map must not still be the Command's, or the source Library an
	// import came from would share it.
	extra["note"] = json.RawMessage(`"mutated"`)
	if string(lib.Commands[0].Extra["note"]) != `"keep me"` {
		t.Error("Extra was aliased rather than cloned")
	}
}

func TestAddRefusesWhatParseWouldReject(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "taken", Command: "ls"})
	for _, tc := range []struct {
		label string
		d     Draft
	}{
		{"an empty name", Draft{Name: "", Command: "ls"}},
		{"a whitespace-only name", Draft{Name: "   ", Command: "ls"}},
		{"an empty command", Draft{Name: "fine", Command: ""}},
		{"a whitespace-only command", Draft{Name: "fine", Command: "  "}},
		{"a name already taken", Draft{Name: "taken", Command: "ls"}},
		{"a name taken after trimming", Draft{Name: "  taken  ", Command: "ls"}},
	} {
		t.Run(tc.label, func(t *testing.T) {
			if _, err := Add(lib, tc.d); err == nil {
				t.Error("expected a refusal")
			}
		})
	}
}

func TestUpdateKeepsTheIDTheSlotAndExtra(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "first", Command: "ls",
		Extra: map[string]json.RawMessage{"note": json.RawMessage(`"mine"`)}})
	lib = add(t, lib, Draft{Name: "second", Command: "pwd"})
	id := lib.Commands[0].ID

	next, err := Update(lib, id, Draft{Name: "renamed", Command: "ls -la"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(next.Commands) != 2 {
		t.Fatalf("got %d commands, want 2", len(next.Commands))
	}
	entry := next.Commands[0]
	if entry.ID != id {
		t.Errorf("id changed to %q", entry.ID)
	}
	if entry.Name != "renamed" || entry.Command != "ls -la" {
		t.Errorf("fields not applied: %+v", entry)
	}
	if next.Commands[1].Name != "second" {
		t.Error("the rename moved the Command out of its slot")
	}
	// The form knows nothing about unknown fields, so it must not be able to
	// drop them by saving.
	if string(entry.Extra["note"]) != `"mine"` {
		t.Errorf("Extra was lost on update: %v", entry.Extra)
	}
}

func TestUpdateAllowsRenamingACommandToItsOwnName(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "same", Command: "ls"})
	if _, err := Update(lib, lib.Commands[0].ID, Draft{Name: "same", Command: "ls -la"}); err != nil {
		t.Errorf("a Command could not keep its own name: %v", err)
	}
}

func TestUpdateRefusesAnotherCommandsName(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "first", Command: "ls"})
	lib = add(t, lib, Draft{Name: "second", Command: "pwd"})
	if _, err := Update(lib, lib.Commands[0].ID, Draft{Name: "second", Command: "ls"}); err == nil {
		t.Error("expected a refusal")
	}
}

func TestUpdateRefusesAnUnknownID(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "first", Command: "ls"})
	if _, err := Update(lib, "nope", Draft{Name: "x", Command: "ls"}); err == nil {
		t.Error("expected a refusal")
	}
}

func TestRemoveDropsOnlyThatCommand(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "first", Command: "ls"})
	lib = add(t, lib, Draft{Name: "second", Command: "pwd"})
	id := lib.Commands[0].ID

	next := Remove(lib, id)
	if len(next.Commands) != 1 || next.Commands[0].Name != "second" {
		t.Errorf("commands = %+v", next.Commands)
	}
	if len(lib.Commands) != 2 {
		t.Error("Remove mutated the Library it was given")
	}
	// An unknown id is not a fault — the Command is already gone.
	if got := Remove(next, "nope"); len(got.Commands) != 1 {
		t.Errorf("removing an unknown id changed the Library: %+v", got.Commands)
	}
}

func TestNameTakenHonoursTheException(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "first", Command: "ls"})
	lib = add(t, lib, Draft{Name: "second", Command: "pwd"})
	first, second := lib.Commands[0].ID, lib.Commands[1].ID

	if !NameTaken(lib, "first", "") {
		t.Error(`"first" reads as free`)
	}
	if NameTaken(lib, "first", first) {
		t.Error("a Command's own name reads as taken from itself")
	}
	if !NameTaken(lib, "first", second) {
		t.Error("another Command's name reads as free")
	}
	if NameTaken(lib, "third", "") {
		t.Error(`"third" reads as taken`)
	}
	// Names are case-sensitive, as Parse has them.
	if NameTaken(lib, "First", "") {
		t.Error("the name check is case-insensitive")
	}
}

func TestFreeNameTakesTheLowestFreeSuffix(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "x", Command: "ls"})
	if got := FreeName(lib, "untaken"); got != "untaken" {
		t.Errorf("FreeName of a free name = %q", got)
	}
	if got := FreeName(lib, "x"); got != "x (1)" {
		t.Errorf("FreeName = %q, want %q", got, "x (1)")
	}

	lib = add(t, lib, Draft{Name: "x (1)", Command: "ls"})
	lib = add(t, lib, Draft{Name: "x (3)", Command: "ls"})
	if got := FreeName(lib, "x"); got != "x (2)" {
		t.Errorf("FreeName = %q, want the lowest free %q", got, "x (2)")
	}
}

func TestFindReturnsACopy(t *testing.T) {
	lib := add(t, Empty(), Draft{Name: "first", Command: "ls"})
	id := lib.Commands[0].ID

	entry, ok := Find(lib, id)
	if !ok || entry.Name != "first" {
		t.Fatalf("Find = %+v, %v", entry, ok)
	}
	// A read is not a way to write: mutating what Find handed back must not
	// reach the Library, which would be a mutation past Add and Update.
	entry.Name = "hijacked"
	if lib.Commands[0].Name != "first" {
		t.Error("Find handed back a pointer into the Library")
	}
	if _, ok := Find(lib, "nope"); ok {
		t.Error("an unknown id was found")
	}
}

// ---------- the round-trip guarantee ----------

// Whatever the write interface produces, Save will accept and Parse will read
// back. This is the promise the interface exists for: mutation used to live in
// four callers, each re-deriving the rules the parser holds the file to, and any
// one of them getting it wrong wrote a Library that the next launch refused.
func TestEveryMutationSequenceStaysReadable(t *testing.T) {
	lib := Empty()
	steps := []struct {
		label string
		apply func(Library) Library
	}{
		{"add one", func(l Library) Library { return add(t, l, Draft{Name: "alpha", Command: "ls"}) }},
		{"add a described one", func(l Library) Library {
			return add(t, l, Draft{Name: "beta", Command: "pwd", Description: "why"})
		}},
		{"add one carrying unknown fields", func(l Library) Library {
			return add(t, l, Draft{Name: "gamma", Command: "df -h",
				Extra: map[string]json.RawMessage{"note": json.RawMessage(`"keep"`)}})
		}},
		{"add a colliding name via FreeName", func(l Library) Library {
			return add(t, l, Draft{Name: FreeName(l, "alpha"), Command: "ls -la"})
		}},
		{"rename the first", func(l Library) Library {
			next, err := Update(l, l.Commands[0].ID, Draft{Name: "renamed", Command: "ls"})
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			return next
		}},
		{"drop a description", func(l Library) Library {
			next, err := Update(l, l.Commands[1].ID, Draft{Name: "beta", Command: "pwd"})
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			return next
		}},
		{"remove the middle one", func(l Library) Library { return Remove(l, l.Commands[1].ID) }},
	}

	file := filepath.Join(t.TempDir(), "commands.json")
	for _, step := range steps {
		lib = step.apply(lib)
		// Save is the guard; that it accepts is half the promise.
		if err := Save(file, lib); err != nil {
			t.Fatalf("after %q, Save refused what the write interface produced: %v", step.label, err)
		}
		// Reading it back is the other half.
		loaded, err := Load(file)
		if err != nil {
			t.Fatalf("after %q, the saved Library would not parse: %v", step.label, err)
		}
		if len(loaded.Commands) != len(lib.Commands) {
			t.Fatalf("after %q, %d commands were saved and %d read back",
				step.label, len(lib.Commands), len(loaded.Commands))
		}
		for i, want := range lib.Commands {
			got := loaded.Commands[i]
			if got.ID != want.ID || got.Name != want.Name || got.Command != want.Command {
				t.Errorf("after %q, command %d changed on the round trip:\n got %+v\nwant %+v",
					step.label, i, got, want)
			}
		}
	}
}

// The other half of the same promise: Save refuses a Library that Parse would
// reject, so a caller that hand-builds one cannot leave an unreadable file
// behind. Nothing in potato writes these — that is the point.
func TestSaveRefusesAnUnreadableLibrary(t *testing.T) {
	description := "d"
	for _, tc := range []struct {
		label string
		lib   Library
	}{
		{"a duplicate name", Library{Version: 2, Commands: []Entry{
			{ID: "a", Name: "x", Command: "ls"},
			{ID: "b", Name: "x", Command: "pwd"},
		}}},
		{"a duplicate id", Library{Version: 2, Commands: []Entry{
			{ID: "a", Name: "x", Command: "ls"},
			{ID: "a", Name: "y", Command: "pwd"},
		}}},
		{"an empty id", Library{Version: 2, Commands: []Entry{
			{ID: "", Name: "x", Command: "ls"},
		}}},
		{"an empty name", Library{Version: 2, Commands: []Entry{
			{ID: "a", Name: "  ", Command: "ls", Description: &description},
		}}},
		{"an empty command", Library{Version: 2, Commands: []Entry{
			{ID: "a", Name: "x", Command: ""},
		}}},
	} {
		t.Run(tc.label, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "commands.json")
			err := Save(file, tc.lib)
			if err == nil {
				t.Fatal("Save wrote a Library that Parse would reject")
			}
			if _, ok := err.(*Error); !ok {
				t.Errorf("error is %T, want *library.Error", err)
			}
			if !strings.Contains(err.Error(), file) {
				t.Errorf("error does not name the file it refused to write: %v", err)
			}
			if _, statErr := os.Stat(file); !os.IsNotExist(statErr) {
				t.Error("a refused Save still touched the disk")
			}
		})
	}
}
