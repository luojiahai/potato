// Package state owns ~/.potato/state.json — a disposable per-Command cache of
// last-used time and last Placeholder values, keyed by Command id so it
// survives renames. Unreadable state resets to empty.
package state

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Command struct {
	LastUsedAt string            `json:"lastUsedAt"`
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

func Save(path string, s State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(Serialize(s)), 0o644)
}

// Serialize matches JSON.stringify(state, null, 2) + '\n'. Keys are sorted
// rather than insertion-ordered — state.json is disposable and nothing reads
// its order.
func Serialize(s State) string {
	var b strings.Builder
	b.WriteString("{")
	for i, id := range sortedKeys(s) {
		if i > 0 {
			b.WriteString(",")
		}
		entry := s[id]
		b.WriteString(quote(id) + `:{"lastUsedAt":` + quote(entry.LastUsedAt))
		if entry.Args != nil {
			b.WriteString(`,"args":{`)
			for j, name := range sortedArgKeys(entry.Args) {
				if j > 0 {
					b.WriteString(",")
				}
				b.WriteString(quote(name) + ":" + quote(entry.Args[name]))
			}
			b.WriteString("}")
		}
		b.WriteString("}")
	}
	b.WriteString("}")

	var out bytes.Buffer
	if err := json.Indent(&out, []byte(b.String()), "", "  "); err != nil {
		return b.String() + "\n"
	}
	return out.String() + "\n"
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
	next[id] = Command{LastUsedAt: Timestamp(now), Args: merged}
	return next
}

// Timestamp matches JavaScript's Date#toISOString.
func Timestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
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

func sortedKeys(s State) []string {
	keys := make([]string, 0, len(s))
	for key := range s {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedArgKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
