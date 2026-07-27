// Package library owns the Library: ~/.potato/commands.json (v2). A Command
// is identified by a stable `id` (a UUID); its `name` is a unique,
// human-facing field. Fail loud on anything invalid — potato never writes to a
// file it couldn't parse. Unknown fields are tolerated and preserved; array
// order is meaningful and kept (renames hold their slot, new entries append).
package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one Command. Extra carries any field potato does not know about,
// so a forward-compatible Library survives a round-trip through this binary.
type Entry struct {
	ID          string
	Name        string
	Description *string
	Command     string
	Extra       map[string]json.RawMessage
}

// Library is the whole file. Extra carries unknown top-level fields.
type Library struct {
	Version  int
	Commands []Entry
	Extra    map[string]json.RawMessage
}

// Error is the fail-loud parse error; every message is prefixed with the
// source the text came from.
type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

// NewError builds the same source-prefixed fail-loud error the parser
// raises, for the v1 reader in package migrate.
func NewError(source, reason string) error {
	return &Error{msg: fmt.Sprintf("%s: %s", source, reason)}
}

func fail(source, reason string) error { return NewError(source, reason) }

func Empty() Library { return Library{Version: 2, Commands: []Entry{}} }

// Parse is version-strict: it parses v2 and fail-loud rejects anything else,
// including v1 (v1 files are upgraded by the migration loader, not here).
func Parse(text, source string) (Library, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &top); err != nil {
		var any json.RawMessage
		if json.Unmarshal([]byte(text), &any) == nil {
			return Library{}, fail(source, "top level must be an object")
		}
		return Library{}, fail(source, fmt.Sprintf("not valid JSON (%s)", err))
	}

	rawVersion, ok := top["version"]
	versionText := "undefined"
	if ok {
		versionText = compact(rawVersion)
	}
	var version int
	if !ok || json.Unmarshal(rawVersion, &version) != nil || version != 2 {
		return Library{}, fail(source, fmt.Sprintf("unsupported version %s (expected 2)", versionText))
	}

	rawCommands, ok := top["commands"]
	if !ok {
		return Library{}, fail(source, `"commands" must be an array of entries`)
	}
	var rawEntries []json.RawMessage
	if err := json.Unmarshal(rawCommands, &rawEntries); err != nil {
		return Library{}, fail(source, `"commands" must be an array of entries`)
	}

	lib := Library{Version: 2, Commands: make([]Entry, 0, len(rawEntries)), Extra: map[string]json.RawMessage{}}
	for key, raw := range top {
		if key != "version" && key != "commands" {
			lib.Extra[key] = raw
		}
	}

	ids := map[string]bool{}
	names := map[string]bool{}
	for i, raw := range rawEntries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return Library{}, fail(source, fmt.Sprintf("command at index %d must be an object", i))
		}

		entry := Entry{Extra: map[string]json.RawMessage{}}

		// id: loose — non-empty string, unique within the file (UUID format
		// not enforced)
		id, isString := decodeString(fields["id"])
		if !isString || id == "" {
			return Library{}, fail(source, fmt.Sprintf("command at index %d needs a non-empty %q string", i, "id"))
		}
		if ids[id] {
			return Library{}, fail(source, fmt.Sprintf("duplicate id %s", quote(id)))
		}
		ids[id] = true
		entry.ID = id

		// name: unique, case-sensitive, non-empty after trimming
		name, isString := decodeString(fields["name"])
		if !isString || strings.TrimSpace(name) == "" {
			return Library{}, fail(source, fmt.Sprintf("command %s needs a non-empty %q", quote(id), "name"))
		}
		if names[name] {
			return Library{}, fail(source, fmt.Sprintf("duplicate name %s", quote(name)))
		}
		names[name] = true
		entry.Name = name

		command, isString := decodeString(fields["command"])
		if !isString || command == "" {
			return Library{}, fail(source, fmt.Sprintf("command %s needs a non-empty %q string", quote(name), "command"))
		}
		entry.Command = command

		if rawDescription, present := fields["description"]; present {
			description, isString := decodeString(rawDescription)
			if !isString {
				return Library{}, fail(source, fmt.Sprintf("command %s has a non-string %q", quote(name), "description"))
			}
			entry.Description = &description
		}

		for key, raw := range fields {
			switch key {
			case "id", "name", "description", "command":
			default:
				entry.Extra[key] = raw
			}
		}
		lib.Commands = append(lib.Commands, entry)
	}
	return lib, nil
}

// Serialize writes the two-space-indented JSON potato has always written,
// with a trailing newline. Known keys come first in a fixed order, unknown
// ones after them sorted; HTML escaping is off, so `&&` and `>` — which are
// in most shell commands — stay readable.
func Serialize(lib Library) string {
	var b strings.Builder
	b.WriteString(`{"version":2,"commands":[`)
	for i, entry := range lib.Commands {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("{")
		b.WriteString(`"id":` + quote(entry.ID))
		b.WriteString(`,"name":` + quote(entry.Name))
		if entry.Description != nil {
			b.WriteString(`,"description":` + quote(*entry.Description))
		}
		b.WriteString(`,"command":` + quote(entry.Command))
		for _, key := range sortedKeys(entry.Extra) {
			b.WriteString("," + quote(key) + ":" + compact(entry.Extra[key]))
		}
		b.WriteString("}")
	}
	b.WriteString("]")
	for _, key := range sortedKeys(lib.Extra) {
		b.WriteString("," + quote(key) + ":" + compact(lib.Extra[key]))
	}
	b.WriteString("}")

	var out bytes.Buffer
	if err := json.Indent(&out, []byte(b.String()), "", "  "); err != nil {
		return b.String() + "\n"
	}
	return out.String() + "\n"
}

func Load(path string) (Library, error) {
	text, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Empty(), nil
	}
	if err != nil {
		return Library{}, err
	}
	return Parse(string(text), path)
}

// Save writes atomically (temp + rename): a failed write leaves the original
// untouched, so a crashed migration or save never corrupts the Library.
func Save(path string, lib Library) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, []byte(Serialize(lib)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func FindByID(lib Library, id string) *Entry {
	for i := range lib.Commands {
		if lib.Commands[i].ID == id {
			return &lib.Commands[i]
		}
	}
	return nil
}

// ---------- JSON helpers ----------

// quote renders a Go string as a JSON string without HTML escaping, matching
// JSON.stringify.
func quote(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return `""`
	}
	return strings.TrimRight(buf.String(), "\n")
}

func compact(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
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

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
