package library

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
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
	lib.Commands = append(lib.Commands,
		Entry{ID: "a", Name: "first", Command: "ls"},
		Entry{ID: "b", Name: "second", Command: "pwd"},
	)
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
