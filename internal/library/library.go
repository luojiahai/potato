// Package library owns the Library: ~/.potato/commands.json (v2). A Command
// is identified by a stable `id` (a UUID); its `name` is a unique,
// human-facing field. Fail loud on anything invalid — potato never writes to a
// file it couldn't parse. Unknown fields are tolerated and preserved; array
// order is meaningful and kept (renames hold their slot, new Commands append).
//
// Mutation lives here too, behind Add / Update / Remove, and nowhere else —
// not in the edit screen, the list screen's delete, or the importer. The rules
// the parser holds a file to are the rules a mutation has to keep: minting an
// id, keeping names unique, holding a renamed Command's slot, nil-ing an empty
// description. A caller re-deriving them and getting one wrong writes a Library
// that the next launch refuses to read. `validate` is the one statement of
// those rules, and both Parse and Save run it, so what potato refuses to read
// and what it refuses to write cannot drift apart.
//
// Every Command in a Library is in normalised form, whichever door it came
// through: `commandFrom` normalises the ones a mutation makes, Parse normalises
// the ones it decodes, and `validate` holds both to it. That is what lets a
// caller move a Command between Libraries — see importer — without having to
// wonder whether the copy it read back is one Add would accept.
package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Command is one saved Command. Template is the template string — the file
// calls that key `command`, which is why the field cannot share its name.
// Extra carries any field potato does not know about, so a forward-compatible
// Library survives a round-trip through this binary.
type Command struct {
	ID          string
	Name        string
	Description *string
	Template    string
	Extra       map[string]json.RawMessage
}

// Library is the whole file. Extra carries unknown top-level fields.
type Library struct {
	Version  int
	Commands []Command
	Extra    map[string]json.RawMessage
}

// libraryKeys and commandKeys are the keys Serialize writes itself, at the two
// levels it writes them. Parse files everything else into an Extra map, and
// validate refuses one that holds a key from this list: Serialize would then
// emit that key twice, and a duplicate key is resolved last-wins on the way
// back in — a written Library that does not read back as itself.
var (
	libraryKeys = []string{"version", "commands"}
	commandKeys = []string{"id", "name", "description", "command"}
)

// Error is the fail-loud parse error; every message is prefixed with the
// source the text came from.
type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

func fail(source, reason string) error {
	return &Error{msg: fmt.Sprintf("%s: %s", source, reason)}
}

func Empty() Library { return Library{Version: 2, Commands: []Command{}} }

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
	Template    string
	Extra       map[string]json.RawMessage
}

// commandFrom normalises a Draft into the fields a Command carries and refuses
// it if the Library will not hold it: a name and a template are required, and
// the name must be free of every Command but exceptID. It does not mint an id —
// that is Add's, since Update must not.
//
// Add and Update share it, so the two verbs cannot come to different
// conclusions about the same Draft, and there is one wording for the one rule.
func commandFrom(lib Library, d Draft, exceptID string) (Command, error) {
	name := strings.TrimSpace(d.Name)
	if name == "" {
		return Command{}, fmt.Errorf("a Command needs a name")
	}
	if strings.TrimSpace(d.Template) == "" {
		return Command{}, fmt.Errorf("command %s needs a command", quote(name))
	}
	if NameTaken(lib, name, exceptID) {
		return Command{}, fmt.Errorf("%s already exists", quote(name))
	}
	cmd := Command{Name: name, Template: d.Template}
	// An empty description is an absent one, so "present but empty" is not a
	// state the Library can be in and nothing downstream has to test for both.
	if description := strings.TrimSpace(d.Description); description != "" {
		cmd.Description = &description
	}
	return cmd, nil
}

// Add appends a new Command under a freshly minted id, refusing a name another
// Command already holds. Extra is carried and cloned — an imported Command's
// unknown fields have to survive the round trip, and the source Library must
// not be left sharing the map.
func Add(lib Library, d Draft) (Library, error) {
	cmd, err := commandFrom(lib, d, "")
	if err != nil {
		return Library{}, err
	}
	cmd.ID = uuid.NewString()
	cmd.Extra = maps.Clone(d.Extra)
	next := lib
	next.Commands = append(append([]Command{}, lib.Commands...), cmd)
	return next, nil
}

// Update rewrites a Command's fields in place. The id, the array slot and the
// Command's unknown fields all stay as they were — a rename is a change of
// name, and State keyed by id and a file whose order is meaningful both depend
// on that being true.
func Update(lib Library, id string, d Draft) (Library, error) {
	cmd, err := commandFrom(lib, d, id)
	if err != nil {
		return Library{}, err
	}
	commands := make([]Command, len(lib.Commands))
	copy(commands, lib.Commands)
	found := false
	for i := range commands {
		if commands[i].ID != id {
			continue
		}
		found = true
		commands[i].Name = cmd.Name
		commands[i].Template = cmd.Template
		commands[i].Description = cmd.Description
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
// the portable Library and the throwaway cache stay independently owned.
func Remove(lib Library, id string) Library {
	commands := make([]Command, 0, len(lib.Commands))
	for _, cmd := range lib.Commands {
		if cmd.ID != id {
			commands = append(commands, cmd)
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
// This is the one statement of the rule: the add/edit form's live warning,
// commandFrom (and so Add and Update), FreeName and validate all ask it, so the
// warning cannot promise a name that the save then refuses.
//
// It is a scan, which makes validate's per-Command call O(n²) over the Library.
// That is the price of the rule having one statement instead of a fast copy in
// the parser, and at the size a hand-curated Library reaches it is not a price
// anyone can measure.
func NameTaken(lib Library, name, exceptID string) bool {
	for _, cmd := range lib.Commands {
		if cmd.Name == name && cmd.ID != exceptID {
			return true
		}
	}
	return false
}

// FreeName is want, or want with the lowest free `(N)` from 1 appended. It is
// how an Import resolves a Collision: both Commands are kept and the incoming
// one takes the suffix.
//
// It expects a name already in normalised form, which is what every Command in
// a parsed or mutated Library carries — otherwise it could report a name free
// that Add then trims onto a taken one.
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

// reservedKey is the first key in this Extra map that Serialize writes itself,
// or "".
func reservedKey(extra map[string]json.RawMessage, reserved []string) string {
	for _, key := range reserved {
		if _, held := extra[key]; held {
			return key
		}
	}
	return ""
}

// validate reports the first Library-level invariant this Library breaks, or
// "". The reason is returned rather than an error so both callers can name
// their own source: Parse the file it read, Save the file it is about to write.
func validate(lib Library) string {
	if lib.Version != 2 {
		return fmt.Sprintf("unsupported version %d (expected 2)", lib.Version)
	}
	if key := reservedKey(lib.Extra, libraryKeys); key != "" {
		return fmt.Sprintf("unknown field %s collides with one potato writes itself", quote(key))
	}

	ids := map[string]bool{}
	for i, cmd := range lib.Commands {
		if cmd.ID == "" {
			return fmt.Sprintf("command at index %d needs a non-empty %q string", i, "id")
		}
		if ids[cmd.ID] {
			return fmt.Sprintf("duplicate id %s", quote(cmd.ID))
		}
		ids[cmd.ID] = true

		if strings.TrimSpace(cmd.Name) == "" {
			return fmt.Sprintf("command %s needs a non-empty %q", quote(cmd.ID), "name")
		}
		// Normalised form, the same one commandFrom produces. A Command whose
		// name still carries the whitespace around it is one no mutation could
		// have written, and one FreeName would misjudge.
		if cmd.Name != strings.TrimSpace(cmd.Name) {
			return fmt.Sprintf("command %s has a %q with surrounding whitespace", quote(cmd.ID), "name")
		}
		// Checked against every *other* command, which is the same rule the
		// add/edit form's warning asks about.
		if NameTaken(lib, cmd.Name, cmd.ID) {
			return fmt.Sprintf("duplicate name %s", quote(cmd.Name))
		}

		if strings.TrimSpace(cmd.Template) == "" {
			return fmt.Sprintf("command %s needs a non-empty %q string", quote(cmd.Name), "command")
		}
		if cmd.Description != nil && strings.TrimSpace(*cmd.Description) == "" {
			return fmt.Sprintf("command %s has an empty %q — omit the key instead", quote(cmd.Name), "description")
		}
		if key := reservedKey(cmd.Extra, commandKeys); key != "" {
			return fmt.Sprintf("command %s has an unknown field %s that collides with one potato writes itself", quote(cmd.Name), quote(key))
		}
	}
	return ""
}

// Parse is version-strict: it parses v2 and fail-loud rejects anything else,
// including v1, which is not migrated — a rejected file is left untouched for
// the user to convert or delete.
//
// What it decodes is normalised on the way in — the name trimmed, an empty
// description dropped — so a Command read from a file is indistinguishable from
// one Add or Update made, and validate can hold every Library to one shape.
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

	rawArray, ok := top["commands"]
	if !ok {
		return Library{}, fail(source, `"commands" must be an array of objects`)
	}
	var rawCommands []json.RawMessage
	if err := json.Unmarshal(rawArray, &rawCommands); err != nil {
		return Library{}, fail(source, `"commands" must be an array of objects`)
	}

	lib := Library{Version: 2, Commands: make([]Command, 0, len(rawCommands)), Extra: map[string]json.RawMessage{}}
	for key, raw := range top {
		if !slices.Contains(libraryKeys, key) {
			lib.Extra[key] = raw
		}
	}

	for i, raw := range rawCommands {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return Library{}, fail(source, fmt.Sprintf("command at index %d must be an object", i))
		}

		cmd := Command{Extra: map[string]json.RawMessage{}}

		// id: loose — non-empty string, unique within the file (UUID format
		// not enforced). Uniqueness is validate's, below; what has to be
		// checked here is what only the raw JSON can tell you — a non-string
		// is not the same fault as an empty one.
		id, isString := decodeString(fields["id"])
		if !isString || id == "" {
			return Library{}, fail(source, fmt.Sprintf("command at index %d needs a non-empty %q string", i, "id"))
		}
		cmd.ID = id

		// name: unique, case-sensitive, non-empty after trimming — and stored
		// trimmed, so the name in hand is the name every other rule compares.
		name, isString := decodeString(fields["name"])
		if !isString || strings.TrimSpace(name) == "" {
			return Library{}, fail(source, fmt.Sprintf("command %s needs a non-empty %q", quote(id), "name"))
		}
		cmd.Name = strings.TrimSpace(name)

		// The template is stored as written — leading and trailing whitespace
		// can matter inside a shell command — but a template that is nothing
		// but whitespace is no template at all.
		template, isString := decodeString(fields["command"])
		if !isString || strings.TrimSpace(template) == "" {
			return Library{}, fail(source, fmt.Sprintf("command %s needs a non-empty %q string", quote(cmd.Name), "command"))
		}
		cmd.Template = template

		if rawDescription, present := fields["description"]; present {
			description, isString := decodeString(rawDescription)
			if !isString {
				return Library{}, fail(source, fmt.Sprintf("command %s has a non-string %q", quote(cmd.Name), "description"))
			}
			// Absent, not present-and-empty — the same rule commandFrom applies
			// to what the form types.
			if trimmed := strings.TrimSpace(description); trimmed != "" {
				cmd.Description = &trimmed
			}
		}

		for key, raw := range fields {
			if !slices.Contains(commandKeys, key) {
				cmd.Extra[key] = raw
			}
		}
		lib.Commands = append(lib.Commands, cmd)
	}
	if reason := validate(lib); reason != "" {
		return Library{}, fail(source, reason)
	}
	return lib, nil
}

// Serialize writes two-space-indented JSON with a trailing newline. Known keys
// come first in a fixed order, unknown ones after them sorted; HTML escaping is
// off, so `&&` and `>` — which are in most shell commands — stay readable.
func Serialize(lib Library) string {
	var b strings.Builder
	b.WriteString(`{"version":2,"commands":[`)
	for i, cmd := range lib.Commands {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("{")
		b.WriteString(`"id":` + quote(cmd.ID))
		b.WriteString(`,"name":` + quote(cmd.Name))
		if cmd.Description != nil {
			b.WriteString(`,"description":` + quote(*cmd.Description))
		}
		b.WriteString(`,"command":` + quote(cmd.Template))
		for _, key := range sortedKeys(cmd.Extra) {
			b.WriteString("," + quote(key) + ":" + compact(cmd.Extra[key]))
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
func Find(lib Library, id string) (Command, bool) {
	for _, cmd := range lib.Commands {
		if cmd.ID == id {
			return cmd, true
		}
	}
	return Command{}, false
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
