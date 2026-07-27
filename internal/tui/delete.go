// The delete confirm.

package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/luojiahai/potato/internal/library"
)

type deleteScreen struct{ id string }

func newDeleteScreen(id string) *deleteScreen { return &deleteScreen{id: id} }

func (s *deleteScreen) update(m *Model, msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch keyMsg.String() {
	case "y", "Y":
		entry := library.FindByID(m.lib, s.id)
		if entry == nil {
			m.screen = newListScreen(m)
			return nil
		}
		name := entry.Name
		next := m.lib
		commands := make([]library.Entry, 0, len(m.lib.Commands))
		for _, c := range m.lib.Commands {
			if c.ID != s.id {
				commands = append(commands, c)
			}
		}
		next.Commands = commands
		m.updateLibrary(next)
		m.screen = newListScreen(m)
		return m.flashDefault(fmt.Sprintf("deleted '%s'", name))
	case "n", "N", "esc", "enter":
		m.screen = newListScreen(m)
		return nil
	}
	return nil
}

func (s *deleteScreen) keys(*Model) []footerKey {
	return []footerKey{{"y", "delete"}, {"n / esc", "keep"}}
}

func (s *deleteScreen) view(m *Model) []string {
	width := m.innerWidth()
	entry := library.FindByID(m.lib, s.id)
	if entry == nil {
		return make([]string, m.bodyHeight())
	}
	body := dimStyle.Render(ansi.Truncate("$ "+entry.Command, width-4, ""))
	lines := panel(fmt.Sprintf("delete '%s'?", entry.Name),
		boldStyle.Foreground(redColor), redStyle, width, []string{body}, 3)
	for len(lines) < m.bodyHeight() {
		lines = append(lines, "")
	}
	return lines
}
