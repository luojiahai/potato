// Every chord potato answers to, in one table. The screens match against these
// bindings and the footer is rendered from the same values' help text, so what
// the footer advertises cannot drift from what the screen reads.
//
// Why the list screen's verbs are bare letters: a Mac terminal configured for
// natural text editing — iTerm2's preset, and the equivalents in VS Code, Warp
// and Ghostty — sends 0x01 for ⌘← and 0x05 for ⌘→, which is byte for byte
// Ctrl-A and Ctrl-E. Nothing downstream can tell them apart, so a search field
// that spends ^A on `add` jumps to the add form when its user meant the start
// of the line. The verbs move to the zone where the keyboard is not spelling
// anything, and there a bare letter is free.
//
// The zones are separate types rather than one flat struct because that is the
// invariant: a binding whose key set holds a bare letter must be unreachable
// from the search zone. `y` copies from the list; `^Y` copies from the field.

package tui

import "charm.land/bubbles/v2/key"

type globalKeys struct{ Cancel key.Binding }

// searchKeys are the list screen's chords while the search field holds the
// keyboard. Nothing here may be a bare letter: every key this zone does not
// claim belongs to the field, which is the whole point.
type searchKeys struct{ Run, Copy, Actions, Quit, Up, Down key.Binding }

// listKeys are the same screen's chords while the results hold the keyboard.
type listKeys struct{ Run, Add, Edit, Delete, Copy, Up, Down, Search, Quit key.Binding }

type confirmKeys struct{ Yes, No key.Binding }
type formKeys struct{ Next, Prev key.Binding }
type editKeys struct{ Save, Cancel key.Binding }
type argsKeys struct{ Run, Copy, Back key.Binding }

var keymap = struct {
	global  globalKeys
	search  searchKeys
	list    listKeys
	confirm confirmKeys
	form    formKeys
	edit    editKeys
	args    argsKeys
}{
	// No help: ^C is the terminal's own way out of anything, not a chord this
	// app has to spend a footer slot advertising.
	global: globalKeys{Cancel: key.NewBinding(key.WithKeys("ctrl+c"))},

	search: searchKeys{
		Run:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "run")),
		Copy:    key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^Y", "copy")),
		Actions: key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "actions")),
		Quit:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "quit")),
		// The arrows move the selection without leaving the field, so a query
		// can be narrowed and then walked without a mode change. Unadvertised:
		// the footer's room is better spent on the keys that need announcing.
		Up:   key.NewBinding(key.WithKeys("up")),
		Down: key.NewBinding(key.WithKeys("down")),
	},

	list: listKeys{
		Run:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "run")),
		Add:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
		Edit:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Copy:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy")),
		Up:     key.NewBinding(key.WithKeys("up", "k")),
		Down:   key.NewBinding(key.WithKeys("down", "j")),
		Search: key.NewBinding(key.WithKeys("tab", "shift+tab", "esc"), key.WithHelp("esc", "search")),
		// Bound but not advertised: the footer's last chord has to be the way
		// back to the field, and esc from there is the way out of potato. A
		// seventh chord would make `esc search` the first thing a narrow
		// terminal drops.
		Quit: key.NewBinding(key.WithKeys("q")),
	},

	confirm: confirmKeys{
		// Both cases spelled out: a printable key reports its own text and
		// Key.String prefers it, so a shifted y arrives as "Y", never as
		// "shift+y", and key.Matches compares strings.
		Yes: key.NewBinding(key.WithKeys("y", "Y"), key.WithHelp("y", "delete")),
		// Any other key cancels; this binding exists so the footer can name one.
		No: key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n / esc", "keep")),
	},

	form: formKeys{
		Next: key.NewBinding(key.WithKeys("tab", "down"), key.WithHelp("tab", "next field")),
		Prev: key.NewBinding(key.WithKeys("shift+tab", "up")),
	},
	edit: editKeys{
		Save:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "save")),
		Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	},
	args: argsKeys{
		Run:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "run")),
		Copy: key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^Y", "copy")),
		Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	},
}

// footerKeys is the only way a footerKey is made: a binding's own help, so the
// footer cannot advertise a chord the screen does not answer to. A binding with
// no help — ^C, the arrows, q — is bound without being announced.
func footerKeys(bindings ...key.Binding) []footerKey {
	out := make([]footerKey, 0, len(bindings))
	for _, b := range bindings {
		if h := b.Help(); b.Enabled() && h.Key != "" {
			out = append(out, h)
		}
	}
	return out
}
