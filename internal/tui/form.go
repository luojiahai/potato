// A Form is the ordered Fields a screen holds, with the one focus ring that
// moves the keyboard between them.
//
// Tab is the Form's key, not a screen's. The add/edit screen and the arg screen
// each used to carry their own copy of the ring — the same modular arithmetic,
// the same two switch cases, the same forward-to-the-focused-field — which is
// two places that had to agree about what Tab does for no reason either of them
// could name. keys.go has grouped those bindings under `form` since it was
// written; this is the thing it was naming.
//
// A Form does not render. The add/edit screen lays its Fields out in labelled
// sections and the arg screen as gutter rows with hints, and those are
// genuinely different layouts over the same ring.

package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type form struct {
	fields []field
	focus  int
}

// newForm takes the Fields in tab order and hands the keyboard to the first.
func newForm(fields ...field) form {
	f := form{fields: fields}
	if len(f.fields) > 0 {
		f.fields[0].Focus()
	}
	return f
}

// Focus moves the keyboard to the i'th Field, wrapping in both directions so
// Tab off the end lands on the first and Shift-Tab off the front on the last.
func (f *form) Focus(i int) {
	if len(f.fields) == 0 {
		return
	}
	f.fields[f.focus].Blur()
	f.focus = ((i % len(f.fields)) + len(f.fields)) % len(f.fields)
	f.fields[f.focus].Focus()
}

func (f *form) Next()              { f.Focus(f.focus + 1) }
func (f *form) Prev()              { f.Focus(f.focus - 1) }
func (f *form) Focused() int       { return f.focus }
func (f *form) Field(i int) *field { return &f.fields[i] }

// Update walks the ring on Tab and Shift-Tab, and gives everything else to the
// Field that has the keyboard — including the messages that are not keys at
// all, which is how the caret's blink reaches the Field drawing it.
func (f *form) Update(msg tea.Msg) tea.Cmd {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, keymap.form.Next):
			f.Next()
			return nil
		case key.Matches(keyMsg, keymap.form.Prev):
			f.Prev()
			return nil
		}
	}
	if len(f.fields) == 0 {
		return nil
	}
	cmd, _ := f.fields[f.focus].Update(msg)
	return cmd
}
