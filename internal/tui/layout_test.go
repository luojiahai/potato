package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The Layout's own tests. The golden frames prove a whole frame holds together
// at four terminal sizes; these prove the rule underneath it at every width, and
// prove the one thing a golden physically cannot see — the goldens are de-ANSI'd,
// so a selection bar full of holes strips to exactly the same text as one
// without any.

// listCells is one list row's cells, sized so its columns give up in a spread of
// widths worth testing between.
func listCells(selected bool) []cell {
	pointer := "  "
	if selected {
		pointer = "❯ "
	}
	cells := make([]cell, 4)
	cells[colPointer] = textCell(pointer, accentStyle)
	cells[colName] = runsCell([]run{{text: "deploy prod", style: titleStyle()}})
	cells[colPreview] = textCell("ssh prod-1 'deploy.sh' --verbose", dimStyle)
	cells[colMeta] = runsCell([]run{{text: "⌁1 · 2h ago", style: dimStyle}})
	return cells
}

// listRow goes through listCandidates rather than assembling its own, so the
// order these tests pin is the order the list screen actually gives up in.
func listRow(width int, selected bool) string {
	rows := []blockRow{{cells: listCells(selected), selected: selected}}
	return layout(width, listCandidates(rows, rows)...)[0]
}

// TestTheListRowGivesUpItsColumnsInOrder walks the width down and watches what
// goes. The preview is the first to go and the meta the second — a name cut to
// `depl…` costs more than either — and the name is never the one that gives.
func TestTheListRowGivesUpItsColumnsInOrder(t *testing.T) {
	for _, tc := range []struct {
		width         int
		preview, meta bool
	}{
		{width: 80, preview: true, meta: true},
		{width: 53, preview: true, meta: true},
		{width: 52, preview: false, meta: true},
		{width: 23, preview: false, meta: true},
		{width: 22, preview: false, meta: false},
		{width: 12, preview: false, meta: false},
	} {
		row := ansi.Strip(listRow(tc.width, false))
		if got := strings.Contains(row, "deploy.sh"); got != tc.preview {
			t.Errorf("width %d: preview shown = %v, want %v\n%q", tc.width, got, tc.preview, row)
		}
		if got := strings.Contains(row, "2h ago"); got != tc.meta {
			t.Errorf("width %d: meta shown = %v, want %v\n%q", tc.width, got, tc.meta, row)
		}
		// Whatever it gave up, the name survives and the row still fits.
		if !strings.Contains(row, "deploy") {
			t.Errorf("width %d: lost the name\n%q", tc.width, row)
		}
		if w := ansi.StringWidth(row); w > tc.width {
			t.Errorf("width %d: row is %d columns wide\n%q", tc.width, w, row)
		}
	}
}

// TestTheNameStopsBeingHeldInItsBandWhenThePreviewGoes is the reason these are
// candidate arrangements rather than one arrangement with columns dropped out
// of it: losing the preview does not merely free the name's neighbour, it
// changes how the name spends its width.
func TestTheNameStopsBeingHeldInItsBandWhenThePreviewGoes(t *testing.T) {
	rows := []blockRow{{cells: listCells(true), selected: true}}
	withPreview, _, _ := measure(80, candidate{columns: listColumns(true, true), rows: rows})
	without, _, _ := measure(80, candidate{columns: listColumns(false, true), rows: rows})

	// Beside a preview the name takes what it measures, held inside the band —
	// this name is shorter than the band's floor, so it takes the floor.
	if want := max(ansi.StringWidth("deploy prod"), 12); withPreview[colName] != want {
		t.Errorf("beside a preview the name should be %d, got %d", want, withPreview[colName])
	}
	if without[colName] <= withPreview[colName] {
		t.Errorf("with no preview the name should take the remainder, got %d (was %d)",
			without[colName], withPreview[colName])
	}
}

// TestAColumnThatMeasuredNothingCostsNothing — a Library where no Command has
// been used and none asks for an argument has no meta column at all, rather
// than an empty one with a gap sitting in front of it. The preview gets both
// back.
func TestAColumnThatMeasuredNothingCostsNothing(t *testing.T) {
	bare := listCells(false)
	bare[colMeta] = runsCell(nil)

	columns := listColumns(true, true)
	without, _, _ := measure(80, candidate{columns: columns, rows: []blockRow{{cells: bare}}})
	with, _, _ := measure(80, candidate{columns: columns, rows: []blockRow{{cells: listCells(false)}}})

	got := without[colPreview] - with[colPreview]
	want := ansi.StringWidth("⌁1 · 2h ago") + 2 // the meta and the gap in front of it
	if got != want {
		t.Errorf("an absent meta should give the preview back %d columns, got %d", want, got)
	}
	if strings.Contains(ansi.Strip(listRow(80, false)), "  \n") {
		t.Error("the row should not end on the gap of a column that is not there")
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
		got := ansi.Strip(runsCell(tc.runs).paint(12, false))
		if w := ansi.StringWidth(got); w != 12 {
			t.Errorf("%s: cut to %d columns, want 12: %q", tc.label, w, got)
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("%s: the cut should be marked, got %q", tc.label, got)
		}
	}
}

// surfaceFill is the SGR parameters lipgloss emits for the selection bar's
// background. Taken from lipgloss rather than written out: the colour is
// potato's, its encoding is not.
var surfaceFill = func() string {
	probe := lipgloss.NewStyle().Background(lipgloss.Color(surfaceColor)).Render("x")
	for _, tok := range tokenize(probe) {
		if params, ok := sgrParams(tok); ok {
			return strings.Join(params, ";")
		}
	}
	return ""
}()

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
	filled, reversed, holes := false, false, 0
	for _, tok := range tokenize(rendered) {
		if params, ok := sgrParams(tok); ok {
			joined := strings.Join(params, ";")
			if strings.Contains(joined, surfaceFill) {
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
		row := listRow(width, true)
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

// TestATrailingColumnSitsInTheSlackRatherThanReservingIt is the search row's
// policy, and the reason alignTrailing exists apart from alignRight: the query
// is given the whole row so a long one slides under its own caret, and the
// count is what gives.
func TestATrailingColumnSitsInTheSlackRatherThanReservingIt(t *testing.T) {
	row := func(query, count string) string {
		return layout(40, candidate{
			columns: searchColumns,
			rows: []blockRow{{cells: []cell{
				textCell("⌕ ", focusStyle()),
				fieldCell(func(w int) string { return ansi.Truncate(query, w, "") }),
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

// TestTheLastCandidateIsDrawnEvenWhenNothingFits — a terminal can always be
// narrower than the least a row can be laid out in, and there is still a frame
// to draw.
func TestTheLastCandidateIsDrawnEvenWhenNothingFits(t *testing.T) {
	for _, width := range []int{6, 3, 1, 0, -1} {
		row := listRow(width, false)
		if w := ansi.StringWidth(row); w > max(0, width) {
			t.Errorf("width %d: row is %d columns wide\n%q", width, w, row)
		}
	}
}
