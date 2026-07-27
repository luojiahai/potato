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
	// confirming holds the id of the Command awaiting a delete confirmation,
	// or "". The confirm is inline rather than a screen of its own so the
	// detail panel keeps showing what is about to be deleted while you answer.
	confirming string
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
	// The bubbles defaults reach for ANSI palette indices, which would leave
	// the caret and the text the user is typing coloured by their terminal
	// theme rather than by potato.
	styles := input.Styles()
	styles.Cursor.Color = lipgloss.Color(accentColor)
	for _, state := range []*textinput.StyleState{&styles.Focused, &styles.Blurred} {
		state.Text = textStyle
		state.Prompt = dimStyle
		state.Placeholder = dimStyle
		state.Suggestion = dimStyle
	}
	input.SetStyles(styles)
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

	// While a confirm is open it owns the keyboard: the answer is one
	// keystroke, and letting anything else through would let the query change
	// under a confirm that names a Command by id.
	if s.confirming != "" {
		switch keyMsg.String() {
		case "y", "Y":
			return s.delete(m)
		default:
			s.confirming = ""
			return nil
		}
	}

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
			s.confirming = selected.ID
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

// delete removes the Command the confirm names, by id — the query and the
// selection may both have moved since the confirm opened.
func (s *listScreen) delete(m *Model) tea.Cmd {
	entry := library.FindByID(m.lib, s.confirming)
	s.confirming = ""
	if entry == nil {
		return nil
	}
	next := m.lib
	commands := make([]library.Entry, 0, len(m.lib.Commands))
	for _, c := range m.lib.Commands {
		if c.ID != entry.ID {
			commands = append(commands, c)
		}
	}
	next.Commands = commands
	m.updateLibrary(next)
	return m.flashDefault(fmt.Sprintf("deleted '%s'", entry.Name))
}

func (s *listScreen) keys(*Model) []footerKey {
	if s.confirming != "" {
		return []footerKey{{"y", "delete"}, {"n / esc", "keep"}}
	}
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
	if m.bannerHeight() == 0 {
		title = "potato"
	}
	left := accentStyle.Bold(true).Render("/ ") + s.input.View()
	gap := max(0, (width-4)-ansi.StringWidth(left)-ansi.StringWidth(right))
	lines := panel(title, boldStyle.Foreground(lipgloss.Color(accentColor)), frameStyle, width,
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
	// The empty-state panels wrap to whatever they are handed, so they ask for
	// a comfortable reading measure rather than a measured one.
	detailBasis := 44
	if len(results) > 0 {
		widest := 0
		for _, line := range s.detailContent(m, results[sel], 0) {
			if n := ansi.StringWidth(line); n > widest {
				widest = n
			}
		}
		detailBasis = widest + 3 // the joined panel spends one column less
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
	query := s.input.Value()

	listPanel := panelWith(boxSeam, "commands", titleStyle(), frameStyle, listWidth,
		s.listRows(m, results, sel, max(1, height-2), listWidth-4, query), height)

	var detailPanel []string
	switch {
	case len(results) > 0:
		entry := results[sel]
		detailPanel = panelWith(boxJoined, entry.Name, titleStyle(), frameStyle, detailWidth,
			s.detailContent(m, entry, detailWidth-3), height)
	case len(m.lib.Commands) == 0:
		detailPanel = panelWith(boxJoined, "getting started", titleStyle(), frameStyle, detailWidth,
			gettingStarted(detailWidth-3), height)
	default:
		detailPanel = panelWith(boxJoined, "no match", titleStyle(), frameStyle, detailWidth,
			noMatch(query, detailWidth-3), height)
	}

	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		out = append(out, listPanel[i]+detailPanel[i])
	}
	return out
}

// listRows renders the visible slice of the results. When they overflow the
// panel the first and last rows are given over to the counts still hidden —
// reserved at both ends whether or not both ends have anything to report, so
// the rows between them hold still as the selection moves.
func (s *listScreen) listRows(m *Model, results []library.Entry, sel, rows, inner int, query string) []string {
	if len(results) == 0 {
		if len(m.lib.Commands) == 0 {
			return []string{dimStyle.Render("no commands yet")}
		}
		return []string{dimStyle.Render("no matches")}
	}
	if len(results) <= rows {
		out := make([]string, 0, len(results))
		for i, entry := range results {
			out = append(out, s.rowFor(entry, i == sel, inner, query))
		}
		return out
	}

	window := max(1, rows-2)
	start := max(0, min(sel-window+1, len(results)-window))
	end := min(len(results), start+window)

	out := make([]string, 0, rows)
	out = append(out, overflowRow("↑", start))
	for i := start; i < end; i++ {
		out = append(out, s.rowFor(results[i], i == sel, inner, query))
	}
	return append(out, overflowRow("↓", len(results)-end))
}

func overflowRow(arrow string, n int) string {
	if n == 0 {
		return ""
	}
	return dimStyle.Render(fmt.Sprintf("  %s %d more", arrow, n))
}

// rowFor renders a Command's row, or the delete confirm in its place when
// that Command is the one awaiting an answer. The confirm takes over the row
// rather than the screen, so the detail panel beside it goes on showing the
// command you are about to lose.
func (s *listScreen) rowFor(entry library.Entry, selected bool, inner int, query string) string {
	if entry.ID != s.confirming {
		return listRow(entry, selected, inner, query)
	}
	label := "⚠ delete? y/n"
	pad := max(0, inner-ansi.StringWidth(label))
	fill := dangerStyle.Bold(true).Background(lipgloss.Color(surfaceColor))
	return fill.Render(label + strings.Repeat(" ", pad))
}

// listRow renders one command: the selection pointer, the name with its
// fuzzy-match hits picked out, and the arg badge pushed to the right edge so
// the badges line up down the panel. The selected row is filled across the
// full width, which reads as a bar — where the inverse video it replaced
// turned the whole row into a bright block.
func listRow(entry library.Entry, selected bool, inner int, query string) string {
	pointer := "  "
	if selected {
		pointer = "❯ "
	}
	badge := argBadge(entry.Command, selected)
	nameWidth := max(0, inner-ansi.StringWidth(pointer)-ansi.StringWidth(badge))
	name := ansi.Truncate(entry.Name, nameWidth, "…")
	gap := max(0, nameWidth-ansi.StringWidth(name))

	return onSelected(accentStyle, selected).Render(pointer) +
		highlightName(query, name, selected) +
		onSelected(lipglossPlain, selected).Render(strings.Repeat(" ", gap)) +
		badge
}

// gettingStarted is the detail panel for an empty Library — the first thing a
// new user sees, so it says what potato is for and which keys to press rather
// than reporting that there is nothing to show.
func gettingStarted(inner int) []string {
	out := dimLines("Potato keeps the long commands you can never remember, and hands them back to your shell.", inner)
	for _, k := range []footerKey{
		{"^A", "add your first command"},
		{"↵ ", "hand it to your shell"},
		{"^Y", "copy it instead"},
	} {
		out = append(out, accentStyle.Bold(true).Render(k.chord)+dimStyle.Render("  "+k.label))
	}
	out = append(out, "")
	return append(out, dimLines("Write {{name}} or {{name=default}} in a command and potato asks for the value before handing it over.", inner)...)
}

// noMatch is the detail panel when the query filters everything out.
func noMatch(query string, inner int) []string {
	out := dimLines(fmt.Sprintf("Nothing in your library matches '%s'.", query), inner)
	return append(out, accentStyle.Bold(true).Render("^A")+dimStyle.Render("  add it as a new command"))
}

// dimLines wraps text to the panel and dims every row of it, closing with the
// blank row that separates it from whatever follows.
func dimLines(text string, inner int) []string {
	var out []string
	for _, line := range wrapOrNot(text, inner) {
		out = append(out, dimStyle.Render(line))
	}
	return append(out, "")
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
		content = append(content, placeholderRows(ps, true)...)
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
			out = append(out, dimStyle.Render("$ ")+textStyle.Render(strings.TrimPrefix(line, "$ ")))
			continue
		}
		out = append(out, textStyle.Render(line))
	}
	return out
}

// placeholderRows lists a Command's Placeholders, one row each. The detail
// panel indents them under its `args:` label; the edit screen's own panel has
// a frame that says the same thing, so it takes them flush.
func placeholderRows(ps []placeholders.Placeholder, indent bool) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		row := highlightStyle.Render(p.Name)
		if indent {
			row = "  " + row
		}
		if p.HasDefault {
			row += dimStyle.Render(" = " + p.Default)
		}
		out = append(out, row)
	}
	return out
}

func argBadge(command string, selected bool) string {
	n := len(placeholders.Parse(command))
	if n == 0 {
		return ""
	}
	return onSelected(accentStyle, selected).Render(fmt.Sprintf(" ⌁%d", n))
}

// highlightName paints the subsequence match positions in the brand's
// brightest gold, carrying the selection fill through both runs.
func highlightName(query, name string, selected bool) string {
	plain := onSelected(boldStyle.Foreground(lipgloss.Color(textColor)), selected)
	hit := onSelected(boldStyle.Foreground(lipgloss.Color(highlightColor)), selected)
	matches, ok := search.NameMatchIndices(query, name)
	if !ok {
		return plain.Render(name)
	}
	var b strings.Builder
	for i, r := range []rune(name) {
		// the name may have been truncated to fit the panel, so a match index
		// past its end simply has nothing left to paint
		if i < len(matches) && matches[i] {
			b.WriteString(hit.Render(string(r)))
			continue
		}
		b.WriteString(plain.Render(string(r)))
	}
	return b.String()
}
