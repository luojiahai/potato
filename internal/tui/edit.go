// The add / edit screen: one titled panel per field, with the command field
// absorbing whatever height the others leave.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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
	title  string
	inputs []textinput.Model
	focus  int
	// tried records a save that was refused. Until then the form stays quiet
	// about the fields it is still waiting for — a brand-new Command is empty
	// by definition, and greeting it with "name is required" is noise.
	tried bool
}

func newEditScreen(m *Model, entry *library.Entry) *editScreen {
	s := &editScreen{title: "new command"}
	var values [fieldCount]string
	if entry != nil {
		s.id = entry.ID
		s.title = fmt.Sprintf("edit '%s'", entry.Name)
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
	switch keyMsg.String() {
	case "esc":
		m.screen = newListScreen(m)
		return nil
	case "enter":
		if s.problem(m) != "" {
			// No flash: the inline warning says the same thing and stays put
			// until the problem is fixed, where a toast would expire while the
			// field it is about is still empty.
			s.tried = true
			return nil
		}
		return s.save(m)
	case "tab", "down":
		s.setFocus(s.focus + 1)
		return nil
	case "shift+tab", "up":
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
	return []footerKey{{"↵", "save"}, {"tab", "next field"}, {"esc", "cancel"}}
}

// rows renders one field's value, wrapped to the panel and carrying the caret
// when focused. Only the command field highlights placeholders — it is the
// only field where `{{name}}` means anything.
func (s *editScreen) rows(i, inner int) []string {
	value := s.value(i)
	focused := i == s.focus

	if i != fieldCommand {
		return valueRows([]run{{text: value, style: textStyle}}, value, s.inputs[i].Position(), inner, focused)
	}
	if value == "" {
		// The hint lives on the caret row rather than in a panel of its own,
		// so it costs no row and vanishes on the first keystroke.
		hint := dimStyle.Render("type a command — {{name}} or {{name=default}} become args")
		if !focused {
			return []string{ansi.Truncate(hint, inner, "")}
		}
		return []string{ansi.Truncate(caretStyle.Render(" ")+hint, inner, "")}
	}
	var runs []run
	for _, seg := range placeholders.TemplateSegments(value) {
		style := textStyle
		if seg.Flag {
			style = highlightStyle.Bold(true)
		}
		runs = append(runs, run{text: seg.Text, style: style})
	}
	return valueRows(runs, value, s.inputs[fieldCommand].Position(), inner, focused)
}

func (s *editScreen) view(m *Model) []string {
	width := m.innerWidth()
	inner := width - 4

	// A name collision is worth saying the moment it is typed — it is the one
	// problem the user cannot see coming. The rest wait for a refused save.
	warning := ""
	if name := strings.TrimSpace(s.value(fieldName)); name != "" && s.taken(m, name) {
		warning = fmt.Sprintf("'%s' already exists", name)
	} else if s.tried {
		warning = s.problem(m)
	}

	ps := placeholders.Parse(s.value(fieldCommand))
	nameRows := s.rows(fieldName, inner)
	descRows := s.rows(fieldDescription, inner)
	cmdRows := s.rows(fieldCommand, inner)

	// One top-to-bottom pass. name and description take what they need, the
	// placeholder list takes what it needs from the remainder, and command —
	// the only field that grows — absorbs everything still unspent.
	budget := m.bodyHeight() - 6 // three field panels' borders
	if len(ps) > 0 {
		budget -= 2
	}
	if warning != "" {
		budget-- // the warning row sits under the panels
	}
	budget -= len(nameRows) + len(descRows)

	phRows := 0
	if len(ps) > 0 {
		phRows = max(1, min(len(ps), budget-1))
		budget -= phRows
	}

	lines := fieldPanel(editLabels[fieldName], "", nameRows, s.focus == fieldName, width, len(nameRows)+2)
	lines = append(lines, fieldPanel(editLabels[fieldDescription], "(optional)", descRows,
		s.focus == fieldDescription, width, len(descRows)+2)...)
	lines = append(lines, fieldPanel(editLabels[fieldCommand], "", cmdRows,
		s.focus == fieldCommand, width, max(len(cmdRows), budget)+2)...)
	if len(ps) > 0 {
		lines = append(lines, panel("placeholders", titleStyle(), frameStyle, width,
			placeholderRows(ps, false), phRows+2)...)
	}
	if warning != "" {
		lines = append(lines, " "+dangerStyle.Render("⚠ "+warning))
	}

	for len(lines) < m.bodyHeight() {
		lines = append(lines, "")
	}
	return lines
}
