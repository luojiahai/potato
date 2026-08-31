package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/luojiahai/potato/internal/library"
)

// The Layout's own tests. The golden frames prove a whole frame holds together
// at four terminal sizes; these prove the rule underneath it at every width, and
// prove the one thing a golden physically cannot see — the goldens are de-ANSI'd,
// so a selection bar full of holes strips to exactly the same text as one
// without any.
//
// Everything here goes in through layout, or through the screen that calls it.
// Nothing reaches past that into how a candidate is fitted: which arrangement
// won is read off the row, which is where a user reads it too.

// ---------- the list row ----------

const (
	listName    = "deploy prod"                      // 11 columns
	listPreview = "ssh prod-1 'deploy.sh' --verbose" // 31
	listMeta    = "⌁1 · 2h ago"                      // 11
	// listLongName is longer than the band the name is held in beside a
	// preview, and shorter than the remainder it takes when there is none.
	listLongName = "deployment-pipeline-staging-eu-west-2-canary" // 44
)

// listCells is one list row's cells, sized so its columns give up in a spread of
// widths worth testing between. The pointer and the column order come from the
// list screen itself, so a change to either shows up here rather than leaving
// these green against a shape the screen stopped drawing.
func listCells() []cell {
	cells := make([]cell, 4)
	cells[colPointer] = pointerCell(false)
	cells[colName] = runsCell([]run{{text: listName, style: titleStyle()}})
	cells[colPreview] = textCell(listPreview, dimStyle)
	cells[colMeta] = runsCell([]run{{text: listMeta, style: dimStyle}})
	return cells
}

// listRow goes through listCandidates rather than assembling its own, so the
// order these tests pin is the order the list screen actually gives up in.
func listRow(width int, cells []cell, selected bool) string {
	rows := []blockRow{{cells: cells, selected: selected}}
	return layout(width, listCandidates(rows, rows)...)[0]
}

// Where the list row's arrangements hand over. Written as numbers with the
// arithmetic beside them: a change to a column's policy should land here as a
// number that moved, not as a rule the test had to work out for itself.
const (
	// pointer 2 + name 12 + meta 11, and two leads of 2, leave the preview
	// width - 29 — and a preview needs 24.
	listPreviewGoesBelow = 53
	// pointer 2 + meta 11, and one lead of 2, leave the name width - 15 — and
	// a name needs contentFloor.
	listMetaGoesBelow = 23
)

// TestTheListRowGivesUpItsColumnsInOrder walks every width the row is ever
// drawn at, rather than the three the golden frames sample. The preview is the
// first to go and the meta the second — a name cut to `depl…` costs more than
// either — and the name is never the one that gives.
func TestTheListRowGivesUpItsColumnsInOrder(t *testing.T) {
	for width := 1; width <= 100; width++ {
		row := ansi.Strip(listRow(width, listCells(), false))

		if got, want := strings.Contains(row, "deploy.sh"), width >= listPreviewGoesBelow; got != want {
			t.Errorf("width %d: preview shown = %v, want %v\n%q", width, got, want, row)
		}
		if got, want := strings.Contains(row, "2h ago"), width >= listMetaGoesBelow; got != want {
			t.Errorf("width %d: meta shown = %v, want %v\n%q", width, got, want, row)
		}
		if w := ansi.StringWidth(row); w > width {
			t.Errorf("width %d: row is %d columns wide\n%q", width, w, row)
		}
		// Whatever it gave up, the name is still what the row is spending its
		// columns on. Below the floor there is nothing left to promise.
		if width >= contentFloor+4 && !strings.Contains(row, "deploy") {
			t.Errorf("width %d: lost the name\n%q", width, row)
		}
	}
}

// TestTheNameStopsBeingHeldInItsBandWhenThePreviewGoes is the reason these are
// candidate arrangements rather than one arrangement with columns dropped out
// of it: losing the preview does not merely free the name's neighbour, it
// changes how the name spends its width. A name that will not fit at ninety
// columns fits at eighty.
func TestTheNameStopsBeingHeldInItsBandWhenThePreviewGoes(t *testing.T) {
	row := func(width int) string {
		cells := listCells()
		cells[colName] = runsCell([]run{{text: listLongName, style: titleStyle()}})
		return ansi.Strip(listRow(width, cells, false))
	}

	// Room for a preview, so the name is held inside its band — forty columns,
	// which this name does not fit in.
	wide := row(90)
	if !strings.Contains(wide, "deploy.sh") {
		t.Fatalf("ninety columns should still carry a preview\n%q", wide)
	}
	if strings.Contains(wide, listLongName) {
		t.Errorf("beside a preview the name should be held in its band\n%q", wide)
	}

	// Ten columns narrower there is no room for a preview, and the name takes
	// the whole remainder rather than the band it was clamped to.
	narrow := row(80)
	if strings.Contains(narrow, "deploy.sh") {
		t.Fatalf("eighty columns should have given the preview up\n%q", narrow)
	}
	if !strings.Contains(narrow, listLongName) {
		t.Errorf("with no preview the name should take the remainder\n%q", narrow)
	}
}

// TestAColumnThatMeasuredNothingCostsNothing — a Library where no Command has
// been used and none asks for an argument has no meta column at all, rather
// than an empty one with a gap sitting in front of it. The preview gets both
// back, which can be read off a preview long enough to be cut either way.
func TestAColumnThatMeasuredNothingCostsNothing(t *testing.T) {
	row := func(meta []run) string {
		cells := listCells()
		// Longer than either allocation, so both are cut and what each was
		// given can be counted off the row.
		cells[colPreview] = textCell(strings.Repeat("x", 80), dimStyle)
		cells[colMeta] = runsCell(meta)
		return ansi.Strip(listRow(80, cells, false))
	}

	with := row([]run{{text: listMeta, style: dimStyle}})
	without := row(nil)

	got := strings.Count(without, "x") - strings.Count(with, "x")
	want := ansi.StringWidth(listMeta) + 2 // the meta and the gap in front of it
	if got != want {
		t.Errorf("an absent meta should give the preview back %d columns, got %d", want, got)
	}
	if !strings.HasSuffix(without, "…") {
		t.Errorf("the row should not end on the gap of a column that is not there\n%q", without)
	}
}

// TestAnOverlongCellIsMarkedHoweverManyRunsItIsIn — a name the query picked out
// is one run per rune, and a cell cut run by run would fill its column exactly
// and never mark that anything was missing. The cut is of the whole cell.
func TestAnOverlongCellIsMarkedHoweverManyRunsItIsIn(t *testing.T) {
	name := "deployment-pipeline-staging"
	matched := nameRuns("dps", name)
	if len(matched) < 2 {
		t.Fatalf("the fixture query should split the name into runs, got %d", len(matched))
	}
	for _, tc := range []struct {
		label string
		runs  []run
	}{
		{"one run", []run{{text: name, style: titleStyle()}}},
		{"one run per rune", matched},
	} {
		got := ansi.Strip(layout(12, candidate{
			columns: arrangement{{spend: spendFlex}},
			rows:    []blockRow{{cells: []cell{runsCell(tc.runs)}}},
		})[0])
		if w := ansi.StringWidth(got); w != 12 {
			t.Errorf("%s: cut to %d columns, want 12: %q", tc.label, w, got)
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("%s: the cut should be marked, got %q", tc.label, got)
		}
	}
}

// ---------- the arg row ----------

const (
	argTemplate = "ssh {{host=prod-1}} {{env}}"
	argNoteText = "(Default: prod-1)" // 17 columns
	// indent 2 + gutter 6 (`host` and the two after it) + note 17, and one lead
	// of 2, leave the value width - 27 — and a value needs contentFloor.
	argNoteGoesBelow = 35
)

// argScreen builds an arg screen through the screen's own constructor, so the
// Fields are the ones potato actually types into — including the painter that
// carries the selection fill across a value the Layout cannot fill itself.
func argScreen(t *testing.T, template string) *argsScreen {
	t.Helper()
	m := New(fixtureDeps())
	m.SetSize(80, 24)
	return newArgsScreen(m, &library.Command{
		ID: "layout-test", Name: "fixture", Template: template,
	})
}

// TestTheArgRowGivesUpItsNoteBeforeItsValue — the note is where a value came
// from, and the value is what you are typing.
func TestTheArgRowGivesUpItsNoteBeforeItsValue(t *testing.T) {
	s := argScreen(t, argTemplate)
	for width := 1; width <= 100; width++ {
		rows := s.rows(width, true)
		if len(rows) != 2 {
			t.Fatalf("width %d: %d rows, want one per Placeholder", width, len(rows))
		}
		for i, row := range rows {
			if w := ansi.StringWidth(row); w > width {
				t.Errorf("width %d: row %d is %d columns wide\n%q", width, i, w, ansi.Strip(row))
			}
		}

		noted := ansi.Strip(rows[0])
		if got, want := strings.Contains(noted, argNoteText), width >= argNoteGoesBelow; got != want {
			t.Errorf("width %d: note shown = %v, want %v\n%q", width, got, want, noted)
		}
		// A note that is shown ends flush right, which is what holds the notes
		// in one column down the panel.
		if width >= argNoteGoesBelow && !strings.HasSuffix(noted, argNoteText) {
			t.Errorf("width %d: the note should end flush right\n%q", width, noted)
		}
	}
}

// TestTheNotesAreGivenUpAsABlock pins how the arg screen gives its notes up.
// The notes are one column, so they go together: a Placeholder with a short
// note does not keep it on a panel too narrow for its neighbour's long one.
func TestTheNotesAreGivenUpAsABlock(t *testing.T) {
	s := argScreen(t, "ssh {{host=prod-1}} {{env=x}}")

	// Wide enough to have carried the short note on its own, too narrow for the
	// long one — so both go.
	for i, row := range s.rows(argNoteGoesBelow-1, true) {
		if plain := ansi.Strip(row); strings.Contains(plain, "(Default:") {
			t.Errorf("row %d kept its note after the block gave them up\n%q", i, plain)
		}
	}

	// And they come back together.
	rows := s.rows(argNoteGoesBelow, true)
	for i, want := range []string{argNoteText, "(Default: x)"} {
		if plain := ansi.Strip(rows[i]); !strings.HasSuffix(plain, want) {
			t.Errorf("row %d should carry %q\n%q", i, want, plain)
		}
	}
}

// ---------- the search row ----------

// TestTheSearchRowShortensItsCountBeforeDroppingIt is the third degradation the
// Layout carries, and the one that is not a drop: the count gives up its sort
// order before it gives up its column.
func TestTheSearchRowShortensItsCountBeforeDroppingIt(t *testing.T) {
	m := New(fixtureDeps())
	m.SetSize(80, 24)
	s := newListScreen()
	results := s.results(m)

	short := fmt.Sprintf("%d/%d", len(results), len(m.lib.Commands))
	long := short + " · Recently used"
	// The count takes what the query leaves: the glyph costs two columns and an
	// empty field's caret one, and the query keeps at least one for itself.
	goesBelow := func(form string) int { return ansi.StringWidth(form) + 4 }

	for width := 1; width <= 60; width++ {
		row := ansi.Strip(s.searchRow(m, results, width))
		switch {
		case width >= goesBelow(long):
			if !strings.HasSuffix(row, long) {
				t.Errorf("width %d: want the count and its sort order\n%q", width, row)
			}
		case width >= goesBelow(short):
			if !strings.HasSuffix(row, short) || strings.Contains(row, "Recently") {
				t.Errorf("width %d: want the count alone\n%q", width, row)
			}
		default:
			if strings.Contains(row, short) {
				t.Errorf("width %d: the count should have given way to the query\n%q", width, row)
			}
		}
		if w := ansi.StringWidth(row); w > width {
			t.Errorf("width %d: row is %d columns wide\n%q", width, w, row)
		}
	}
}

// TestATrailingColumnSitsInTheSlackRatherThanReservingIt is the policy under
// that, and the reason alignTrailing exists apart from alignRight: the query is
// given the whole row so a long one slides under its own caret, and the count
// is what gives.
func TestATrailingColumnSitsInTheSlackRatherThanReservingIt(t *testing.T) {
	row := func(query, count string) string {
		return layout(40, candidate{
			columns: searchColumns,
			rows: []blockRow{{cells: []cell{
				textCell("⌕ ", focusStyle()),
				drawnCell(func(w int) string { return ansi.Truncate(query, w, "") }),
				textCell(count, dimStyle),
			}}},
		})[0]
	}

	// A short query leaves room, so the count rides the far end of the row.
	short := ansi.Strip(row("api", "3/9"))
	if !strings.HasSuffix(short, "3/9") {
		t.Errorf("the count should be flush right\n%q", short)
	}
	if w := ansi.StringWidth(short); w != 40 {
		t.Errorf("the row should reach the full width, got %d\n%q", w, short)
	}

	// A query long enough to fill the row leaves no slack, and the count is not
	// squeezed in beside it — the flex column was never sized around it.
	long := ansi.Strip(row(strings.Repeat("x", 60), "3/9"))
	if strings.Contains(long, "3/9") {
		t.Errorf("the count should have given way to the query\n%q", long)
	}
	if w := ansi.StringWidth(long); w > 40 {
		t.Errorf("the row overflowed to %d columns\n%q", w, long)
	}
}

// ---------- the fill ----------

// surfaceFill is the SGR parameters lipgloss emits for the selection bar's
// background. Taken from lipgloss rather than written out: the colour is
// potato's, its encoding is not. Probed on every call, because which palette
// potato paints in is settled by the terminal's answer rather than at package
// initialisation, and a fill cached before that describes no colour at all.
func surfaceFill() string {
	probe := lipgloss.NewStyle().Background(lipgloss.Color(surfaceColor)).Render("x")
	for _, tok := range tokenize(probe) {
		if params, ok := sgrParams(tok); ok {
			return strings.Join(params, ";")
		}
	}
	return ""
}

// barHoles counts the columns of a rendered row that are not wearing the
// selection fill.
//
// This is the one thing the golden frames cannot check. lipgloss cannot cascade
// a background over an escape sequence that has already been emitted, so the
// fill has to go on each run as it is built — and a run that skips it leaves a
// gap in the bar that strips to exactly the same text as one that did not.
//
// A reverse-video cell counts as painted. It is a caret, which is a painted
// cell in its own right rather than a hole: the block is drawn, it is simply
// drawn in the accent rather than the surface.
func barHoles(rendered string) int {
	fill := surfaceFill()
	filled, reversed, holes := false, false, 0
	for _, tok := range tokenize(rendered) {
		if params, ok := sgrParams(tok); ok {
			joined := strings.Join(params, ";")
			if strings.Contains(joined, fill) {
				filled = true
			}
			for _, p := range params {
				switch p {
				case "7":
					reversed = true
				case "0", "":
					filled, reversed = false, false
				}
			}
			continue
		}
		if strings.HasPrefix(tok, "\x1b") {
			continue
		}
		if !filled && !reversed {
			holes++
		}
	}
	return holes
}

// TestTheSelectionBarHasNoHoles walks a selected row column by column. Every
// cell the Layout renders and every pad it adds has to carry the fill — the
// pointer, the name, the gap before the preview, the preview, the padding after
// it, the gap before the meta, and the meta.
func TestTheSelectionBarHasNoHoles(t *testing.T) {
	for _, width := range []int{80, 60, 52, 51, 40, 26, 25, 20} {
		row := listRow(width, listCells(), true)
		if holes := barHoles(row); holes > 0 {
			t.Errorf("width %d: %d columns of the bar are unfilled\n%q", width, holes, row)
		}
	}
}

// TestTheFocusedArgRowWearsTheSameBar is the one row with a cell the Layout
// hands over already rendered. A pre-rendered cell cannot be filled after the
// fact by anyone — a background does not cascade over escape sequences that are
// already written — so the value is filled where its sequences are written,
// inside the Field, by the painter the arg screen gives it. This is the test
// that the two halves meet.
func TestTheFocusedArgRowWearsTheSameBar(t *testing.T) {
	s := argScreen(t, argTemplate)
	for _, width := range []int{80, 60, 40, argNoteGoesBelow, argNoteGoesBelow - 1, 20, 12} {
		row := s.rows(width, true)[0] // the Form starts focused on the first Field
		if holes := barHoles(row); holes > 0 {
			t.Errorf("width %d: %d columns of the bar are unfilled\n%q", width, holes, row)
		}
	}
}

// TestTheConfirmWearsTheSameBar — the delete confirm is one danger-red run
// across the whole row, and it is filled the same way every selected row is
// rather than painting a bar of its own.
func TestTheConfirmWearsTheSameBar(t *testing.T) {
	row := confirmRow(60, "deploy prod")
	if holes := barHoles(row); holes > 0 {
		t.Errorf("%d columns of the confirm's bar are unfilled\n%q", holes, row)
	}
	if !strings.Contains(ansi.Strip(row), "Delete 'deploy prod'?") {
		t.Errorf("the confirm should name the Command\n%q", ansi.Strip(row))
	}
}

// TestTheLastCandidateIsDrawnEvenWhenNothingFits — a terminal can always be
// narrower than the least a row can be laid out in, and there is still a frame
// to draw.
func TestTheLastCandidateIsDrawnEvenWhenNothingFits(t *testing.T) {
	for _, width := range []int{6, 3, 1, 0, -1} {
		row := listRow(width, listCells(), false)
		if w := ansi.StringWidth(row); w > max(0, width) {
			t.Errorf("width %d: row is %d columns wide\n%q", width, w, row)
		}
	}
}
