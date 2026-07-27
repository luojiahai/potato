// The arg screen: one form for every Placeholder in a Command, with a live
// preview of what will run.

package tui

import (
	"fmt"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/placeholders"
)

type argsScreen struct {
	id       string
	name     string
	command  string
	ps       []placeholders.Placeholder
	lastArgs map[string]string
	inputs   []textinput.Model
	focus    int
}

func newArgsScreen(m *Model, entry *library.Entry) *argsScreen {
	ps := placeholders.Parse(entry.Command)
	s := &argsScreen{
		id:       entry.ID,
		name:     entry.Name,
		command:  entry.Command,
		ps:       ps,
		lastArgs: m.st[entry.ID].Args,
	}
	for _, p := range ps {
		input := newField()
		// pre-fill precedence: last value > default > empty (spec §2)
		if value, ok := s.lastArgs[p.Name]; ok {
			input.SetValue(value)
		} else if p.HasDefault {
			input.SetValue(p.Default)
		}
		s.inputs = append(s.inputs, input)
	}
	if len(s.inputs) > 0 {
		s.inputs[0].Focus()
	}
	return s
}

func (s *argsScreen) values() map[string]string {
	out := map[string]string{}
	for i, p := range s.ps {
		out[p.Name] = s.inputs[i].Value()
	}
	return out
}

func (s *argsScreen) setFocus(i int) {
	if len(s.inputs) == 0 {
		return
	}
	s.inputs[s.focus].Blur()
	s.focus = ((i % len(s.inputs)) + len(s.inputs)) % len(s.inputs)
	s.inputs[s.focus].Focus()
}

func (s *argsScreen) update(m *Model, msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s.forward(msg)
	}
	switch keyMsg.String() {
	case "esc":
		m.screen = newListScreen(m)
		return nil
	case "enter":
		return m.run(s.id, s.values())
	case "ctrl+y":
		return m.copy(s.id, s.values())
	case "tab", "down":
		s.setFocus(s.focus + 1)
		return nil
	case "shift+tab", "up":
		s.setFocus(s.focus - 1)
		return nil
	}
	return s.forward(msg)
}

func (s *argsScreen) forward(msg tea.Msg) tea.Cmd {
	if len(s.inputs) == 0 {
		return nil
	}
	var cmd tea.Cmd
	s.inputs[s.focus], cmd = s.inputs[s.focus].Update(msg)
	return cmd
}

func (s *argsScreen) keys(*Model) []footerKey {
	return []footerKey{{"↵", "run"}, {"^Y", "copy"}, {"tab", "next field"}, {"esc", "back"}}
}

func (s *argsScreen) view(m *Model) []string {
	width := m.innerWidth()
	inner := width - 4

	plural := "s"
	if len(s.ps) == 1 {
		plural = ""
	}
	content := []string{dimStyle.Render(fmt.Sprintf("needs %d arg%s", len(s.ps), plural)), ""}
	for i, p := range s.ps {
		hint := ""
		if _, ok := s.lastArgs[p.Name]; ok {
			hint = "(last used)"
		} else if p.HasDefault {
			hint = fmt.Sprintf("(default: %s)", p.Default)
		}
		value := s.inputs[i].Value()
		if i == s.focus {
			value = s.inputs[i].View()
		}
		content = append(content, field(p.Name, value, i == s.focus, hint))
	}
	lines := panel(s.name, boldStyle, frameStyle, width, content, len(content)+2)

	preview := []run{{text: "$ ", style: dimStyle}}
	for _, seg := range placeholders.RenderSegments(s.command, s.values()) {
		style := lipglossPlain
		if seg.Flag {
			style = cyanStyle.Bold(true)
		}
		preview = append(preview, run{text: seg.Text, style: style})
	}
	previewLines := wrapStyled(preview, inner)
	lines = append(lines, panel("will run", boldStyle, frameStyle, width, previewLines, len(previewLines)+2)...)

	// <Box flexGrow={1}/> — everything above sits at the top of the screen
	for len(lines) < m.bodyHeight() {
		lines = append(lines, "")
	}
	return lines
}
