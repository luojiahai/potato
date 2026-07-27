package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/state"
)

// v1 → v2 migration: fresh UUIDs, a name→id map that also rekeys state, and an
// atomic auto-on-load upgrade that leaves the v1 file intact on failure.

func parseV1(t *testing.T, text string) LibraryV1 {
	t.Helper()
	v1, err := ParseV1(text, "commands.json")
	if err != nil {
		t.Fatalf("ParseV1: %v", err)
	}
	return v1
}

func TestMigratePreservesOrderAndMintsIDs(t *testing.T) {
	v1 := parseV1(t, `{"version":1,"commands":{"deploy":{"command":"ssh prod","description":"roll out"},"list":{"command":"ls"}}}`)
	lib, nameToID := Migrate(v1)

	if lib.Version != 2 {
		t.Errorf("version = %d", lib.Version)
	}
	if len(lib.Commands) != 2 || lib.Commands[0].Name != "deploy" || lib.Commands[1].Name != "list" {
		t.Fatalf("order not preserved: %+v", lib.Commands)
	}
	if lib.Commands[0].Command != "ssh prod" {
		t.Errorf("command = %q", lib.Commands[0].Command)
	}
	if lib.Commands[0].Description == nil || *lib.Commands[0].Description != "roll out" {
		t.Errorf("description = %v", lib.Commands[0].Description)
	}
	if lib.Commands[1].Description != nil {
		t.Errorf("description should be absent, got %v", *lib.Commands[1].Description)
	}
	if nameToID["deploy"] != lib.Commands[0].ID {
		t.Error("name→id map does not match the minted id")
	}
	if lib.Commands[0].ID == lib.Commands[1].ID {
		t.Error("ids are not unique")
	}
}

func TestMigratePreservesUnknownFields(t *testing.T) {
	v1 := parseV1(t, `{"version":1,"color":"blue","commands":{"x":{"command":"ls","note":"keep"}}}`)
	lib, _ := Migrate(v1)
	if string(lib.Extra["color"]) != `"blue"` {
		t.Errorf("top-level unknown field = %s", lib.Extra["color"])
	}
	if string(lib.Commands[0].Extra["note"]) != `"keep"` {
		t.Errorf("entry unknown field = %s", lib.Commands[0].Extra["note"])
	}
}

// v1 tolerated arbitrary unknown fields; reserved-looking ones must not
// override the minted id, the map-keyed name, or the command.
func TestMigrateStrayReservedFieldsCannotClobberIdentity(t *testing.T) {
	v1 := parseV1(t, `{"version":1,"commands":{"a":{"command":"echo a","id":"evil","name":"spoof"},"b":{"command":"echo b","id":"evil"}}}`)
	lib, nameToID := Migrate(v1)
	a, b := lib.Commands[0], lib.Commands[1]

	if a.ID == "evil" || a.ID == b.ID {
		t.Errorf("minted ids did not win: %q %q", a.ID, b.ID)
	}
	if nameToID["a"] != a.ID {
		t.Error("name→id map does not match")
	}
	if a.Name != "a" || a.Command != "echo a" {
		t.Errorf("name/command clobbered: %q %q", a.Name, a.Command)
	}
	if _, err := library.Parse(library.Serialize(lib), "migrated"); err != nil {
		t.Errorf("migrated library does not round-trip: %v", err)
	}
}

func TestRekeyStateDropsOrphans(t *testing.T) {
	s := state.State{"deploy": {LastUsedAt: "2026-01-01T00:00:00Z"}, "gone": {LastUsedAt: "x"}}
	next := RekeyState(s, map[string]string{"deploy": "id-1"})
	if next["id-1"].LastUsedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("state not rekeyed: %v", next)
	}
	if len(next) != 1 {
		t.Errorf("orphan kept: %v", next)
	}
}

func TestLoadMigratesV1OnDisk(t *testing.T) {
	dir := t.TempDir()
	commands := filepath.Join(dir, "commands.json")
	statePath := filepath.Join(dir, "state.json")
	if err := os.WriteFile(commands, []byte(`{"version":1,"commands":{"deploy":{"command":"ssh prod"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte(`{"deploy":{"lastUsedAt":"2026-07-01T00:00:00Z"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Load(commands, statePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !result.Migrated {
		t.Error("Migrated flag not set")
	}
	id := result.Library.Commands[0].ID
	if result.State[id].LastUsedAt != "2026-07-01T00:00:00Z" {
		t.Errorf("state was not rekeyed onto the new id: %v", result.State)
	}

	// the file on disk is now v2 and parses strictly
	raw, err := os.ReadFile(commands)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"version": 2`) {
		t.Errorf("file was not upgraded:\n%s", raw)
	}
	if _, err := library.Parse(string(raw), commands); err != nil {
		t.Errorf("upgraded file does not parse: %v", err)
	}
}

func TestLoadLeavesV2Alone(t *testing.T) {
	dir := t.TempDir()
	commands := filepath.Join(dir, "commands.json")
	if err := os.WriteFile(commands, []byte(`{"version":2,"commands":[{"id":"a","name":"x","command":"ls"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Load(commands, filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if result.Migrated {
		t.Error("a v2 library should not report a migration")
	}
	if result.Library.Commands[0].ID != "a" {
		t.Error("v2 ids must be left alone")
	}
}

func TestLoadFailsLoudOnAFutureVersion(t *testing.T) {
	dir := t.TempDir()
	commands := filepath.Join(dir, "commands.json")
	if err := os.WriteFile(commands, []byte(`{"version":3,"commands":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(commands, filepath.Join(dir, "state.json")); err == nil {
		t.Error("expected a failure on an unknown version")
	}
}
