// Package library owns the Library: ~/.potato/commands.json (v2). A Command
// is identified by a stable `id` (a UUID); its `name` is a unique,
// human-facing field. Fail loud on anything invalid — potato never writes to a
// file it couldn't parse. Unknown fields are tolerated and preserved; array
// order is meaningful and kept (renames hold their slot, new entries append).
//
// Mutation lives here too, behind Add / Update / Remove. It used to live in
// whichever caller needed it — the edit screen, the list screen's delete, the
// importer — and each of them re-derived the rules the parser would hold the
// file to: minting an id, keeping names unique, holding a renamed Command's
// slot, nil-ing an empty description. One of them getting it wrong would have
// written a Library that the next launch refused to read. Now `validate` is
// the one statement of those rules, and both Parse and Save run it, so what
// potato refuses to read and what it refuses to write cannot drift apart.
package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
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

func fail(source, reason string) error {
	return &Error{msg: fmt.Sprintf("%s: %s", source, reason)}
}

func Empty() Library { return Library{Version: 2, Commands: []Entry{}} }

// Draft is a Command's content without its identity: a name, an optional
// description, and a template string. It is what the add/edit form holds and
// what Add and Update accept; a Draft becomes a Command when the Library gives
// it an id.
//
// Extra carries the unknown fields of a Command being re-homed from another
// Library — see Add. Update ignores it, because the Command it is updating
// already has its own and a form that knows nothing about unknown fields must
// not be able to drop them.
type Draft struct {
	Name        string
	Description string
	Command     string
	Extra       map[string]json.RawMessage
}

// draftEntry normalises a Draft into the Entry fields a Command carries: the
// name trimmed, the description trimmed and absent when empty. It does not
// mint an id — that is Add's, since Update must not.
func draftEntry(d Draft) (Entry, error) {
	name := strings.TrimSpace(d.Name)
	if name == "" {
		return Entry{}, fmt.Errorf("a Command needs a name")
	}
	if strings.TrimSpace(d.Command) == "" {
		return Entry{}, fmt.Errorf("command %s needs a command", quote(name))
	}
	entry := Entry{Name: name, Command: d.Command}
	// An empty description is an absent one, so "present but empty" is not a
	// state the Library can be in and nothing downstream has to test for both.
	if description := strings.TrimSpace(d.Description); description != "" {
		entry.Description = &description
	}
	return entry, nil
}

// Add appends a new Command under a freshly minted id, refusing a name another
// Command already holds. Extra is carried and cloned — an imported Command's
// unknown fields have to survive the round trip, and the source Library must
// not be left sharing the map.
func Add(lib Library, d Draft) (Library, error) {
	entry, err := draftEntry(d)
	if err != nil {
		return Library{}, err
	}
	if NameTaken(lib, entry.Name, "") {
		return Library{}, fmt.Errorf("%s already exists", quote(entry.Name))
	}
	entry.ID = uuid.NewString()
	entry.Extra = maps.Clone(d.Extra)
	next := lib
	next.Commands = append(append([]Entry{}, lib.Commands...), entry)
	return next, nil
}

// Update rewrites a Command's fields in place. The id, the array slot and the
// Command's unknown fields all stay as they were — a rename is a change of
// name, and State keyed by id and a file whose order is meaningful both depend
// on that being true.
func Update(lib Library, id string, d Draft) (Library, error) {
	entry, err := draftEntry(d)
	if err != nil {
		return Library{}, err
	}
	if NameTaken(lib, entry.Name, id) {
		return Library{}, fmt.Errorf("%s already exists", quote(entry.Name))
	}
	commands := make([]Entry, len(lib.Commands))
	copy(commands, lib.Commands)
	found := false
	for i := range commands {
		if commands[i].ID != id {
			continue
		}
		found = true
		commands[i].Name = entry.Name
		commands[i].Command = entry.Command
		commands[i].Description = entry.Description
	}
	if !found {
		return Library{}, fmt.Errorf("no Command with id %s", quote(id))
	}
	next := lib
	next.Commands = commands
	return next, nil
}

// Remove drops a Command from the Library and nothing else. An unknown id is
// not an error — the Command is already gone, which is what the caller wanted.
//
// State is a separate, disposable file: callers pair this with state.Forget so
// the portable Library and the throwaway cache stay independently owned. See
// docs/adr/0002-library-does-not-prune-state.md.
func Remove(lib Library, id string) Library {
	commands := make([]Entry, 0, len(lib.Commands))
	for _, entry := range lib.Commands {
		if entry.ID != id {
			commands = append(commands, entry)
		}
	}
	next := lib
	next.Commands = commands
	return next
}

// NameTaken reports whether some Command other than exceptID already holds
// this name. Pass "" for exceptID when adding; pass the Command's own id when
// updating, so renaming a Command to the name it already has is allowed.
//
// This is the one statement of the rule: the add/edit form's live warning, Add,
// Update, FreeName and validate all ask it, so the warning cannot promise a
// name that the save then refuses.
func NameTaken(lib Library, name, exceptID string) bool {
	for _, entry := range lib.Commands {
		if entry.Name == name && entry.ID != exceptID {
			return true
		}
	}
	return false
}

// FreeName is want, or want with the lowest free `(N)` from 1 appended. It is
// how an Import resolves a Collision: both Commands are kept and the incoming
// one takes the suffix.
func FreeName(lib Library, want string) string {
	if !NameTaken(lib, want, "") {
		return want
	}
	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s (%d)", want, n)
		if !NameTaken(lib, candidate, "") {
			return candidate
		}
	}
}

// validate reports the first Library-level invariant this Library breaks, or
// "". The reason is returned rather than an error so both callers can name
// their own source: Parse the file it read, Save the file it is about to write.
func validate(lib Library) string {
	ids := map[string]bool{}
	for i, entry := range lib.Commands {
		if entry.ID == "" {
			return fmt.Sprintf("command at index %d needs a non-empty %q string", i, "id")
		}
		if ids[entry.ID] {
			return fmt.Sprintf("duplicate id %s", quote(entry.ID))
		}
		ids[entry.ID] = true

		if strings.TrimSpace(entry.Name) == "" {
			return fmt.Sprintf("command %s needs a non-empty %q", quote(entry.ID), "name")
		}
		// Checked against every *other* entry, which is the same rule the
		// add/edit form's warning asks about.
		if NameTaken(lib, entry.Name, entry.ID) {
			return fmt.Sprintf("duplicate name %s", quote(entry.Name))
		}

		if entry.Command == "" {
			return fmt.Sprintf("command %s needs a non-empty %q string", quote(entry.Name), "command")
		}
	}
	return ""
}

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

	for i, raw := range rawEntries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return Library{}, fail(source, fmt.Sprintf("command at index %d must be an object", i))
		}

		entry := Entry{Extra: map[string]json.RawMessage{}}

		// id: loose — non-empty string, unique within the file (UUID format
		// not enforced). Uniqueness is validate's, below; what has to be
		// checked here is what only the raw JSON can tell you — a non-string
		// is not the same fault as an empty one.
		id, isString := decodeString(fields["id"])
		if !isString || id == "" {
			return Library{}, fail(source, fmt.Sprintf("command at index %d needs a non-empty %q string", i, "id"))
		}
		entry.ID = id

		// name: unique, case-sensitive, non-empty after trimming
		name, isString := decodeString(fields["name"])
		if !isString || strings.TrimSpace(name) == "" {
			return Library{}, fail(source, fmt.Sprintf("command %s needs a non-empty %q", quote(id), "name"))
		}
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
	if reason := validate(lib); reason != "" {
		return Library{}, fail(source, reason)
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
// untouched, so a crashed save never corrupts the Library.
//
// It refuses a Library that Parse would reject, before touching the disk. The
// mutations that reach here all come from Add / Update / Remove and are valid
// by construction, so this fires only on a Library some caller hand-built —
// which is exactly the case worth catching, because the cost of writing one is
// a file potato cannot read on the next launch.
func Save(path string, lib Library) error {
	if reason := validate(lib); reason != "" {
		return fail(path, reason)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, []byte(Serialize(lib)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Find returns the Command with this id. It hands back a copy rather than a
// pointer into the Commands slice: a read must not be a way to write past
// Add / Update, and comma-ok is a contract a caller cannot forget to honour
// the way a nil pointer can.
func Find(lib Library, id string) (Entry, bool) {
	for _, entry := range lib.Commands {
		if entry.ID == id {
			return entry, true
		}
	}
	return Entry{}, false
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
