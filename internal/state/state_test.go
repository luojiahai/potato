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
	if loaded["cmd-1"].LastUsedAt != "2026-07-24T09:12:00.000Z" {
		t.Errorf("lastUsedAt = %q", loaded["cmd-1"].LastUsedAt)
	}
	if loaded["cmd-1"].Args["host"] != "prod-2" {
		t.Errorf("args = %v", loaded["cmd-1"].Args)
	}
}

func TestRecordUseMergesArgs(t *testing.T) {
	s := RecordUse(State{}, "x", map[string]string{"a": "1", "b": "2"}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s = RecordUse(s, "x", map[string]string{"b": "3"}, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if s["x"].LastUsedAt != "2026-02-01T00:00:00.000Z" {
		t.Errorf("lastUsedAt = %q", s["x"].LastUsedAt)
	}
	if s["x"].Args["a"] != "1" || s["x"].Args["b"] != "3" {
		t.Errorf("args = %v, want a=1 b=3", s["x"].Args)
	}
}
