package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// state.json is a disposable per-Command cache, keyed by Command id —
// unreadable means silently reset to empty. LastUsedAt drives MRU; Args are
// the last Placeholder values.

func TestLoadMissingFile(t *testing.T) {
	if s := Load(filepath.Join(t.TempDir(), "state.json")); len(s) != 0 {
		t.Errorf("got %v, want empty", s)
	}
}

func TestLoadCorruptFileResets(t *testing.T) {
	for _, content := range []string{"{ nope", "[1, 2]"} {
		file := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if s := Load(file); len(s) != 0 {
			t.Errorf("%q loaded as %v, want empty", content, s)
		}
	}
}

func TestRecordUseRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	s := Load(file)
	s = RecordUse(s, "cmd-1", map[string]string{"host": "prod-2"}, time.Date(2026, 7, 24, 9, 12, 0, 0, time.UTC))
	if err := Save(file, s); err != nil {
		t.Fatal(err)
	}
	loaded := Load(file)
	if want := time.Date(2026, 7, 24, 9, 12, 0, 0, time.UTC); !loaded["cmd-1"].LastUsedAt.Equal(want) {
		t.Errorf("lastUsedAt = %v, want %v", loaded["cmd-1"].LastUsedAt, want)
	}
	if loaded["cmd-1"].Args["host"] != "prod-2" {
		t.Errorf("args = %v", loaded["cmd-1"].Args)
	}
}

func TestForgetDropsOnlyThatCommand(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := RecordUse(State{}, "keep", map[string]string{"a": "1"}, now)
	s = RecordUse(s, "drop", nil, now)

	next := Forget(s, "drop")
	if _, ok := next["drop"]; ok {
		t.Error("the forgotten Command is still cached")
	}
	if _, ok := next["keep"]; !ok {
		t.Error("Forget dropped a Command it was not asked about")
	}
	if _, ok := s["drop"]; !ok {
		t.Error("Forget mutated the State it was given")
	}
	// Forgetting what was never cached is not a fault.
	if got := Forget(next, "never"); len(got) != 1 {
		t.Errorf("forgetting an unknown id changed the State: %v", got)
	}
}

func TestRecordUseMergesArgs(t *testing.T) {
	s := RecordUse(State{}, "x", map[string]string{"a": "1", "b": "2"}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s = RecordUse(s, "x", map[string]string{"b": "3"}, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if want := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC); !s["x"].LastUsedAt.Equal(want) {
		t.Errorf("lastUsedAt = %v, want %v", s["x"].LastUsedAt, want)
	}
	if s["x"].Args["a"] != "1" || s["x"].Args["b"] != "3" {
		t.Errorf("args = %v, want a=1 b=3", s["x"].Args)
	}
}

func TestLoadReadsMillisecondTimestamps(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	content := `{"a":{"lastUsedAt":"2026-07-24T09:12:00.000Z","args":{"host":"prod-2"}}}`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded := Load(file)
	if want := time.Date(2026, 7, 24, 9, 12, 0, 0, time.UTC); !loaded["a"].LastUsedAt.Equal(want) {
		t.Errorf("lastUsedAt = %v, want %v", loaded["a"].LastUsedAt, want)
	}
	if loaded["a"].Args["host"] != "prod-2" {
		t.Errorf("args = %v", loaded["a"].Args)
	}
}

func TestSaveWritesSortedIndentedJSON(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	now := time.Date(2026, 7, 24, 9, 12, 0, 0, time.FixedZone("AEST", 10*60*60))
	s := RecordUse(State{}, "b", map[string]string{"z": "a && b", "a": "<x>"}, now)
	s = RecordUse(s, "a", nil, now)
	if err := Save(file, s); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "a": {
    "lastUsedAt": "2026-07-23T23:12:00Z"
  },
  "b": {
    "lastUsedAt": "2026-07-23T23:12:00Z",
    "args": {
      "a": "<x>",
      "z": "a && b"
    }
  }
}
`
	if string(got) != want {
		t.Errorf("state.json =\n%s\nwant\n%s", got, want)
	}
}
