// Package migrate holds the v1 → v2 migration. v1 keyed Commands by name; v2
// identifies them by a stable UUID with the name as a unique field (see
// package library). Migration runs auto-on-load: an entry point reading the
// Library detects `version: 1`, upgrades it in memory, and writes the v2 file
// back atomically. The name→id map minted here — the only moment it exists —
// also rekeys state.json so last-used history survives the upgrade.
package migrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/state"
)

// ---------- retained v1 reader ----------

type EntryV1 struct {
	Command     string
	Description *string
	Extra       map[string]json.RawMessage
}

type LibraryV1 struct {
	// Names holds the command names in file order; Commands maps each to its
	// entry. v1's `commands` was an object, and its key order is the array
	// order the migrated Library inherits.
	Names    []string
	Commands map[string]EntryV1
	Extra    map[string]json.RawMessage
}

// ParseV1 parses a v1 file, fail-loud on anything invalid — the same guarantee
// the v1 loader gave, kept so migration only ever upgrades a file it fully
// understood.
func ParseV1(text, source string) (LibraryV1, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &top); err != nil {
		var any json.RawMessage
		if json.Unmarshal([]byte(text), &any) == nil {
			return LibraryV1{}, failV1(source, "top level must be an object")
		}
		return LibraryV1{}, failV1(source, fmt.Sprintf("not valid JSON (%s)", err))
	}

	var version int
	raw, ok := top["version"]
	if !ok || json.Unmarshal(raw, &version) != nil || version != 1 {
		return LibraryV1{}, failV1(source, "expected a version 1 library")
	}

	rawCommands, ok := top["commands"]
	if !ok {
		return LibraryV1{}, failV1(source, `"commands" must be an object keyed by command name`)
	}
	pairs, err := orderedObject(rawCommands)
	if err != nil {
		return LibraryV1{}, failV1(source, `"commands" must be an object keyed by command name`)
	}

	out := LibraryV1{Commands: map[string]EntryV1{}, Extra: map[string]json.RawMessage{}}
	for key, raw := range top {
		if key != "version" && key != "commands" {
			out.Extra[key] = raw
		}
	}

	for _, pair := range pairs {
		name := pair.key
		if strings.TrimSpace(name) == "" {
			return LibraryV1{}, failV1(source, "command names must be non-empty after trimming")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(pair.raw, &fields); err != nil {
			return LibraryV1{}, failV1(source, fmt.Sprintf("command %s must be an object", quote(name)))
		}
		entry := EntryV1{Extra: map[string]json.RawMessage{}}
		command, isString := decodeString(fields["command"])
		if !isString || command == "" {
			return LibraryV1{}, failV1(source, fmt.Sprintf("command %s needs a non-empty %q string", quote(name), "command"))
		}
		entry.Command = command
		if rawDescription, present := fields["description"]; present {
			description, isString := decodeString(rawDescription)
			if !isString {
				return LibraryV1{}, failV1(source, fmt.Sprintf("command %s has a non-string %q", quote(name), "description"))
			}
			entry.Description = &description
		}
		for key, raw := range fields {
			switch key {
			case "command", "description":
			default:
				entry.Extra[key] = raw
			}
		}
		out.Names = append(out.Names, name)
		out.Commands[name] = entry
	}
	return out, nil
}

// ---------- pure migration ----------

// Migrate assigns each v1 Command a fresh UUID, producing the v2 array. It
// also returns the name→id map so state.json can be rekeyed in the same
// operation.
func Migrate(v1 LibraryV1) (library.Library, map[string]string) {
	nameToID := map[string]string{}
	commands := make([]library.Entry, 0, len(v1.Names))
	for _, name := range v1.Names {
		entry := v1.Commands[name]
		id := uuid.NewString()
		nameToID[name] = id
		// Extra is carried, but the minted id, the map-keyed name and the
		// command can never be clobbered by a stray v1 field named
		// id/name/command (v1 tolerated arbitrary unknowns) — that would
		// desync the name→id map or mint a duplicate id that fails to
		// re-parse.
		extra := map[string]json.RawMessage{}
		for key, raw := range entry.Extra {
			switch key {
			case "id", "name", "command", "description":
			default:
				extra[key] = raw
			}
		}
		commands = append(commands, library.Entry{
			ID:          id,
			Name:        name,
			Description: entry.Description,
			Command:     entry.Command,
			Extra:       extra,
		})
	}
	return library.Library{Version: 2, Commands: commands, Extra: v1.Extra}, nameToID
}

// RekeyState moves each State entry from its name key to the new id. Orphans —
// a state key whose Command no longer exists — are dropped; State is
// disposable.
func RekeyState(s state.State, nameToID map[string]string) state.State {
	next := state.State{}
	for name, entry := range s {
		if id, ok := nameToID[name]; ok {
			next[id] = entry
		}
	}
	return next
}

// ---------- load orchestration ----------

type Result struct {
	Library  library.Library
	State    state.State
	Migrated bool
}

// Load is the Library load path for every app entry point: read → if v1,
// migrate + write both files back → parse as v2. A v1 Library's commands.json
// write is atomic and authoritative (fail-loud, original preserved on
// failure); the state.json rekey rides along as best-effort (its loss is
// disposable).
func Load(commandsPath, statePath string) (Result, error) {
	s := state.Load(statePath)
	text, err := os.ReadFile(commandsPath)
	if os.IsNotExist(err) {
		return Result{Library: library.Empty(), State: s}, nil
	}
	if err != nil {
		return Result{}, err
	}

	if peekVersion(string(text)) == 1 {
		v1, err := ParseV1(string(text), commandsPath)
		if err != nil {
			return Result{}, err
		}
		lib, nameToID := Migrate(v1)
		// atomic + authoritative — fail loud, leave v1 intact on failure
		if err := library.Save(commandsPath, lib); err != nil {
			return Result{}, err
		}
		rekeyed := RekeyState(s, nameToID)
		// best-effort: a lost state rewrite only forfeits disposable timestamps
		_ = state.Save(statePath, rekeyed)
		return Result{Library: lib, State: rekeyed, Migrated: true}, nil
	}

	// v2, or an unknown/future version — library.Parse fail-louds on the latter.
	lib, err := library.Parse(string(text), commandsPath)
	if err != nil {
		return Result{}, err
	}
	return Result{Library: lib, State: s}, nil
}

// peekVersion is a cheap look at the version to route the load; a malformed
// file returns 0 and falls through to library.Parse, which reports the real
// error.
func peekVersion(text string) int {
	var top struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal([]byte(text), &top); err != nil {
		return 0
	}
	return top.Version
}

// ---------- helpers ----------

func failV1(source, reason string) error { return library.NewError(source, reason) }

type pair struct {
	key string
	raw json.RawMessage
}

// orderedObject decodes a JSON object preserving key order, which v1's
// name-keyed `commands` map depends on for the migrated array order.
func orderedObject(raw json.RawMessage) ([]pair, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("not an object")
	}
	var out []pair
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("bad key")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		out = append(out, pair{key: key, raw: value})
	}
	if _, err := dec.Token(); err != nil && err != io.EOF {
		return nil, err
	}
	return out, nil
}

func decodeString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func quote(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return `""`
	}
	return strings.TrimRight(buf.String(), "\n")
}
