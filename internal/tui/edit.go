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
	"github.com/google/uuid"
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
	fieldName:        "name",
	fieldDescription: "description",
	fieldCommand:     "command",
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

func newEditScreen(m *Model, entry *library.Entry) *editScreen {
	s := &editScreen{}
	var values [fieldCount]string
	if entry != nil {
		s.id = entry.ID
		values[fieldName] = entry.Name
		values[fieldCommand] = entry.Command
		if entry.Description != nil {
			values[fieldDescription] = *entry.Description
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

// problem is the first reason this Command cannot be saved, or "".
func (s *editScreen) problem(m *Model) string {
	name := strings.TrimSpace(s.value(fieldName))
	if name == "" {
		return "name is required"
	}
	if s.taken(m, name) {
		return fmt.Sprintf("'%s' already exists", name)
	}
	if strings.TrimSpace(s.value(fieldCommand)) == "" {
		return "command is required"
	}
	return ""
}

func (s *editScreen) taken(m *Model, name string) bool {
	for _, entry := range m.lib.Commands {
		if entry.Name == name && entry.ID != s.id {
			return true
		}
	}
	return false
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

func (s *editScreen) save(m *Model) tea.Cmd {
	name := strings.TrimSpace(s.value(fieldName))
	command := s.value(fieldCommand)
	description := strings.TrimSpace(s.value(fieldDescription))

	next := m.lib
	if s.id == "" {
		entry := library.Entry{ID: uuid.NewString(), Name: name, Command: command}
		if description != "" {
			entry.Description = &description
		}
		next.Commands = append(append([]library.Entry{}, m.lib.Commands...), entry)
		m.updateLibrary(next)
		m.screen = newListScreen(m)
		return m.flashDefault("added")
	}

	// rename/edit in place: keep the id and the file slot (only the fields
	// change), so State and array order both survive.
	commands := make([]library.Entry, len(m.lib.Commands))
	copy(commands, m.lib.Commands)
	for i := range commands {
		if commands[i].ID != s.id {
			continue
		}
		commands[i].Name = name
		commands[i].Command = command
		if description != "" {
			commands[i].Description = &description
		} else {
			commands[i].Description = nil
		}
	}
	next.Commands = commands
	m.updateLibrary(next)
	m.screen = newListScreen(m)
	return m.flashDefault("saved")
}

func (s *editScreen) keys(*Model) []footerKey {
	return footerKeys(keymap.edit.Save, keymap.form.Next, keymap.edit.Cancel)
}

// rows renders one field's value, wrapped to the width and carrying the caret
// when focused, and reports which row the caret landed on. Only the command
// field highlights placeholders — it is the only field where `{{name}}` means
// anything.
func (s *editScreen) rows(i, inner int) ([]string, int) {
	value := s.value(i)
	focused := i == s.focus

	if i != fieldCommand {
		return valueRowsAt([]run{{text: value, style: textStyle}}, value, s.inputs[i].Position(), inner, focused)
	}
	if value == "" {
		// The hint lives on the caret row rather than in a section of its own,
		// so it costs no row and vanishes on the first keystroke.
		hint := dimStyle.Render("type a command — {{name}} or {{name=default}} become args")
		if !focused {
			return []string{ansi.Truncate(hint, inner, "")}, 0
		}
		return []string{ansi.Truncate(caretStyle.Render(" ")+hint, inner, "")}, 0
	}
	var runs []run
	for _, seg := range placeholders.TemplateSegments(value) {
		style := textStyle
		if seg.Flag {
			style = highlightStyle.Bold(true)
		}
		runs = append(runs, run{text: seg.Text, style: style})
	}
	return valueRowsAt(runs, value, s.inputs[fieldCommand].Position(), inner, focused)
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
	warning := ""
	if name := strings.TrimSpace(s.value(fieldName)); name != "" && s.taken(m, name) {
		warning = fmt.Sprintf("'%s' already exists", name)
	} else if s.tried {
		warning = s.problem(m)
	}
	var bottom []string
	if warning != "" {
		bottom = []string{dangerStyle.Render("⚠ " + warning)}
	}

	nameRows, _ := s.rows(fieldName, inner)
	descRows, _ := s.rows(fieldDescription, inner)
	cmdRows, caretRow := s.rows(fieldCommand, inner)

	nameSec := section(width, editLabels[fieldName], s.label(fieldName), "", nameRows)
	descSec := section(width, editLabels[fieldDescription], s.label(fieldDescription), "optional", descRows)
	var phSec []string
	if ps := placeholders.Parse(s.value(fieldCommand)); len(ps) > 0 {
		phSec = section(width, "placeholders", sectionStyle(), "", placeholderRows(ps, inner, false))
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
