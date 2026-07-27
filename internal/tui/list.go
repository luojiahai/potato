// The list screen: a full-width fuzzy-search list, with the selected Command's
// detail in a strip under it.
//
// The list used to be thirty columns of names beside a detail pane, which gave
// a Command's text under half the terminal — on a tool whose whole premise is
// commands too long to remember. Full width lets the row carry a dim preview of
// the command itself, so the list can be scanned without walking the selection
// down it, and lets the detail strip show the command unwrapped.

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

// focus is which of the list screen's two zones has the keyboard. The screen is
// split this way so the search field can keep the whole readline key set: every
// verb lives in the zone where nothing is being spelled, and there a bare
// letter costs nothing. See keys.go for why that matters.
type focus int

const (
	// focusSearch: the query field. Every chord it does not claim belongs to
	// readline — which is the point. ^A is line-start again.
	focusSearch focus = iota
	// focusList: the results. The field is blurred, so the verbs can be plain
	// letters and an unbound key is dropped rather than typed.
	focusList
)

type listScreen struct {
	input textinput.Model
	sel   int
	focus focus
	// confirming holds the id of the Command awaiting a delete confirmation,
	// or "". The confirm is inline rather than a screen of its own so the
	// detail strip keeps showing what is about to be deleted while you answer.
	confirming string
}

func newListScreen(m *Model) *listScreen {
	s := &listScreen{input: newField()}
	// An empty Library has nothing to search and one thing to do, and `a` has
	// to work on the first key a new user presses.
	if len(m.lib.Commands) == 0 {
		s.focus = focusList
		return s
	}
	s.setFocus(focusSearch)
	return s
}

// setFocus moves the keyboard between the screen's two zones. The caret goes
// with it: the field that has it is the field that will receive a keystroke.
func (s *listScreen) setFocus(f focus) {
	s.focus = f
	if f != focusSearch {
		s.input.Blur()
		return
	}
	// The blink command is dropped, the way the Model drops the one its own
	// caret clock hands back. Focus alone leaves the caret solid; the next
	// keystroke re-arms the blink. Returning it here would start the field
	// blinking on a tab round-trip but not on launch, which reads as two
	// different carets rather than one that has just been handed the keyboard.
	s.input.Focus()
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

	// While a confirm is open it owns the keyboard: the answer is one
	// keystroke, and letting anything else through would let the query change
	// under a confirm that names a Command by id. It is checked before the
	// focus branch — from the list zone, `y` is copy, and a confirm that read
	// it that way would copy the Command it was asking to delete.
	if s.confirming != "" {
		if key.Matches(keyMsg, keymap.confirm.Yes) {
			return s.delete(m)
		}
		s.confirming = ""
		return nil
	}

	if s.focus == focusList {
		return s.fromList(m, keyMsg)
	}
	return s.fromSearch(m, keyMsg)
}

// fromSearch handles the screen with the search field holding the keyboard.
// Only the chords that cannot be typed are claimed; everything else — ^A, ^E,
// home, end, ^K, ^U, ^W — is the field's own.
func (s *listScreen) fromSearch(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keymap.search.Actions):
		s.setFocus(focusList)
		return nil
	case key.Matches(msg, keymap.search.Quit):
		return m.quit()
	case key.Matches(msg, keymap.search.Run):
		return s.run(m)
	case key.Matches(msg, keymap.search.Copy):
		return s.copy(m)
	case key.Matches(msg, keymap.search.Up):
		s.move(m, -1)
		return nil
	case key.Matches(msg, keymap.search.Down):
		s.move(m, +1)
		return nil
	}

	before := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	// Editing the query resets the selection; pure cursor motion does not.
	// This is the only place the query can change, which is what lets the
	// selection survive a round trip through the list zone.
	if s.input.Value() != before {
		s.sel = 0
	}
	return cmd
}

// fromList handles the screen with the results holding the keyboard: the verbs
// are plain letters, and a key that is not one of them is dropped rather than
// typed into a field that is not listening.
func (s *listScreen) fromList(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keymap.list.Search):
		s.setFocus(focusSearch)
	case key.Matches(msg, keymap.list.Quit):
		return m.quit()
	case key.Matches(msg, keymap.list.Add):
		m.screen = newEditScreen(m, nil)
	case key.Matches(msg, keymap.list.Edit):
		if selected := s.selected(m); selected != nil {
			m.screen = newEditScreen(m, selected)
		}
	case key.Matches(msg, keymap.list.Delete):
		if selected := s.selected(m); selected != nil {
			s.confirming = selected.ID
		}
	case key.Matches(msg, keymap.list.Run):
		return s.run(m)
	case key.Matches(msg, keymap.list.Copy):
		return s.copy(m)
	case key.Matches(msg, keymap.list.Up):
		s.move(m, -1)
	case key.Matches(msg, keymap.list.Down):
		s.move(m, +1)
	}
	return nil
}

// run and copy are the two verbs both zones share, and the two that divert to
// the arg form when the Command has Placeholders to fill in first.
func (s *listScreen) run(m *Model) tea.Cmd {
	selected := s.selected(m)
	if selected == nil {
		return nil
	}
	if len(placeholders.Parse(selected.Command)) > 0 {
		m.screen = newArgsScreen(m, selected)
		return nil
	}
	return m.run(selected.ID, map[string]string{})
}

func (s *listScreen) copy(m *Model) tea.Cmd {
	selected := s.selected(m)
	if selected == nil {
		return nil
	}
	if len(placeholders.Parse(selected.Command)) > 0 {
		m.screen = newArgsScreen(m, selected)
		return m.flashDefault("needs args — fill in, then " + keymap.args.Copy.Help().Key)
	}
	return m.copy(selected.ID, map[string]string{})
}

// move walks the selection, clamping the old value into range first — a query
// may have shortened the results since it was last set.
func (s *listScreen) move(m *Model, delta int) {
	last := len(s.results(m)) - 1
	s.sel = max(0, min(last, min(s.sel, last)+delta))
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
		return footerKeys(keymap.confirm.Yes, keymap.confirm.No)
	}
	if s.focus == focusList {
		return footerKeys(keymap.list.Run, keymap.list.Add, keymap.list.Edit,
			keymap.list.Delete, keymap.list.Copy, keymap.list.Search)
	}
	return footerKeys(keymap.search.Run, keymap.search.Copy,
		keymap.search.Actions, keymap.search.Quit)
}

func (s *listScreen) view(m *Model) []string {
	return pin(s.content(m), nil, m.bodyHeight())
}

// content is the list screen's rows before they are padded out to the frame's
// height. Model.measure sizes the frame from this, so it has to be the rows the
// screen actually wants rather than the rows it ends up drawing.
func (s *listScreen) content(m *Model) []string {
	width := m.innerWidth()
	body := m.bodyHeight()
	query := s.input.Value()
	results := s.results(m)
	sel := min(s.sel, max(0, len(results)-1))

	// The brand rides the frame's top rule the way every section label rides
	// its own, and the version rides the right end — one row for what the
	// seven-row wordmark and its strapline used to spend eight on. It gives the
	// frame a top edge to answer the footer's bottom one.
	top := []string{
		rule(width, "potato", brandStyle(), versionLabel()),
		s.searchRow(m, results, width),
		rule(width, "", lipglossPlain, ""),
	}

	if len(results) == 0 {
		var rows []string
		if len(m.lib.Commands) == 0 {
			rows = s.gettingStarted(width - 2)
		} else {
			rows = s.noMatch(query, width-2)
		}
		return append(top, indent(rows)...)
	}

	// The detail strip follows the list rather than pinning to the bottom of
	// the screen. Pinned, a three-command library put it thirteen rows below
	// the row it described — the strip read as an unrelated status bar instead
	// of as the selection's own detail. Following the list, it only ever moves
	// when the result count does, which is to say while you are typing a query
	// and looking at the search row anyway; it never moves under an arrow key.
	detail := s.detail(m, results, sel, width)
	rows := s.listRows(m, results, sel, max(0, body-len(top)-len(detail)), width, query)
	return append(append(top, rows...), detail...)
}

// searchRow is the prompt and the result count on one row — where the search
// field used to cost three, two of them border. The count yields to the query
// as the row narrows: what you are typing outranks how much it found, and the
// sort order outranks neither.
func (s *listScreen) searchRow(m *Model, results []library.Entry, width int) string {
	// The glyph carries focus the way an edit screen's section label does:
	// accent when this field will receive the next keystroke, muted when the
	// list below has taken the keyboard. The caret says the same thing — a
	// blurred field draws none — and the footer says it in words.
	glyph := focusStyle()
	if s.focus != focusSearch {
		glyph = sectionStyle()
	}
	left := glyph.Render("⌕ ") + s.input.View()
	counts := fmt.Sprintf("%d/%d", len(results), len(m.lib.Commands))
	for _, right := range []string{counts + " · recently used", counts, ""} {
		if s.input.Value() != "" && strings.HasSuffix(right, "recently used") {
			continue
		}
		gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
		if gap < 1 && right != "" {
			continue
		}
		return left + strings.Repeat(" ", max(0, gap)) + dimStyle.Render(right)
	}
	return left
}

// ---------- the detail strip ----------

// detail renders the selected Command below the list, pinned above the footer.
//
// Its height is measured across every result rather than from the selected one,
// so it does not change as the selection moves — a strip that grew and shrank
// would move the list's bottom edge, and with it the scroll window, on every
// press of ↓. The price is blank rows under a short Command in a library that
// holds a long one, and blank rows above the footer cost nothing: they are not
// fenced in a box, so they read as space rather than as something missing.
func (s *listScreen) detail(m *Model, results []library.Entry, sel, width int) []string {
	// Everything here is measured against the terminal's ceiling rather than
	// the frame's height, because the frame's height is measured from this.
	// Reading bodyHeight would size the strip from a number that is only
	// settled once the strip has been sized.
	ceiling := m.ceiling()
	// Below this the list is the only thing worth having.
	if ceiling < 10 {
		return nil
	}
	inner := width - 2

	need := 0
	for _, entry := range results {
		if n := len(s.detailContent(m, entry, inner)); n > need {
			need = n
		}
	}
	// Eight rows carries a description, a command wrapped over three, and the
	// arguments it will ask for — and never more than half the frame, so the
	// strip cannot crowd out the list it belongs to.
	rows := min(max(need, 1), min(detailMaxRows, max(1, ceiling/2)))

	content := s.detailContent(m, results[sel], inner)
	if len(content) > rows {
		content = append(content[:rows-1:rows-1], dimStyle.Render("…"))
	}
	for len(content) < rows {
		content = append(content, "")
	}
	// the blank row that keeps the strip's rule off the last list row
	return append([]string{""}, section(width, results[sel].Name, titleStyle(), "", content)...)
}

// detailContent builds the detail strip's rows: what the Command is for, what
// it is, and what it will ask for. When it was last used lives on the list row
// instead, where it can be compared against its neighbours.
func (s *listScreen) detailContent(m *Model, entry library.Entry, inner int) []string {
	var content []string
	if entry.Description != nil && *entry.Description != "" {
		for _, line := range wrapLines(*entry.Description, inner) {
			content = append(content, dimStyle.Render(line))
		}
	}
	content = append(content, commandBlock(entry.Command, inner)...)
	return append(content, placeholderRows(placeholders.Parse(entry.Command), inner, true)...)
}

// ---------- the list ----------

// rowLayout is the column geometry every row of one frame shares, measured once
// across the results so names and badges line up down the list.
type rowLayout struct {
	name    int
	preview int // 0 when the terminal is too narrow to carry one
	meta    int
	gap     int // the columns between the preview and the meta
}

// previewFloor is the width below which a row shows its name alone. A command
// preview narrower than this is more ellipsis than command.
const previewFloor = 24

// detailMaxRows caps the detail strip; see listScreen.detail.
const detailMaxRows = 8

func (s *listScreen) columns(m *Model, results []library.Entry, width int) rowLayout {
	meta, longest := 0, 0
	for _, entry := range results {
		if _, n := rowMeta(m, entry, false); n > meta {
			meta = n
		}
		if n := ansi.StringWidth(entry.Name); n > longest {
			longest = n
		}
	}
	// The meta column yields first, and whole rather than truncated: a name cut
	// to `depl…` costs more than not knowing how long ago it was used, and half
	// a badge says nothing at all.
	l := rowLayout{}
	if meta > 0 && width-2-meta-2 >= 8 {
		l.meta, l.gap = meta, 2
	}

	// what the pointer and the meta column leave for the name and the preview
	free := max(1, width-2-l.meta-l.gap)
	// A name column sized to the longest name would swing with the query; the
	// clamp keeps it inside a band wide enough to read and narrow enough to
	// leave the preview something.
	l.name = min(min(max(longest, 12), 32), free)
	if rest := free - l.name - 2; rest >= previewFloor {
		l.preview = rest
	} else {
		l.name = free
	}
	return l
}

// listRows renders the visible slice of the results. When they overflow the
// list the first and last rows are given over to the counts still hidden —
// reserved at both ends whether or not both ends have anything to report, so
// the rows between them hold still as the selection moves.
func (s *listScreen) listRows(m *Model, results []library.Entry, sel, rows, width int, query string) []string {
	if rows <= 0 {
		return nil
	}
	l := s.columns(m, results, width)
	if len(results) <= rows {
		out := make([]string, 0, len(results))
		for i, entry := range results {
			out = append(out, s.rowFor(m, entry, i == sel, l, query))
		}
		return out
	}

	// Reserving both counters costs two of the rows there are to give. Below
	// three, that is all of them but one, and the counters would be spending
	// the budget the list rows and the detail strip under them were counted
	// into — the strip would lose its last line to a row saying how many lines
	// were not shown.
	// Reserving both counters costs two of the rows there are to give. Below
	// three, that is all of them but one, and the counters would be spending
	// the budget the list rows and the detail strip under them were counted
	// into — the strip would lose its last line to a row saying how many lines
	// were not shown.
	visible := rows
	if rows >= 3 {
		visible = rows - 2
	}
	start := max(0, min(sel-visible+1, len(results)-visible))
	end := min(len(results), start+visible)

	out := make([]string, 0, rows)
	if rows < 3 {
		for i := start; i < end; i++ {
			out = append(out, s.rowFor(m, results[i], i == sel, l, query))
		}
		return out
	}
	out = append(out, overflowRow("↑", start))
	for i := start; i < end; i++ {
		out = append(out, s.rowFor(m, results[i], i == sel, l, query))
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
// rather than the screen, so the detail strip below it goes on showing the
// command you are about to lose — and now has the width to name it.
func (s *listScreen) rowFor(m *Model, entry library.Entry, selected bool, l rowLayout, query string) string {
	width := 2 + l.name + l.meta + l.gap
	if l.preview > 0 {
		width += l.preview + 2
	}
	if entry.ID == s.confirming {
		label := fmt.Sprintf("⚠ delete '%s'? y/n", entry.Name)
		label = ansi.Truncate(label, width, "…")
		fill := dangerStyle.Bold(true).Background(lipgloss.Color(surfaceColor))
		return fill.Render(label + strings.Repeat(" ", max(0, width-ansi.StringWidth(label))))
	}

	pointer := "  "
	if selected {
		pointer = "❯ "
	}
	fill := onSelected(lipglossPlain, selected)
	out := onSelected(accentStyle, selected).Render(pointer) + column(highlightName(query, entry.Name, l.name, selected), l.name, selected)

	if l.preview > 0 {
		out += fill.Render("  ") + column(onSelected(dimStyle, selected).Render(oneLine(entry.Command, l.preview)), l.preview, selected)
	}
	if l.meta > 0 {
		rendered, w := rowMeta(m, entry, selected)
		out += fill.Render(strings.Repeat(" ", l.gap+max(0, l.meta-w))) + rendered
	}
	return out
}

// column pads a rendered cell out to its column width, carrying the selection
// fill through the padding so the bar has no holes in it.
func column(rendered string, width int, selected bool) string {
	pad := max(0, width-ansi.StringWidth(rendered))
	return rendered + onSelected(lipglossPlain, selected).Render(strings.Repeat(" ", pad))
}

// oneLine flattens a Command to a single row of preview text. A Command may
// hold newlines — potato stores a Continuation's backslash and newline verbatim
// — and a row is one row.
func oneLine(command string, width int) string {
	flat := strings.Join(strings.Fields(strings.ReplaceAll(command, "\n", " ")), " ")
	return ansi.Truncate(flat, width, "…")
}

// rowMeta is the right-hand end of a row: how many arguments the Command asks
// for, and when it was last used. Returns the rendered string and its width,
// which the layout needs before it can right-align the column.
func rowMeta(m *Model, entry library.Entry, selected bool) (string, int) {
	var parts []run
	if n := len(placeholders.Parse(entry.Command)); n > 0 {
		parts = append(parts, run{text: fmt.Sprintf("⌁%d", n), style: onSelected(accentStyle, selected)})
	}
	if used := m.st[entry.ID].LastUsedAt; used != "" {
		if ago := timeAgo(used, m.deps.Now()); ago != "" {
			if len(parts) > 0 {
				parts = append(parts, run{text: " · ", style: onSelected(dimStyle, selected)})
			}
			parts = append(parts, run{text: ago, style: onSelected(dimStyle, selected)})
		}
	}
	var b strings.Builder
	width := 0
	for _, p := range parts {
		b.WriteString(p.style.Render(p.text))
		width += ansi.StringWidth(p.text)
	}
	return b.String(), width
}

// ---------- empty states ----------

// gettingStarted fills the list region for an empty Library — the first thing a
// new user sees, so it says what potato is for and which keys to press rather
// than reporting that there is nothing to show.
func (s *listScreen) gettingStarted(inner int) []string {
	out := dimLines("Potato keeps the long commands you can never remember, and hands them back to your shell.", inner)
	out = append(out, chordRows([]footerKey{
		{Key: s.addChord(), Desc: "add your first command"},
		{Key: keymap.list.Run.Help().Key, Desc: "hand it to your shell"},
		{Key: s.copyChord(), Desc: "copy it instead"},
	})...)
	out = append(out, "")
	return append(out, dimLines("Write {{name}} or {{name=default}} in a command and potato asks for the value before handing it over.", inner)...)
}

// noMatch fills the list region when the query filters everything out.
func (s *listScreen) noMatch(query string, inner int) []string {
	out := dimLines(fmt.Sprintf("Nothing in your library matches '%s'.", query), inner)
	return append(out, chordRows([]footerKey{{Key: s.addChord(), Desc: "add it as a new command"}})...)
}

// addChord and copyChord are what to press from wherever the keyboard is. The
// verbs live with the list, so from the search field the answer is two presses
// — which is why an empty Library starts in the list zone, where it is one.
func (s *listScreen) addChord() string {
	if s.focus == focusList {
		return keymap.list.Add.Help().Key
	}
	return keymap.search.Actions.Help().Key + " " + keymap.list.Add.Help().Key
}

func (s *listScreen) copyChord() string {
	if s.focus == focusList {
		return keymap.list.Copy.Help().Key
	}
	return keymap.search.Copy.Help().Key
}

// chordRows renders a column of chord-and-label rows from the bindings
// themselves, the chords padded so the labels line up whatever they are.
func chordRows(rows []footerKey) []string {
	w := 0
	for _, r := range rows {
		w = max(w, ansi.StringWidth(r.Key))
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		pad := strings.Repeat(" ", w-ansi.StringWidth(r.Key))
		out = append(out, accentStyle.Bold(true).Render(r.Key)+pad+dimStyle.Render("  "+r.Desc))
	}
	return out
}

// dimLines wraps text and dims every row of it, closing with the blank row that
// separates it from whatever follows.
func dimLines(text string, inner int) []string {
	var out []string
	for _, line := range wrapLines(text, inner) {
		out = append(out, dimStyle.Render(line))
	}
	return append(out, "")
}

// ---------- shared renderers ----------

// commandBlock renders a Command with its `$ ` gutter, its Placeholders picked
// out, wrapped to width.
func commandBlock(command string, inner int) []string {
	runs := []run{{text: "$ ", style: dimStyle}}
	for _, seg := range placeholders.TemplateSegments(command) {
		style := textStyle
		if seg.Flag {
			style = highlightStyle.Bold(true)
		}
		runs = append(runs, run{text: seg.Text, style: style})
	}
	return wrapStyled(runs, inner)
}

// placeholderRows lists a Command's Placeholders, one row each. The detail
// strip indents them under the command they belong to; the edit screen's own
// section has a rule that says the same thing, so it takes them flush.
func placeholderRows(ps []placeholders.Placeholder, width int, indent bool) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		row := highlightStyle.Render(p.Name)
		if indent {
			row = "  " + row
		}
		if p.HasDefault {
			row += dimStyle.Render(" = " + p.Default)
		}
		// A default can be arbitrarily long — it is whatever the Command's
		// author typed between `=` and `}}`.
		out = append(out, ansi.Truncate(row, width, "…"))
	}
	return out
}

// highlightName paints the subsequence match positions in the brand's
// brightest gold, carrying the selection fill through both runs.
func highlightName(query, name string, width int, selected bool) string {
	name = ansi.Truncate(name, width, "…")
	plain := onSelected(boldStyle.Foreground(lipgloss.Color(textColor)), selected)
	hit := onSelected(boldStyle.Foreground(lipgloss.Color(highlightColor)), selected)
	matches, ok := search.NameMatchIndices(query, name)
	if !ok {
		return plain.Render(name)
	}
	var b strings.Builder
	for i, r := range []rune(name) {
		// the name may have been truncated to fit the column, so a match index
		// past its end simply has nothing left to paint
		if i < len(matches) && matches[i] {
			b.WriteString(hit.Render(string(r)))
			continue
		}
		b.WriteString(plain.Render(string(r)))
	}
	return b.String()
}
