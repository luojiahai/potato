// The add / edit screen: one ruled section per field, each sized to what it
// holds, stacked from the top.
//
// The sections no longer share out a fixed height between them. Every one is as
// tall as its own content, so the command field grows downward as you type and
// pushes `placeholders` along with it, and the slack collects above the footer
// where nothing frames it.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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

type editScreen struct {
	id     string // "" = a new Command
	inputs []textinput.Model
	focus  int
	// tried records a save that was refused. Until then the form stays quiet
	// about the fields it is still waiting for — a brand-new Command is empty
	// by definition, and greeting it with "name is required" is noise.
	tried bool
}

func newEditScreen(m *Model, command *library.Command) *editScreen {
	s := &editScreen{}
	var values [fieldCount]string
	if command != nil {
		s.id = command.ID
		values[fieldName] = command.Name
		values[fieldCommand] = command.Template
		if command.Description != nil {
			values[fieldDescription] = *command.Description
		}
	}
	for _, value := range values {
		input := newField()
		input.SetValue(value)
		s.inputs = append(s.inputs, input)
	}
	s.inputs[fieldName].Focus()
	return s
}

func (s *editScreen) value(i int) string { return s.inputs[i].Value() }

func (s *editScreen) setFocus(i int) {
	s.inputs[s.focus].Blur()
	s.focus = ((i % len(s.inputs)) + len(s.inputs)) % len(s.inputs)
	s.inputs[s.focus].Focus()
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

func (s *editScreen) update(m *Model, msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s.forward(msg)
	}
	switch {
	case key.Matches(keyMsg, keymap.edit.Cancel):
		m.screen = newListScreen(m)
		return nil
	case key.Matches(keyMsg, keymap.edit.Save):
		if s.problem(m) != "" {
			// No flash: the inline warning says the same thing and stays put
			// until the problem is fixed, where a toast would expire while the
			// field it is about is still empty.
			s.tried = true
			return nil
		}
		return s.save(m)
	case key.Matches(keyMsg, keymap.form.Next):
		s.setFocus(s.focus + 1)
		return nil
	case key.Matches(keyMsg, keymap.form.Prev):
		s.setFocus(s.focus - 1)
		return nil
	}
	return s.forward(msg)
}

func (s *editScreen) forward(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.inputs[s.focus], cmd = s.inputs[s.focus].Update(msg)
	return cmd
}

// save hands the form's fields to the Library and goes back to the list. The
// trimming, the id, the slot and the empty-description rule are all the
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
	m.screen = newListScreen(m)
	return m.finish(verb, saved)
}

func (s *editScreen) keys(*Model) []footerKey {
	return footerKeys(keymap.edit.Save, keymap.form.Next, keymap.edit.Cancel)
}

// rows renders one field's value, wrapped to the width and carrying the caret
// when focused, and reports which row the caret landed on. Only the command
// field highlights placeholders — it is the only field where `{{name}}` means
// anything.
func (s *editScreen) rows(i, inner int, on bool) ([]string, int) {
	value := s.value(i)
	focused := i == s.focus

	if i != fieldCommand {
		return valueRowsAt([]run{{text: value, style: textStyle}}, value, s.inputs[i].Position(), inner, focused, on)
	}
	if value == "" {
		// The hint lives on the caret row rather than in a section of its own,
		// so it costs no row and vanishes on the first keystroke.
		const hint = "Type a command — {{name}} or {{name=default}} become args"
		if !focused {
			return []string{ansi.Truncate(dimStyle.Render(hint), inner, "")}, 0
		}
		// The caret sits on the hint's first character rather than in a cell of
		// its own in front of it. Given a cell, the hint stepped a column to the
		// right the moment the field took the keyboard — the text moved to make
		// room for a caret that has nothing to sit on yet. A placeholder is
		// exactly what a caret should be allowed to sit on: bubbles puts it on
		// the first character in the search field, and the row is the same width
		// either way, so nothing moves when focus arrives or leaves.
		rs := []rune(hint)
		head := dimStyle
		if on {
			head = caretStyle
		}
		return []string{ansi.Truncate(head.Render(string(rs[0]))+dimStyle.Render(string(rs[1:])), inner, "")}, 0
	}
	var runs []run
	for _, seg := range placeholders.TemplateSegments(value) {
		style := textStyle
		if seg.Flag {
			style = highlightStyle.Bold(true)
		}
		runs = append(runs, run{text: seg.Text, style: style})
	}
	return valueRowsAt(runs, value, s.inputs[fieldCommand].Position(), inner, focused, on)
}

// label heads a field's section, in accent when the field has the keyboard.
func (s *editScreen) label(i int) lipgloss.Style {
	if i == s.focus {
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
	var bottom []string
	if warning != "" {
		bottom = []string{dangerStyle.Render("⚠ " + warning)}
	}

	on := m.caretOn()
	nameRows, _ := s.rows(fieldName, inner, on)
	descRows, _ := s.rows(fieldDescription, inner, on)
	cmdRows, caretRow := s.rows(fieldCommand, inner, on)

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
