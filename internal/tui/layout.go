// A Layout is the cells one line of the frame is laid out from, and the
// candidate arrangements that fit them to a width.
//
// Four lines go through a Layout: a list row, an arg row, the search row and
// the delete confirm. The first three each used to carry their own copy of the
// same four decisions — measure the widest cell down the block, decide what to
// give up when the row will not fit, pad what is left, and carry the selection
// fill through every run and every pad — while the confirm reconstructed the
// frame's width by summing those columns back up. The fill is the decision that
// bites. lipgloss cannot cascade a background over an already-rendered escape
// sequence, so the fill has to go on each run as it is built; a run that skips
// it punches a hole in the bar, and nothing short of looking at the screen
// would tell you. A list row applied it in six places and an arg row in five.
//
// Measuring is why a Layout is handed the whole block — the set of lines sized
// together — rather than one line at a time. A column that lines up down the
// list has to know the widest cell in it before it can render the first, which
// is what the list screen's rowLayout and the arg screen's labelWidth were each
// working out on their own. The search row and the confirm are blocks of one.
//
// Giving up is a list of whole arrangements rather than a set of per-column
// priorities, because two of the three that degrade do not merely drop a column
// when they run out of width — they rearrange. A list row's name is clamped to a
// band while there is a preview beside it and takes the whole remainder when
// there is not, and the search row's count shortens ("7/9 · Recently used" →
// "7/9" → nothing) rather than yielding its column. Both fall out of "try these
// arrangements in order, take the first that fits". Neither falls out of a
// priority, and the search row was already written as a candidate list before
// there was a Layout to name it one.

package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// spend is how a column decides its width.
type spend int

const (
	// spendFixed takes exactly what it asks for — a selection pointer, a
	// content indent, the search glyph.
	spendFixed spend = iota
	// spendWidest measures its own cell down the whole block and takes the
	// widest, so the column lines up from row to row however long any one value
	// in it is.
	spendWidest
	// spendFlex takes what the other columns leave.
	spendFlex
	// spendNone is a column this candidate gave up. It takes no width, adds no
	// lead and draws nothing, and it has nothing it needs. It holds its place so
	// that a row's cells still line up with the arrangement however many of them
	// are being shown — a candidate drops a column without renumbering the ones
	// after it, and a block builds its cells once for every candidate rather
	// than once each.
	spendNone
)

// contentFloor is the least a row's own content may be squeezed to before the
// row gives a whole column up instead: the value being typed on an arg row, the
// name being read on a list row. Below eight columns either is more ellipsis
// than content. One constant rather than one per screen, so the arg row and the
// list row cannot drift apart on what "too narrow" means.
const contentFloor = 8

// align is where a column's content sits in its width — and, for the two
// right-hand kinds, whether that width is reserved before flex takes the rest.
type align int

const (
	alignLeft align = iota
	// alignRight reserves its width: the flex column beside it is sized to what
	// is left over. A list row's meta and an arg row's note work this way — the
	// value is given a narrower column so the badge beside it always has room.
	alignRight
	// alignTrailing reserves nothing. It sits in whatever slack the flex column
	// does not fill and vanishes when there is none. The search row's count
	// works this way, and the difference is deliberate: the query is given the
	// whole row so a long one slides under its own caret rather than being
	// windowed to a narrower column, and the count is what gives.
	alignTrailing
)

// column is one column of an arrangement: how it takes its width, where its
// content sits in it, and the least it can be given before the arrangement is
// judged not to fit.
type column struct {
	spend spend
	// n is what a spendFixed column takes.
	n int
	// pad is the columns a spendWidest column adds to what it measured — an arg
	// row's label gutter is its widest Placeholder name and two columns after
	// it. Content is cut to what is left when they are taken back off, so the
	// gap they buy cannot be eaten by the value that earned it.
	pad int
	// clampMin and clampMax bound what spendWidest measures. A list row's name
	// is held between twelve and forty columns: sized to the longest name alone
	// it would swing with every keystroke of the query. Zero is unbounded.
	clampMin, clampMax int
	// needs is the least this column may be allocated. An arrangement that
	// cannot give every column what it needs does not fit, and the next
	// candidate is tried. This is what takes a list row's preview away below
	// twenty-four columns, and its meta when that would leave the name under
	// eight.
	needs int
	// lead is the blank columns before the content, filled with the rest of the
	// row so the selection bar runs through them.
	lead  int
	align align
}

// givenUp is the one question the measuring, the fitting and the painting all
// have to ask of a column, so it is asked in one vocabulary. See spendNone for
// what a given-up column costs and draws, which is nothing.
func (c column) givenUp() bool { return c.spend == spendNone }

// arrangement is one complete set of columns.
type arrangement []column

// candidate is one whole way to lay a block out: its columns and the cells that
// go in them. A Layout renders the first candidate that fits.
//
// The cells belong to the candidate rather than sitting outside the list
// because a row does not only give up columns as it narrows — it can give up
// words. The search row's count shortens ("7/9 · Recently used" → "7/9") before
// it goes altogether, which is the same column holding different content, and
// there is nowhere else for the shorter form to live.
type candidate struct {
	columns arrangement
	rows    []blockRow
	// sizedBy is the block the spendWidest columns are measured against, when
	// that is wider than the block being drawn. The list sizes its name and
	// meta columns across every result rather than the seven rows on screen, so
	// the columns hold still while the selection moves through them. Nil means
	// the rows size themselves.
	sizedBy []blockRow
}

// cell is one column's content in one row.
//
// runs are text and style, which is what lets a Layout put the selection fill
// on them: a style can still be added to before it renders.
//
// rendered is the exception, and the one place a Layout's promise to fill
// everything it draws stops. It is content that arrived with its escape
// sequences already emitted — a Field's row, which paints its own caret while
// it windows its value — and a Layout places it without restyling it. Not by
// preference: a background cannot be cascaded over an escape sequence that has
// already been written, so nobody downstream can fill a pre-rendered cell after
// the fact. The only place the fill can go on is where the sequences are
// written, which is inside the Field. That is why an arg row's value is filled
// today — its Field is given a painter that carries the selection colour (see
// argPaint) — and why it is filled there rather than here. A pass-through cell
// whose drawer has no such painter is drawn plain, and the bar has a hole in it
// that no caller of this package can close.
type cell struct {
	runs     []run
	rendered string
	// draw renders content that cannot be drawn until its column has been
	// sized. A Field windows its value around the caret, so it has to be told
	// how much room it got before it knows what it looks like. Legal only in a
	// flex column, which is the one kind whose width is settled without
	// measuring what goes in it.
	draw func(width int) string
}

// textCell is one run of one style — most cells.
func textCell(text string, style lipgloss.Style) cell {
	return cell{runs: []run{{text: text, style: style}}}
}

// runsCell is a cell already broken into styled runs: a name with its
// fuzzy-match hits picked out, a meta badge with its two halves.
func runsCell(runs []run) cell { return cell{runs: runs} }

// drawnCell carries content its own drawer renders once it is told its width.
// See cell for what a Layout can and cannot do to it afterwards, and fieldCell
// for the only thing that is one.
func drawnCell(draw func(width int) string) cell { return cell{draw: draw} }

// resolve draws a drawnCell now that its column is sized. Every other cell is
// already what it is.
func (c cell) resolve(width int) cell {
	if c.draw == nil {
		return c
	}
	return cell{rendered: c.draw(width)}
}

// width is what the cell wants. An unresolved drawnCell reports nothing, which
// is why one may not sit in a spendWidest column — there would be nothing to
// measure it by.
func (c cell) width() int {
	if c.runs == nil {
		return ansi.StringWidth(c.rendered)
	}
	w := 0
	for _, r := range c.runs {
		w += ansi.StringWidth(r.text)
	}
	return w
}

// paint renders a cell, truncated to width, with the selection fill carried
// through every run of it.
func (c cell) paint(width int, selected bool) string {
	if width < 1 {
		return ""
	}
	if c.runs == nil {
		// A Field's row is already styled and already the width it was given.
		// Truncating here is a backstop against a caller that measured it
		// against a different width than it rendered into.
		return ansi.Truncate(c.rendered, width, "…")
	}
	// The cut is of the whole cell, not of the run that happened to straddle the
	// edge, and the mark costs a column of its own. A name the query picked out
	// arrives as one run per rune — cutting those one at a time fills the column
	// exactly and leaves nothing to hang the mark on, so the same name would be
	// marked when it did not match and silently clipped when it did.
	room, cut := width, false
	if c.width() > width {
		room, cut = width-1, true
	}

	var b strings.Builder
	used := 0
	mark := lipglossPlain
	for _, r := range c.runs {
		if used >= room {
			break
		}
		text := r.text
		if used+ansi.StringWidth(text) > room {
			text = ansi.Truncate(text, room-used, "")
		}
		if text == "" {
			continue
		}
		b.WriteString(onSelected(r.style, selected).Render(text))
		used += ansi.StringWidth(text)
		// The mark wears what it replaced, so it reads as the end of the run it
		// cut rather than as a glyph of its own.
		mark = r.style
	}
	if cut {
		b.WriteString(onSelected(mark, selected).Render("…"))
	}
	return b.String()
}

// blockRow is one row's cells, and whether it wears the selection fill.
type blockRow struct {
	cells []cell
	// selected fills the row's whole width — every run and every pad. It is a
	// flag rather than a style because the fill is one colour: what varies is
	// the run underneath, which the caller styles. The delete confirm is the
	// proof — one danger-red run on the same bar every selected row wears.
	selected bool
}

// sized is an arrangement with its columns allocated across a width. The widths
// index into the arrangement they were measured for and mean nothing apart from
// it, so the three travel together rather than being kept in step by hand.
type sized struct {
	columns arrangement
	widths  []int
	width   int
}

// layout renders a block into a width, using the first candidate that fits. The
// last candidate is the fallback and is used whether it fits or not, so there is
// always a row to draw.
func layout(width int, candidates ...candidate) []string {
	if len(candidates) == 0 {
		return nil
	}
	// A candidate is drawn before it can be judged: a trailing column is sized
	// against the slack the flex column's own content leaves, so whether the
	// arrangement fits is not known until the Fields in it have been drawn.
	try := func(c candidate) (sized, []blockRow) {
		s := allocate(width, c)
		return s, s.draw(c.rows)
	}

	chosen, drawn := try(candidates[len(candidates)-1])
	for _, c := range candidates {
		if s, d := try(c); s.fits(d) {
			chosen, drawn = s, d
			break
		}
	}
	out := make([]string, 0, len(drawn))
	for _, row := range drawn {
		out = append(out, chosen.paint(row))
	}
	return out
}

// allocate gives every column of a candidate its width across the block.
func allocate(width int, c candidate) sized {
	a := c.columns
	sizedBy := c.sizedBy
	if sizedBy == nil {
		sizedBy = c.rows
	}

	widths := make([]int, len(a))
	spent, leads := 0, 0
	// Every arrangement has exactly one flex column — the value, the preview,
	// the query — and it is what the other columns leave.
	flex := -1
	for i, col := range a {
		if col.givenUp() {
			continue
		}
		switch col.spend {
		case spendFixed:
			widths[i] = col.n
			spent += col.n
		case spendWidest:
			w := 0
			for _, row := range sizedBy {
				if i < len(row.cells) {
					w = max(w, row.cells[i].width())
				}
			}
			if col.clampMin > 0 {
				w = max(w, col.clampMin)
			}
			if col.clampMax > 0 {
				w = min(w, col.clampMax)
			}
			widths[i] = w + col.pad
			// A trailing column is not reserved — that is what makes it
			// trailing — so it is measured but not charged for here.
			if col.align != alignTrailing {
				spent += widths[i]
			}
		case spendFlex:
			flex = i
			leads += col.lead
			continue
		}
		// A column that measured nothing costs nothing, its lead included. A
		// Library where no Command has been used and none asks for an argument
		// has no meta column at all, rather than an empty one with a gap sitting
		// in front of it.
		if widths[i] > 0 {
			leads += col.lead
		}
	}

	if flex >= 0 {
		widths[flex] = max(0, width-leads-spent)
	}
	return sized{columns: a, widths: widths, width: width}
}

// draw renders the cells that could not be drawn until their column was sized.
// Every other cell is already what it is.
func (s sized) draw(rows []blockRow) []blockRow {
	out := make([]blockRow, len(rows))
	for r, row := range rows {
		cells := make([]cell, len(row.cells))
		for i, c := range row.cells {
			if i < len(s.widths) {
				c = c.resolve(s.widths[i])
			}
			cells[i] = c
		}
		out[r] = blockRow{cells: cells, selected: row.selected}
	}
	return out
}

// fits reports whether every column got the least it can be drawn in. A
// candidate that does not fit is passed over for the next one.
func (s sized) fits(rows []blockRow) bool {
	for i, col := range s.columns {
		if col.givenUp() {
			continue
		}
		got := s.widths[i]
		if col.align == alignTrailing {
			// A trailing column lives in the slack its flex neighbour leaves,
			// measured against the fullest row in the block: it is the whole
			// row that has to fit, not the emptiest one in it.
			got = s.slack(rows) - s.widths[i]
		}
		if got < col.needs {
			return false
		}
	}
	return true
}

// slack is what the flex column's own content leaves unused, at its tightest
// down the block.
func (s sized) slack(rows []blockRow) int {
	out := -1
	for i, col := range s.columns {
		if col.spend != spendFlex {
			continue
		}
		for _, row := range rows {
			used := 0
			if i < len(row.cells) {
				used = row.cells[i].width()
			}
			if left := s.widths[i] - used; out < 0 || left < out {
				out = left
			}
		}
	}
	return max(0, out)
}

// paint lays one row out into the arrangement.
func (s sized) paint(row blockRow) string {
	width, a, widths := s.width, s.columns, s.widths
	fill := onSelected(lipglossPlain, row.selected)
	pad := func(n int) string {
		if n < 1 {
			return ""
		}
		return fill.Render(strings.Repeat(" ", n))
	}

	// A trailing column sits in the flex column's slack, so the flex column
	// stops at its content and the padding that would have filled out its width
	// goes in front of the trailing content instead.
	trailing := false
	for _, col := range a {
		if col.align == alignTrailing && !col.givenUp() {
			trailing = true
		}
	}

	var b strings.Builder
	used := 0
	for i, col := range a {
		// A column that took no width is not drawn and does not lead — see
		// allocate, which did not charge the row for it either.
		if col.givenUp() || (col.spend != spendFlex && widths[i] == 0) {
			continue
		}
		var c cell
		if i < len(row.cells) {
			c = row.cells[i]
		}
		// Every column is held inside what the row has left, padding as well as
		// content. A terminal can always be narrower than the least an
		// arrangement can be drawn in — the fallback candidate is drawn whether
		// it fits or not — and a row that answered by overflowing would wrap,
		// which costs the frame its last line rather than this row its last
		// column.
		lead := min(col.lead, max(0, width-used))
		b.WriteString(pad(lead))
		used += lead

		box := min(widths[i], max(0, width-used))
		room := box
		if col.spend == spendWidest {
			room = max(0, box-col.pad)
		}
		text := c.paint(room, row.selected)
		w := ansi.StringWidth(text)

		switch {
		case col.align == alignTrailing:
			b.WriteString(pad(max(0, width-used) - w))
			b.WriteString(text)
			used = width
		case col.align == alignRight:
			b.WriteString(pad(box - w))
			b.WriteString(text)
			used += box
		case col.spend == spendFlex && trailing:
			b.WriteString(text)
			used += w
		default:
			b.WriteString(text)
			b.WriteString(pad(box - w))
			used += box
		}
	}
	return b.String()
}
