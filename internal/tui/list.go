// The list screen: a split pane with fuzzy search on the left and the
// selected Command's detail on the right.

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
	"github.com/luojiahai/potato/internal/search"
)

const listPanelWidth = 30

type listScreen struct {
	input textinput.Model
	sel   int
}

func newListScreen(m *Model) *listScreen {
	input := newField()
	// The search field is the one place ^A and ^E must NOT mean line-start
	// and line-end: the list screen spends them on add and edit.
	input.KeyMap.LineStart = key.NewBinding()
	input.KeyMap.LineEnd = key.NewBinding()
	input.Focus()
	return &listScreen{input: input}
}

// newField builds a text input shaped like potato's fields: no prompt, no
// placeholder, and a cursor rendered into the view rather than delegated to
// the terminal.
func newField() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.SetVirtualCursor(true)
	return input
}

func (s *listScreen) results(m *Model) []library.Entry {
	return search.Commands(m.lib.Commands, m.st, s.input.Value())
}

func (s *listScreen) selected(m *Model) *library.Entry {
	results := s.results(m)
	if len(results) == 0 {
		return nil
	}
	idx := min(s.sel, len(results)-1)
	return &results[idx]
}

func (s *listScreen) update(m *Model, msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return cmd
	}

	results := s.results(m)
	selected := s.selected(m)

	switch keyMsg.String() {
	case "esc":
		return tea.Quit
	case "ctrl+a":
		m.screen = newEditScreen(m, nil)
		return nil
	case "ctrl+e":
		if selected != nil {
			m.screen = newEditScreen(m, selected)
		}
		return nil
	case "ctrl+d":
		if selected != nil {
			m.screen = newDeleteScreen(selected.ID)
		}
		return nil
	case "ctrl+y":
		if selected == nil {
			return nil
		}
		if len(placeholders.Parse(selected.Command)) > 0 {
			m.screen = newArgsScreen(m, selected)
			return m.flashDefault("needs args — fill in, then ^Y")
		}
		return m.copy(selected.ID, map[string]string{})
	case "enter":
		if selected == nil {
			return nil
		}
		if len(placeholders.Parse(selected.Command)) > 0 {
			m.screen = newArgsScreen(m, selected)
			return nil
		}
		return m.run(selected.ID, map[string]string{})
	case "up":
		s.sel = max(0, s.sel-1)
		return nil
	case "down":
		s.sel = min(len(results)-1, s.sel+1)
		return nil
	}

	before := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	// Editing the query resets the selection; pure cursor motion does not.
	if s.input.Value() != before {
		s.sel = 0
	}
	return cmd
}

func (s *listScreen) keys(*Model) []footerKey {
	return []footerKey{
		{"↵", "run"}, {"^Y", "copy"}, {"^A", "add"},
		{"^E", "edit"}, {"^D", "delete"}, {"esc", "quit"},
	}
}

func (s *listScreen) view(m *Model) []string {
	width := m.innerWidth()
	query := s.input.Value()
	results := s.results(m)
	sel := min(s.sel, max(0, len(results)-1))

	// search panel
	right := fmt.Sprintf("%d/%d", len(results), len(m.lib.Commands))
	if query == "" {
		right = "(recently used first)  " + right
	}
	title := ""
	if !m.showBanner() {
		title = "potato"
	}
	left := cyanStyle.Bold(true).Render("/ ") + s.input.View()
	gap := max(0, (width-4)-ansi.StringWidth(left)-ansi.StringWidth(right))
	lines := panel(title, boldStyle.Foreground(lipgloss.Color("3")), frameStyle, width,
		[]string{left + strings.Repeat(" ", gap) + dimStyle.Render(right)}, 3)

	// the split pane fills what is left between the search panel and the footer
	middleHeight := max(0, m.bodyHeight()-3)
	listWidth, detailWidth := s.split(m, results, sel, width)

	lines = append(lines, s.pane(m, results, sel, listWidth, detailWidth, middleHeight)...)
	return lines
}

// split divides the pane between the fixed-width command list and the detail
// panel. The list asks for 30 columns and the detail for whatever its widest
// unwrapped line needs; when the two together exceed the space, both give up
// columns in proportion to what they asked for, so a narrow terminal squeezes
// the list rather than only the detail.
func (s *listScreen) split(m *Model, results []library.Entry, sel, width int) (int, int) {
	detailBasis := ansi.StringWidth("nothing selected") + 4
	if len(results) > 0 {
		widest := 0
		for _, line := range s.detailContent(m, results[sel], 0) {
			if n := ansi.StringWidth(line); n > widest {
				widest = n
			}
		}
		detailBasis = widest + 4
	}

	total := listPanelWidth + detailBasis
	if total <= width {
		listWidth := min(listPanelWidth, width)
		return listWidth, width - listWidth
	}
	overflow := total - width
	shrunk := float64(listPanelWidth) - float64(overflow)*float64(listPanelWidth)/float64(total)
	listWidth := int(shrunk + 0.5)
	listWidth = max(1, min(listWidth, width))
	return listWidth, width - listWidth
}

func (s *listScreen) pane(m *Model, results []library.Entry, sel, listWidth, detailWidth, height int) []string {
	height = max(0, height)
	// The window is sized from the terminal, the panel from the layout; where
	// they disagree the panel clips, exactly as the flexbox build does.
	chrome := 7
	if m.showBanner() {
		chrome += bannerRowCount
	}
	visible := max(2, m.height-chrome)
	start := 0
	if len(results) > visible {
		start = max(0, min(sel-visible+1, len(results)-visible))
	}
	end := min(len(results), start+visible)

	var listContent []string
	if len(results) == 0 {
		listContent = append(listContent, dimStyle.Render("no matches"))
	}
	query := s.input.Value()
	for i := start; i < end; i++ {
		entry := results[i]
		pointer := "  "
		if i == sel {
			pointer = "❯ "
		}
		listContent = append(listContent, accentStyle.Render(pointer)+
			highlightName(query, entry.Name, i == sel)+badge(entry.Command))
	}
	listPanel := panel("commands", boldStyle, frameStyle, listWidth, listContent, height)

	var detailPanel []string
	if len(results) > 0 {
		entry := results[sel]
		detailPanel = panel(entry.Name, boldStyle, frameStyle, detailWidth,
			s.detailContent(m, entry, detailWidth-4), height)
	} else {
		detailPanel = panel("", boldStyle, frameStyle, detailWidth,
			[]string{dimStyle.Render("nothing selected")}, height)
	}

	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		out = append(out, listPanel[i]+detailPanel[i])
	}
	return out
}

// detailContent builds the detail panel's rows. inner is the wrap width; 0
// leaves every row unwrapped, which is how split measures what the panel
// would like to be.
func (s *listScreen) detailContent(m *Model, entry library.Entry, inner int) []string {
	var content []string
	if entry.Description != nil && *entry.Description != "" {
		for _, line := range wrapOrNot(*entry.Description, inner) {
			content = append(content, dimStyle.Render(line))
		}
		content = append(content, "")
	}
	content = append(content, commandBlock(entry.Command, inner)...)
	if ps := placeholders.Parse(entry.Command); len(ps) > 0 {
		content = append(content, "", dimStyle.Render("args:"))
		content = append(content, placeholderRows(ps)...)
	}
	if used := m.st[entry.ID].LastUsedAt; used != "" {
		if ago := timeAgo(used, m.deps.Now()); ago != "" {
			content = append(content, "", dimStyle.Render("used "+ago))
		}
	}
	return content
}

// ---------- shared renderers ----------

// commandBlock renders a Command with its `$ ` gutter, wrapped to width.
func commandBlock(command string, inner int) []string {
	var out []string
	for i, line := range wrapOrNot("$ "+command, inner) {
		if i == 0 {
			out = append(out, dimStyle.Render("$ ")+cyanStyle.Render(strings.TrimPrefix(line, "$ ")))
			continue
		}
		out = append(out, cyanStyle.Render(line))
	}
	return out
}

func placeholderRows(ps []placeholders.Placeholder) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		row := "  " + yellowStyle.Render(p.Name)
		if p.HasDefault {
			row += dimStyle.Render(" = " + p.Default)
		}
		out = append(out, row)
	}
	return out
}

func badge(command string) string {
	n := len(placeholders.Parse(command))
	if n == 0 {
		return ""
	}
	return dimStyle.Foreground(lipgloss.Color("6")).Render(fmt.Sprintf(" ⌁%d", n))
}

// highlightName paints the subsequence match positions yellow inside the
// (optionally inverted) selected row.
func highlightName(query, name string, selected bool) string {
	base := boldStyle
	if selected {
		base = base.Reverse(true)
	}
	matches, ok := search.NameMatchIndices(query, name)
	if !ok {
		return base.Render(name)
	}
	var b strings.Builder
	for i, r := range []rune(name) {
		if matches[i] {
			b.WriteString(base.Foreground(lipgloss.Color("3")).Render(string(r)))
			continue
		}
		b.WriteString(base.Render(string(r)))
	}
	return b.String()
}
