// The add / edit screen: one ruled section per field, each sized to what it
// holds, stacked from the top.
//
// No section is given a share of a fixed height; every one is as tall as its
// own content, so the command field grows downward as you type and pushes
// `placeholders` along with it, and the slack collects above the footer where
// nothing frames it.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/placeholders"
)

// Field order, and therefore Tab order. Command sits last because it is the
// only field that grows, so it expands into the free space at the bottom of
// the screen instead of shoving the other two down as you type.
const (
	fieldName = iota
	fieldDescription
	fieldCommand
	fieldCount
)

var editLabels = [fieldCount]string{
	fieldName:        "Name",
	fieldDescription: "Description",
	fieldCommand:     "Command",
}

// commandHint is what the command field shows while it is empty. It lives on
// the field rather than in a section of its own, so it costs no row and
// vanishes on the first keystroke.
const commandHint = "Type a command — {{name}} or {{name=default}} become args"

type editScreen struct {
	id string // "" = a new Command
	// back is the screen this one was opened from, and the one both ways out of
	// it lead to. Held rather than rebuilt so the list is found as it was left.
	back screen
	form form
	// initial is what the fields were opened with, which is what makes an edit
	// distinguishable from a form that has only been looked at.
	initial [fieldCount]string
	// tried records a save that was refused. Until then the form stays quiet
	// about the fields it is still waiting for — a brand-new Command is empty
	// by definition, and greeting it with "name is required" is noise.
	tried bool
	// discardArmed is an esc waiting for its second press. See update.
	discardArmed bool
}

func newEditScreen(m *Model, back screen, command *library.Command) *editScreen {
	s := &editScreen{back: back}
	var values [fieldCount]string
	if command != nil {
		s.id = command.ID
		values[fieldName] = command.Name
		values[fieldCommand] = command.Template
		if command.Description != nil {
			values[fieldDescription] = *command.Description
		}
	}
	s.initial = values
	fields := make([]field, 0, fieldCount)
	for i, value := range values {
		f := newField(wrapMode)
		// Only the command field highlights Placeholders — it is the only one
		// where `{{name}}` means anything — and only it has a hint to offer.
		if i == fieldCommand {
			f.paint = func(value string, _ bool) []run { return templateRuns(value) }
			f.hint = commandHint
		}
		f.SetValue(value)
		fields = append(fields, f)
	}
	s.form = newForm(fields...)
	return s
}

func (s *editScreen) value(i int) string { return s.form.Field(i).Value() }

// dirty reports whether any field has moved off what the screen opened with —
// which is the whole of what the esc guard in update is protecting.
func (s *editScreen) dirty() bool {
	for i, was := range s.initial {
		if s.value(i) != was {
			return true
		}
	}
	return false
}

// collision is the warning for a name another Command already holds, or "". It
// is the one problem worth saying before a save is even attempted — the user
// cannot see it coming — so the view raises it on its own and `problem` reports
// it in turn, both from here so there is one wording for the one rule.
//
// The rule itself is the Library's — asking library.NameTaken is what keeps the
// warning and the refusal from being able to disagree. The wording stays
// potato's, because the Library phrases its refusals for a file and this is a
// sentence under a field.
func (s *editScreen) collision(m *Model) string {
	name := strings.TrimSpace(s.value(fieldName))
	if name == "" || !library.NameTaken(m.lib, name, s.id) {
		return ""
	}
	return fmt.Sprintf("'%s' already exists", name)
}

// problem is the first reason this Command cannot be saved, or "".
func (s *editScreen) problem(m *Model) string {
	if strings.TrimSpace(s.value(fieldName)) == "" {
		return "Name is required"
	}
	if taken := s.collision(m); taken != "" {
		return taken
	}
	if strings.TrimSpace(s.value(fieldCommand)) == "" {
		return "Command is required"
	}
	return ""
}

// update claims the screen's own two verbs; the ring and the typing are the
// Form's.
func (s *editScreen) update(m *Model, msg tea.Msg) tea.Cmd {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		// esc is the reflexive way out of anything, and a form with edits in it
		// is the one place that reflex destroys work. So a dirty form spends the
		// first esc saying what it is about to lose and leaves on the second; a
		// form nothing has been typed into has nothing to lose and goes at once.
		if key.Matches(keyMsg, keymap.edit.Cancel) {
			if s.dirty() && !s.discardArmed {
				s.discardArmed = true
				return nil
			}
			m.screen = s.back
			return nil
		}
		// Any other keystroke is the user carrying on with the form, so the armed
		// esc is spent rather than left lying in wait several keys later.
		s.discardArmed = false
		if key.Matches(keyMsg, keymap.edit.Save) {
			if s.problem(m) != "" {
				// No flash: the inline warning says the same thing and stays put
				// until the problem is fixed, where a toast would expire while the
				// field it is about is still empty.
				s.tried = true
				return nil
			}
			return s.save(m)
		}
	}
	return s.form.Update(msg)
}

// save hands the form's fields to the Library and goes back where it came from.
// The trimming, the id, the slot and the empty-description rule are all the
// Library's — this knows only which of the two verbs it is performing.
func (s *editScreen) save(m *Model) tea.Cmd {
	draft := library.Draft{
		Name:        s.value(fieldName),
		Description: s.value(fieldDescription),
		Template:    s.value(fieldCommand),
	}

	var (
		next library.Library
		err  error
	)
	verb := "Saved"
	if s.id == "" {
		verb = "Added"
		next, err = library.Add(m.lib, draft)
	} else {
		next, err = library.Update(m.lib, s.id, draft)
	}
	if err != nil {
		// problem() has already refused everything the Library refuses, so this
		// is unreachable — but it is a refusal, and staying on the form with the
		// reason on it is what a refusal looks like here.
		s.tried = true
		return nil
	}

	saved := m.updateLibrary(next)
	m.screen = s.back
	return m.finish(verb, saved)
}

func (s *editScreen) keys(*Model) []footerKey {
	return footerKeys(keymap.edit.Save, keymap.form.Next, keymap.edit.Cancel)
}

// label heads a field's section, in accent when the field has the keyboard.
func (s *editScreen) label(i int) lipgloss.Style {
	if i == s.form.Focused() {
		return focusStyle()
	}
	return sectionStyle()
}

func (s *editScreen) view(m *Model) []string {
	width := m.innerWidth()
	inner := width - 2

	// A name collision is worth saying the moment it is typed — it is the one
	// problem the user cannot see coming. The rest wait for a refused save.
	warning := s.collision(m)
	if warning == "" && s.tried {
		warning = s.problem(m)
	}
	// An armed esc outranks both. It is the only warning here about the key the
	// user is holding rather than about the form, and it is gone next keystroke.
	if s.discardArmed {
		warning = "Unsaved changes — esc again to discard"
	}
	var bottom []string
	if warning != "" {
		bottom = []string{dangerStyle.Render("⚠ " + warning)}
	}

	on := m.caretOn()
	nameRows, _ := s.form.Field(fieldName).Rows(inner, on)
	descRows, _ := s.form.Field(fieldDescription).Rows(inner, on)
	cmdRows, caretRow := s.form.Field(fieldCommand).Rows(inner, on)

	nameSec := section(width, editLabels[fieldName], s.label(fieldName), "", nameRows)
	descSec := section(width, editLabels[fieldDescription], s.label(fieldDescription), "Optional", descRows)
	var phSec []string
	if ps := placeholders.Parse(s.value(fieldCommand)); len(ps) > 0 {
		phSec = section(width, "Placeholders", sectionStyle(), "", placeholderRows(ps, inner))
	}

	// Everything but the command is as tall as its own content; the command
	// takes what is left and scrolls around the caret, so typing past the
	// bottom of the screen moves the view rather than the caret out of sight.
	//
	// On a screen too short for all of it, the placeholder list is what gives.
	// It is derived from the command — which already picks the same names out
	// in gold — so losing it costs less than losing the field being typed in,
	// and a section rule with its rows cut off under it reads as a fault.
	spent := len(nameSec) + 1 + len(descSec) + 1 + 1 + len(bottom) // the blanks between sections, and command's own rule
	avail := m.bodyHeight() - spent
	if len(phSec) > 0 {
		height := min(len(phSec), max(0, avail-min(len(cmdRows), 3)-1))
		if height < 2 {
			// a rule with nothing under it is not a section
			phSec = nil
		} else {
			if height < len(phSec) {
				phSec = append(phSec[:height-1:height-1], contentIndent+dimStyle.Render("…"))
			}
			avail -= height + 1
		}
	}
	cmdRows = window(cmdRows, caretRow, max(1, avail))

	top := append([]string{}, nameSec...)
	top = append(top, "")
	top = append(top, descSec...)
	top = append(top, "")
	top = append(top, section(width, editLabels[fieldCommand], s.label(fieldCommand), "", cmdRows)...)
	if len(phSec) > 0 {
		top = append(top, "")
		top = append(top, phSec...)
	}
	return pin(top, bottom, m.bodyHeight())
}
