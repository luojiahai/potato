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
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/placeholders"
	"github.com/luojiahai/potato/internal/search"
	"github.com/luojiahai/potato/internal/state"
)

type listScreen struct {
	query field
	sel   int
	// confirming holds the id of the Command awaiting a delete confirmation,
	// or "". The confirm is inline rather than a screen of its own so the
	// detail strip keeps showing what is about to be deleted while you answer.
	confirming string
}

// newListScreen builds the screen with the search field holding the keyboard,
// which it keeps for the life of the screen: every verb is a chord the field
// does not claim, so the keyboard never has to leave it. See keys.go for why
// the verbs are the chords they are.
func newListScreen() *listScreen {
	s := &listScreen{query: newField(lineMode)}
	s.query.Focus()
	return s
}

func (s *listScreen) results(m *Model) []library.Command {
	return search.Commands(m.lib.Commands, m.st, s.query.Value())
}

func (s *listScreen) selected(m *Model) *library.Command {
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
		cmd, _ := s.query.Update(msg)
		return cmd
	}

	// While a confirm is open it owns the keyboard: the answer is one
	// keystroke, and letting anything else through would let the query change
	// under a confirm that names a Command by id — the `y` that answers it
	// must never reach the field.
	if s.confirming != "" {
		if key.Matches(keyMsg, keymap.confirm.Yes) {
			return s.delete(m)
		}
		s.confirming = ""
		return nil
	}

	return s.handleKey(m, keyMsg)
}

// handleKey dispatches a keystroke. Only the chords the field does not claim
// are matched; everything else — every letter, ^A, ^E, home, end, ^K, ^U, ^W
// — is the field's own.
func (s *listScreen) handleKey(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keymap.list.Quit):
		return m.quit()
	case key.Matches(msg, keymap.list.Run):
		return s.run(m)
	case key.Matches(msg, keymap.list.Copy):
		return s.copy(m)
	case key.Matches(msg, keymap.list.Add):
		m.screen = newEditScreen(m, nil)
		return nil
	case key.Matches(msg, keymap.list.Edit):
		if selected := s.selected(m); selected != nil {
			m.screen = newEditScreen(m, selected)
		}
		return nil
	case key.Matches(msg, keymap.list.Delete):
		if selected := s.selected(m); selected != nil {
			s.confirming = selected.ID
		}
		return nil
	case key.Matches(msg, keymap.list.Up):
		s.move(m, -1)
		return nil
	case key.Matches(msg, keymap.list.Down):
		s.move(m, +1)
		return nil
	case key.Matches(msg, keymap.list.Tab):
		return nil
	}

	// Editing the query resets the selection; pure cursor motion does not. The
	// field reports the difference, and this is the only place the query can
	// change.
	cmd, edited := s.query.Update(msg)
	if edited {
		s.sel = 0
	}
	return cmd
}

// run and copy are the two verbs that divert to the arg form when the Command
// has Placeholders to fill in first.
func (s *listScreen) run(m *Model) tea.Cmd {
	selected := s.selected(m)
	if selected == nil {
		return nil
	}
	if len(placeholders.Parse(selected.Template)) > 0 {
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
	if len(placeholders.Parse(selected.Template)) > 0 {
		m.screen = newArgsScreen(m, selected)
		return m.flashDefault("Needs args — fill in, then " + keymap.args.Copy.Help().Key)
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
//
// Both files are written: the Library loses the Command and State loses its
// cache entry. They are separate calls because they are separate files with
// separate lifetimes — see docs/adr/0002 — and this is the one place that knows
// a Command is being destroyed rather than merely edited.
func (s *listScreen) delete(m *Model) tea.Cmd {
	command, ok := library.Find(m.lib, s.confirming)
	s.confirming = ""
	if !ok {
		return nil
	}
	saved := m.updateLibrary(library.Remove(m.lib, command.ID))
	forgotten := m.updateState(state.Forget(m.st, command.ID))
	return m.finish(fmt.Sprintf("Deleted '%s'", command.Name), saved, forgotten)
}

func (s *listScreen) keys(*Model) []footerKey {
	if s.confirming != "" {
		return footerKeys(keymap.confirm.Yes, keymap.confirm.No)
	}
	return footerKeys(keymap.list.Run, keymap.list.Add, keymap.list.Edit,
		keymap.list.Delete, keymap.list.Copy, keymap.list.Quit)
}

func (s *listScreen) view(m *Model) []string {
	return pin(s.content(m), nil, m.bodyHeight())
}

// content is the list screen's rows before they are padded out to the frame's
// height.
func (s *listScreen) content(m *Model) []string {
	width := m.innerWidth()
	body := m.bodyHeight()
	query := s.query.Value()
	results := s.results(m)
	sel := min(s.sel, max(0, len(results)-1))

	// The brand rides the frame's top rule the way every section label rides
	// its own, and the version rides the right end — one row for what the
	// seven-row wordmark and its strapline used to spend eight on. It gives the
	// frame a top edge to answer the footer's bottom one.
	//
	// The potato is the one glyph here that costs two columns rather than one.
	// Nothing needs to know that: rule measures its label with StringWidth, so
	// the dashes it draws are already short by two, and the narrow-width tests
	// hold the line at 40 columns.
	top := []string{
		rule(width, brand, brandStyle(), versionLabel()),
		s.searchRow(m, results, width),
		rule(width, "", lipglossPlain, ""),
	}

	if len(m.lib.Commands) == 0 {
		return append(top, indent(s.gettingStarted(width-2))...)
	}

	// Both regions are as fixed as the frame they sit in: the list keeps its
	// seven rows — padded out rather than handing spare rows down when the
	// results run short, holding the no-match note when they run out — and
	// the strip's height comes from the frame alone, so neither a query nor
	// an arrow key can move the strip's rule or the list's bottom edge.
	budget, detailRows := regions(body, len(top))
	var rows []string
	var command *library.Command
	if len(results) == 0 {
		rows = indent(s.noMatch(query, width-2))
		if len(rows) > budget {
			rows = rows[:budget]
		}
	} else {
		command = &results[sel]
		rows = s.listRows(m, results, sel, budget, width, query)
	}
	detail := s.detail(command, detailRows, width)
	for len(rows) < budget {
		rows = append(rows, "")
	}
	return append(append(top, rows...), detail...)
}

// regions splits the frame's body below the top rows between the two fixed
// regions. The list's rows come first: the strip's content gets what the body
// leaves after them and the strip's own blank and rule, up to its cap. On a
// terminal too short for both, the strip is the one to go — below that the
// list is the only thing worth having.
func regions(body, top int) (list, detail int) {
	detail = max(0, min(detailMaxRows, body-top-listRegionRows-2))
	strip := 0
	if detail > 0 {
		strip = detail + 2 // its blank and its rule
	}
	list = max(0, min(listRegionRows, body-top-strip))
	return list, detail
}

const (
	searchGlyph = "⌕ "
	// searchGlyphWidth is what the glyph and its space cost the field beside
	// it. Written out because the field has to be given its width before
	// anything is rendered, so there is nothing to measure yet.
	searchGlyphWidth = 2
)

// searchColumns is the search row's shape: the glyph, the query, and the count
// in whatever the query leaves behind it.
//
// The count trails rather than holding a column of its own, so the field is
// given the row less its glyph. A query longer than that slides under its own
// caret rather than being windowed into a narrower column — or running off the
// end of the frame to be cut by the clamp in View, which took the caret with it.
var searchColumns = arrangement{
	{spend: spendFixed, n: searchGlyphWidth},
	{spend: spendFlex, needs: 1},
	{spend: spendWidest, align: alignTrailing, needs: 1},
}

// searchRow is the prompt and the result count on one row — where the search
// field used to cost three, two of them border. The count yields to the query
// as the row narrows: what you are typing outranks how much it found, and the
// sort order outranks neither.
func (s *listScreen) searchRow(m *Model, results []library.Command, width int) string {
	// The glyph carries focus the way an edit screen's section label does:
	// accent, because this field always receives the next keystroke.
	glyph := focusStyle()
	counts := fmt.Sprintf("%d/%d", len(results), len(m.lib.Commands))
	// The count's forms, longest first. A query has no sort order to report, so
	// that form is not offered rather than being offered and rejected.
	forms := []string{counts + " · Recently used", counts, ""}
	if s.query.Value() != "" {
		forms = forms[1:]
	}

	on := m.caretOn()
	candidates := make([]candidate, 0, len(forms))
	for _, form := range forms {
		candidates = append(candidates, candidate{
			columns: searchColumns,
			rows: []blockRow{{cells: []cell{
				textCell(searchGlyph, glyph),
				fieldCell(&s.query, on),
				textCell(form, dimStyle),
			}}},
		})
	}
	return layout(width, candidates...)[0]
}

// ---------- the detail strip ----------

// detail renders the selected Command below the list, pinned above the footer.
//
// Its height comes from the frame alone, the way the frame's comes from the
// terminal: no query, keystroke or Command can grow or shrink it. The price
// is blank rows under a short Command, and blank rows cost nothing — they are
// not fenced in a box, so they read as space rather than as something missing
// — while a Command too long for the strip is cut with an ellipsis. With no
// command to show it keeps its rows and shows nothing, so a query that filters
// everything out cannot move the rule either.
func (s *listScreen) detail(command *library.Command, rows, width int) []string {
	if rows < 1 {
		return nil
	}
	var content []string
	if command != nil {
		content = s.detailContent(*command, width-2)
		if len(content) > rows {
			content = append(content[:rows-1:rows-1], dimStyle.Render("…"))
		}
	}
	for len(content) < rows {
		content = append(content, "")
	}
	// the blank row that keeps the strip's rule off the last list row
	return append([]string{""}, section(width, "", lipglossPlain, "", content)...)
}

// The detail strip's field labels, named so the gutter can be computed from
// the set — renaming one cannot silently misalign the column under it.
const (
	labelName         = "Name"
	labelDescription  = "Description"
	labelCommand      = "Command"
	labelPlaceholders = "Placeholders"
)

// detailGutter is the column the detail strip's values hang from: its widest
// label and the two columns after it. The gap is not free — the strip's rows
// are fixed, so every gutter column narrows the values and wraps a long
// Command that much sooner.
var detailGutter = 2 + max(len(labelName), len(labelDescription), len(labelCommand), len(labelPlaceholders))

// detailContent builds the detail strip's rows: what the Command is called,
// what it is for, what it is, and what it will ask for — each field headed by
// its label in the shared gutter. When it was last used lives on the list row
// instead, where it can be compared against its neighbours.
func (s *listScreen) detailContent(command library.Command, inner int) []string {
	value := max(1, inner-detailGutter)
	var content []string
	add := func(label string, rows []string) {
		for i, row := range rows {
			head := strings.Repeat(" ", detailGutter)
			if i == 0 {
				head = sectionStyle().Render(label) +
					strings.Repeat(" ", max(1, detailGutter-len(label)))
			}
			content = append(content, head+row)
		}
	}

	var name []string
	for _, line := range wrapLines(command.Name, value) {
		name = append(name, titleStyle().Render(line))
	}
	add(labelName, name)
	if command.Description != nil && *command.Description != "" {
		var desc []string
		for _, line := range wrapLines(*command.Description, value) {
			desc = append(desc, dimStyle.Render(line))
		}
		add(labelDescription, desc)
	}
	add(labelCommand, commandBlock(command.Template, value))
	if ps := placeholders.Parse(command.Template); len(ps) > 0 {
		add(labelPlaceholders, placeholderRows(ps, value))
	}
	return content
}

// ---------- the list ----------

// previewFloor is the width below which a row shows its name alone. A command
// preview narrower than this is more ellipsis than command.
const previewFloor = 24

// The list row's four columns, in the order they are drawn.
const (
	colPointer = iota
	colName
	colPreview
	colMeta
)

// listColumns is one of the list row's four arrangements.
//
// The preview yields first: it is the one column whose absence costs nothing
// you cannot get by moving the selection onto the row. When it goes, the name
// stops being held inside its band and takes the whole remainder — which is the
// reason these are arrangements rather than one arrangement with columns
// dropped out of it. Losing the preview does not just free the name's
// neighbour, it changes how the name spends its width.
//
// The meta goes next, and whole rather than truncated: a name cut to `depl…`
// costs more than not knowing how long ago it was used, and half a badge says
// nothing at all. The name is never the column that gives.
func listColumns(preview, meta bool) arrangement {
	a := arrangement{
		colPointer: {spend: spendFixed, n: contentIndentWidth},
		// Sized to the longest name the column would swing with every keystroke
		// of the query; the band keeps it wide enough to read and narrow enough
		// to leave the preview something. With no preview beside it there is
		// nothing to leave, so the name takes the remainder down to the floor
		// every row's own content shares.
		colName:    {spend: spendWidest, clampMin: 12, clampMax: 40},
		colPreview: {spend: spendFlex, lead: 2, needs: previewFloor},
		colMeta:    {spend: spendWidest, lead: 2, align: alignRight},
	}
	if !preview {
		a[colName] = column{spend: spendFlex, needs: contentFloor}
		a[colPreview] = column{spend: spendNone}
	}
	if !meta {
		a[colMeta] = column{spend: spendNone}
	}
	return a
}

// listCandidates is the order the list row gives its columns up in — the four
// combinations of the two it can do without, widest first.
//
// The third is reachable only behind a meta wider than a name and a preview
// together, which no badge potato writes comes close to. It is here because the
// two columns are decided independently: nothing about dropping the preview
// says the meta has to go with it, and enumerating the pair is cheaper than a
// rule explaining why one of them cannot happen.
func listCandidates(rows, sized []blockRow) []candidate {
	shapes := [...]struct{ preview, meta bool }{
		{preview: true, meta: true},
		{preview: false, meta: true},
		{preview: true, meta: false},
		{preview: false, meta: false},
	}
	out := make([]candidate, 0, len(shapes))
	for _, shape := range shapes {
		out = append(out, candidate{
			columns: listColumns(shape.preview, shape.meta),
			rows:    rows,
			sizedBy: sized,
		})
	}
	return out
}

// listRegionRows is the fixed height of the list region: seven rows of
// commands — or five with the overflow counters around them — padded out
// with blanks when the results run short.
const listRegionRows = 7

// detailMaxRows caps the detail strip's content: ten rows is the fullest
// command it is asked to carry — a name, a description, a command wrapped over
// three rows, and five Placeholders.
const detailMaxRows = 10

// listRows renders the visible slice of the results. When they overflow the
// list the first and last rows are given over to the counts still hidden —
// reserved at both ends whether or not both ends have anything to report, so
// the rows between them hold still as the selection moves.
func (s *listScreen) listRows(m *Model, results []library.Command, sel, rows, width int, query string) []string {
	if rows <= 0 {
		return nil
	}
	start, end, counters := 0, len(results), false
	if len(results) > rows {
		// Reserving both counters costs two of the rows there are to give.
		// Below three, that is all of them but one, and the counters would be
		// spending the budget the list rows and the detail strip under them
		// were counted into — the strip would lose its last line to a row
		// saying how many lines were not shown.
		visible := rows
		if rows >= 3 {
			visible, counters = rows-2, true
		}
		start = max(0, min(sel-visible+1, len(results)-visible))
		end = min(len(results), start+visible)
	}

	out := s.block(m, results, results[start:end], sel-start, width, query)
	if !counters {
		return out
	}
	return append(append([]string{overflowRow("↑", start)}, out...),
		overflowRow("↓", len(results)-end))
}

// block lays the visible rows out, sized against every result rather than the
// seven on screen — so the name and meta columns do not resize under the
// selection as it moves down a long list.
func (s *listScreen) block(m *Model, all, visible []library.Command, sel, width int, query string) []string {
	sized := make([]blockRow, 0, len(all))
	for _, command := range all {
		sized = append(sized, blockRow{cells: s.rowCells(m, command, query, false)})
	}
	rows := make([]blockRow, 0, len(visible))
	for i, command := range visible {
		rows = append(rows, blockRow{cells: s.rowCells(m, command, query, i == sel), selected: i == sel})
	}

	out := layout(width, listCandidates(rows, sized)...)

	// The confirm takes over the row rather than the screen, so the detail
	// strip below it goes on showing the command you are about to lose — and
	// now has the width to name it.
	for i, command := range visible {
		if command.ID == s.confirming {
			out[i] = confirmRow(width, command.Name)
		}
	}
	return out
}

// rowCells is one Command's row: the selection pointer, its name with the
// query's hits picked out, a flattened preview of the command itself, and what
// it asks for and when it was last used.
func (s *listScreen) rowCells(m *Model, command library.Command, query string, selected bool) []cell {
	cells := make([]cell, 4)
	cells[colPointer] = pointerCell(selected)
	cells[colName] = runsCell(nameRuns(query, command.Name))
	cells[colPreview] = textCell(oneLine(command.Template), dimStyle)
	cells[colMeta] = runsCell(metaRuns(m, command))
	return cells
}

// pointerCell marks the selected row. Blank on every other row rather than
// absent, so the names below it stay in the column the content indent puts
// everything else in.
func pointerCell(selected bool) cell {
	pointer := contentIndent
	if selected {
		pointer = "❯ "
	}
	return textCell(pointer, accentStyle)
}

// confirmRow is the delete confirm in a Command's place: one run across the
// whole row, on the same bar every selected row wears.
func confirmRow(width int, name string) string {
	return layout(width, candidate{
		columns: arrangement{{spend: spendFlex}},
		rows: []blockRow{{
			selected: true,
			cells: []cell{textCell(
				fmt.Sprintf("⚠ Delete '%s'? y/n", name),
				dangerStyle.Bold(true),
			)},
		}},
	})[0]
}

func overflowRow(arrow string, n int) string {
	if n == 0 {
		return ""
	}
	return dimStyle.Render(fmt.Sprintf("  %s %d more", arrow, n))
}

// oneLine flattens a Command to a single row of preview text. A Command may
// hold newlines, since a hand-edited library file or an import can carry them
// where the single-line edit fields cannot, and a row is one row. Cutting it to
// the column is the Layout's — this only has to make it one line long.
func oneLine(command string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(command, "\n", " ")), " ")
}

// metaRuns is the right-hand end of a row: how many arguments the Command asks
// for, and when it was last used. The selection fill is the Layout's to carry,
// so these are the runs underneath it.
func metaRuns(m *Model, command library.Command) []run {
	var parts []run
	if n := len(placeholders.Parse(command.Template)); n > 0 {
		parts = append(parts, run{text: fmt.Sprintf("⌁%d", n), style: accentStyle})
	}
	if used := m.st[command.ID].LastUsedAt; used != "" {
		if ago := timeAgo(used, m.deps.Now()); ago != "" {
			if len(parts) > 0 {
				parts = append(parts, run{text: " · ", style: dimStyle})
			}
			parts = append(parts, run{text: ago, style: dimStyle})
		}
	}
	return parts
}

// ---------- empty states ----------

// gettingStarted fills the list region for an empty Library — the first thing a
// new user sees, so it says what potato is for and which keys to press rather
// than reporting that there is nothing to show.
func (s *listScreen) gettingStarted(inner int) []string {
	out := dimLines("Potato keeps the long commands you can never remember, and hands them back to your shell.", inner)
	out = append(out, chordRows([]footerKey{
		{Key: keymap.list.Add.Help().Key, Desc: "Add your first command"},
		{Key: keymap.list.Run.Help().Key, Desc: "Hand it to your shell"},
		{Key: keymap.list.Copy.Help().Key, Desc: "Copy it instead"},
	})...)
	out = append(out, "")
	return append(out, dimLines("Write {{name}} or {{name=default}} in a command and potato asks for the value before handing it over.", inner)...)
}

// noMatch fills the list region when the query filters everything out.
func (s *listScreen) noMatch(query string, inner int) []string {
	out := dimLines(fmt.Sprintf("Nothing in your library matches '%s'.", query), inner)
	return append(out, chordRows([]footerKey{{Key: keymap.list.Add.Help().Key, Desc: "Add it as a new command"}})...)
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
	return wrapStyled(withPrompt(templateRuns(command)), inner)
}

// placeholderRows lists a Command's Placeholders, one row each, flush left —
// the detail strip's gutter and the edit screen's section rule each say whose
// they are.
func placeholderRows(ps []placeholders.Placeholder, width int) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		row := highlightStyle.Render(p.Name)
		if p.HasDefault {
			row += dimStyle.Render(" = " + p.Default)
		}
		// A default can be arbitrarily long — it is whatever the Command's
		// author typed between `=` and `}}`.
		out = append(out, ansi.Truncate(row, width, "…"))
	}
	return out
}

// nameRuns paints the subsequence match positions in the brand's brightest
// gold. The name is matched whole and cut by the Layout, which cuts from the
// right — so a hit that falls off the end simply has nothing left to paint.
func nameRuns(query, name string) []run {
	plain := boldStyle.Foreground(lipgloss.Color(textColor))
	hit := boldStyle.Foreground(lipgloss.Color(highlightColor))
	matches, ok := search.NameMatchIndices(query, name)
	if !ok {
		return []run{{text: name, style: plain}}
	}
	out := make([]run, 0, len(name))
	for i, r := range []rune(name) {
		style := plain
		if i < len(matches) && matches[i] {
			style = hit
		}
		out = append(out, run{text: string(r), style: style})
	}
	return out
}
