// The add / edit screen: three fields plus a live template panel.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/placeholders"
)

var editOrder = []string{"name", "command", "description"}

type editScreen struct {
	id     string // "" = a new Command
	title  string
	inputs []textinput.Model
	focus  int
}

func newEditScreen(m *Model, entry *library.Entry) *editScreen {
	s := &editScreen{title: "new command"}
	values := []string{"", "", ""}
	if entry != nil {
		s.id = entry.ID
		s.title = fmt.Sprintf("edit '%s'", entry.Name)
		values[0] = entry.Name
		values[1] = entry.Command
		if entry.Description != nil {
			values[2] = *entry.Description
		}
	}
	for _, value := range values {
		input := newField()
		input.SetValue(value)
		s.inputs = append(s.inputs, input)
	}
	s.inputs[0].Focus()
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
	name := strings.TrimSpace(s.value(0))
	if name == "" {
		return "name is required"
	}
	if s.taken(m, name) {
		return fmt.Sprintf("'%s' already exists", name)
	}
	if strings.TrimSpace(s.value(1)) == "" {
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
		if problem := s.problem(m); problem != "" {
			return m.flashDefault(problem)
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
	name := strings.TrimSpace(s.value(0))
	command := s.value(1)
	description := strings.TrimSpace(s.value(2))

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

func (s *editScreen) view(m *Model) []string {
	width := m.innerWidth()
	inner := width - 4

	var content []string
	for i, label := range editOrder {
		hint := ""
		if label == "description" {
			hint = "(optional)"
		}
		value := s.inputs[i].Value()
		if i == s.focus {
			value = s.inputs[i].View()
		}
		content = append(content, field(label, value, i == s.focus, hint))
	}
	name := strings.TrimSpace(s.value(0))
	if name != "" && s.taken(m, name) {
		content = append(content, "", redStyle.Render(fmt.Sprintf("⚠ '%s' already exists", name)))
	}
	lines := panel(s.title, boldStyle, frameStyle, width, content, len(content)+2)

	command := s.value(1)
	var template []string
	if strings.TrimSpace(command) == "" {
		template = []string{dimStyle.Render("type a command — {{name}} or {{name=default}} become args")}
	} else {
		runs := []run{{text: "$ ", style: dimStyle}}
		for _, seg := range placeholders.TemplateSegments(command) {
			style := cyanStyle
			if seg.Flag {
				style = yellowStyle.Bold(true)
			}
			runs = append(runs, run{text: seg.Text, style: style})
		}
		template = wrapStyled(runs, inner)
		if ps := placeholders.Parse(command); len(ps) > 0 {
			template = append(template, "", dimStyle.Render("args:"))
			template = append(template, placeholderRows(ps)...)
		}
	}
	lines = append(lines, panel("template", boldStyle, frameStyle, width, template, len(template)+2)...)

	for len(lines) < m.bodyHeight() {
		lines = append(lines, "")
	}
	return lines
}
