// Every chord potato answers to, in one table. The screens match against these
// bindings and the footer is rendered from the same values' help text, so what
// the footer advertises cannot drift from what the screen reads.
//
// Why the list screen's verbs are ^N, ^O and ^X rather than the mnemonic ^A,
// ^E and ^D: the search field always has the keyboard, and a Mac terminal
// configured for natural text editing — iTerm2's preset, and the equivalents
// in VS Code, Warp and Ghostty — sends 0x01 for ⌘← and 0x05 for ⌘→, which is
// byte for byte Ctrl-A and Ctrl-E. Nothing downstream can tell them apart, so
// a field that spends ^A on `add` jumps to the add form when its user meant
// the start of the line. Every readline chord the field claims — ^A, ^E, ^K,
// ^U, ^W, and the rest — is the field's for the same reason, and the verbs are
// picked from what is left.

package tui

import "charm.land/bubbles/v2/key"

type globalKeys struct{ Cancel key.Binding }

// listKeys are the list screen's chords. The search field holds the keyboard
// for the whole life of the screen, so nothing here may be a bare letter or a
// readline chord: every key this table does not claim belongs to the field,
// which is the whole point.
type listKeys struct{ Run, Add, Edit, Delete, Copy, Up, Down, Quit, Tab key.Binding }

type confirmKeys struct{ Yes, No key.Binding }
type formKeys struct{ Next, Prev key.Binding }
type editKeys struct{ Save, Cancel key.Binding }
type argsKeys struct{ Run, Copy, Back key.Binding }

var keymap = struct {
	global  globalKeys
	list    listKeys
	confirm confirmKeys
	form    formKeys
	edit    editKeys
	args    argsKeys
}{
	// No help: ^C is the terminal's own way out of anything, not a chord this
	// app has to spend a footer slot advertising.
	global: globalKeys{Cancel: key.NewBinding(key.WithKeys("ctrl+c"))},

	list: listKeys{
		Run:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "Run")),
		Add:    key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("^N", "Add")),
		Edit:   key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("^O", "Edit")),
		Delete: key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("^X", "Delete")),
		Copy:   key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^Y", "Copy")),
		// The arrows move the selection without leaving the field, so a query
		// can be narrowed and then walked without a mode change. Unadvertised:
		// the footer's room is better spent on the keys that need announcing.
		Up:   key.NewBinding(key.WithKeys("up")),
		Down: key.NewBinding(key.WithKeys("down")),
		// Advertised last, where the footer keeps the way out visible as a
		// narrow terminal drops the chords between it and Run.
		Quit: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "Quit")),
		// Tab used to move between the screen's two keyboard zones; the zones
		// are gone and it means nothing here. Claimed so it is dropped rather
		// than reaching the field, whose sanitiser would type it as a space.
		Tab: key.NewBinding(key.WithKeys("tab", "shift+tab")),
	},

	confirm: confirmKeys{
		// Both cases spelled out: a printable key reports its own text and
		// Key.String prefers it, so a shifted y arrives as "Y", never as
		// "shift+y", and key.Matches compares strings.
		Yes: key.NewBinding(key.WithKeys("y", "Y"), key.WithHelp("y", "Delete")),
		// Any other key cancels; this binding exists so the footer can name one.
		No: key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n / esc", "Keep")),
	},

	form: formKeys{
		Next: key.NewBinding(key.WithKeys("tab", "down"), key.WithHelp("tab", "Next field")),
		Prev: key.NewBinding(key.WithKeys("shift+tab", "up")),
	},
	edit: editKeys{
		Save:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "Save")),
		Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "Cancel")),
	},
	args: argsKeys{
		Run:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "Run")),
		Copy: key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^Y", "Copy")),
		Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "Back")),
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
