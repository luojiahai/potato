package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// potato paints its caret from two places: bubbles draws the search field's
// inside textinput.View, and wrapStyledHard draws every other field's. The
// frame goldens are compared de-ANSI'd and cannot see either. These assert that
// the two are the same cell, and that the blink only changes what colour that
// cell is — never the row around it.

// The caret is one cell wherever it is. A field that painted its own would be a
// second caret with the same job, and the difference would only show up on
// someone else's terminal.
func TestTheFieldsPaintTheSearchFieldsCaret(t *testing.T) {
	input := newField()
	input.SetValue("x")
	input.Focus()
	input.SetCursor(0)

	// Rendered rather than hand-written: bubbles builds the cell from the
	// cursor colour it was given, and this asserts the fields build the same
	// bytes from the same colour, not merely something that looks close.
	want := caretStyle.Render("x")
	if got := input.View(); !strings.Contains(got, want) {
		t.Errorf("the search field paints %q where the fields paint %q — "+
			"the caret changes identity between screens", got, want)
	}
}

// The caret has to survive the trim every row goes through on the way out of
// View. A field's row ends at its value, so a caret parked past the last
// character is the last thing on the line, and the trim took it for padding —
// leaving the escape sequences with nothing between them, which draws nothing.
// The whole cell is matched here, not the colour that opens it: an empty run
// carries the colour too, which is why nothing else in the suite noticed.
func TestTheCaretSurvivesTheTrim(t *testing.T) {
	for name, keys := range map[string][]string{
		"a field being typed in": {"tab", "a", "deploy"},
		"an empty field":         {"tab", "a"},
		"an arg row":             {"enter"},
	} {
		m := New(fixtureDeps())
		m.SetSize(80, 24)
		press(m, keys)

		if !strings.Contains(m.View().Content, caretStyle.Render(" ")) {
			t.Errorf("%s: the caret's cell is empty — the frame carries the colour "+
				"but not the column it paints", name)
		}
	}
}

// The command field's hint does not step aside for the caret. It is text the
// caret can sit on, and a hint that moved a column to the right the moment the
// field took the keyboard would read as the field having been typed into.
func TestTheHintDoesNotStepAsideForTheCaret(t *testing.T) {
	const hint = "Type a command"
	column := func(keys []string) (int, string) {
		t.Helper()
		m := New(fixtureDeps())
		m.SetSize(80, 24)
		press(m, keys)
		for _, row := range strings.Split(render(t, m), "\n") {
			if i := strings.Index(row, hint); i >= 0 {
				return i, m.View().Content
			}
		}
		t.Fatalf("no hint row in the frame for %v", keys)
		return 0, ""
	}

	// `a` opens the add form with the name field holding the keyboard; two tabs
	// hand it to the command field, which is still empty and so still showing
	// the hint.
	blurred, _ := column([]string{"tab", "a"})
	focused, frame := column([]string{"tab", "a", "tab", "tab"})

	if blurred != focused {
		t.Errorf("the hint sits at column %d blurred and %d focused — it stepped aside for the caret",
			blurred, focused)
	}
	if !strings.Contains(frame, caretStyle.Render(string([]rune(hint)[0]))) {
		t.Error("the caret is not on the hint's first character")
	}
}

// The same thing again against whole frames, which is where it would be felt:
// every screen that draws its own caret has to lay out identically on both
// halves of the blink. A row that reflowed at 530ms would be a screen that
// twitched while you were reading it.
//
// Compared with each row's trailing space dropped. A caret parked at the end of
// a row keeps its cell only while it is lit — nothing follows it there, so the
// cell is the row's last column either way and lighting it moves nothing. What
// this is looking for is a row whose *content* sits somewhere else.
func TestTheBlinkDoesNotMoveTheFrame(t *testing.T) {
	for name, keys := range map[string][]string{
		"args":         {"enter", "prod-9"},
		"edit":         {"tab", "a", "deploy"},
		"edit-command": {"tab", "a", "n", "tab", "tab", "git push {{branch}}"},
		"edit-empty":   {"tab", "a", "n", "tab", "tab"},
	} {
		m := New(fixtureDeps())
		m.SetSize(80, 24)
		press(m, keys)

		caret := strings.SplitN(caretStyle.Render("x"), "x", 2)[0]
		lit := columns(render(t, m))
		if got := strings.Count(m.View().Content, caret); got != 1 {
			t.Errorf("%s: the lit half carries %d carets, want exactly 1", name, got)
		}

		m.caret.IsBlinked = true
		dark := columns(render(t, m))

		if lit != dark {
			t.Errorf("%s: the frame moved between the halves of the blink\n--- lit ---\n%s\n--- dark ---\n%s",
				name, lit, dark)
		}
		if strings.Contains(m.View().Content, caret) {
			t.Errorf("%s: the dark half still paints a caret", name)
		}
	}
}

// The dark half of the blink gives up the caret's colour, not its column.
// Handing the cell back would shorten the row twice a second, and an arg row
// measures its fill and its hint against that width — the row would step
// sideways in time with the blink.
func TestTheBlinkDoesNotMoveTheRow(t *testing.T) {
	runs := []run{{text: "abc", style: textStyle}}
	caret := strings.SplitN(caretStyle.Render("x"), "x", 2)[0]

	for name, at := range map[string]int{"mid-value": 1, "past the last rune": 3} {
		lit, litRow := wrapStyledHard(runs, 10, at, true)
		dark, darkRow := wrapStyledHard(runs, 10, at, false)

		if len(lit) != len(dark) || litRow != darkRow {
			t.Errorf("%s: the blink moved the caret from row %d of %d to row %d of %d",
				name, litRow, len(lit), darkRow, len(dark))
			continue
		}
		for i := range lit {
			if ansi.StringWidth(lit[i]) != ansi.StringWidth(dark[i]) {
				t.Errorf("%s: row %d is %d columns lit and %d dark",
					name, i, ansi.StringWidth(lit[i]), ansi.StringWidth(dark[i]))
			}
		}
		if !strings.Contains(strings.Join(lit, ""), caret) {
			t.Errorf("%s: the lit half draws no caret", name)
		}
		if strings.Contains(strings.Join(dark, ""), caret) {
			t.Errorf("%s: the dark half still draws the caret", name)
		}
	}
}
