// Package state owns ~/.potato/state.json — a disposable per-Command cache of
// last-used time and last Placeholder values, keyed by Command id so it
// survives renames. Unreadable state resets to empty.
package state

import (
	"bytes"
	"encoding/json"
	"os"
	"time"

	"github.com/luojiahai/potato/internal/atomicfile"
)

type Command struct {
	LastUsedAt time.Time         `json:"lastUsedAt"`
	Args       map[string]string `json:"args,omitempty"`
}

type State map[string]Command

func Load(path string) State {
	text, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	var s State
	if err := json.Unmarshal(text, &s); err != nil {
		return State{}
	}
	if s == nil {
		return State{}
	}
	return s
}

// Save writes two-space-indented JSON with a trailing newline, through
// atomicfile. encoding/json sorts map keys, so the same State always writes the
// same bytes.
func Save(path string, s State) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return err
	}
	return atomicfile.Write(path, b.Bytes(), 0o644)
}

// RecordUse stamps the Command's last use and merges the supplied arguments
// over whatever was remembered before.
func RecordUse(s State, id string, args map[string]string, now time.Time) State {
	next := State{}
	for key, value := range s {
		next[key] = value
	}
	merged := map[string]string{}
	for key, value := range s[id].Args {
		merged[key] = value
	}
	for key, value := range args {
		merged[key] = value
	}
	next[id] = Command{LastUsedAt: now.UTC(), Args: merged}
	return next
}

// Forget drops a Command's cache entry, for when the Command itself is gone.
// Nothing breaks if it is never called — State is disposable and an orphaned
// key is only dead weight — but a Library and a State that disagree about which
// Commands exist is dead weight that accumulates for as long as potato is
// installed.
func Forget(s State, id string) State {
	next := State{}
	for key, value := range s {
		if key != id {
			next[key] = value
		}
	}
	return next
}
