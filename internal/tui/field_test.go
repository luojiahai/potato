package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// A Field's rules — which way the value moves when it outgrows its room, which
// row the caret lands on, what an empty one shows — are reached directly here
// rather than through a whole 80×24 frame, which the goldens compare de-ANSI'd
// and so cannot see a caret in at all.
//
// Not covered, because it cannot be reached: the caret clamp in Rows and
// windowValue. bubbles re-clamps the cursor into the value on every SetValue
// and SetCursor, so Position never exceeds the rune count. The clamp guards an
// assumption about the model inside the Field, not a path a test can take.

// A one-line Field slides its value under the caret rather than growing, so the
// caret is on screen whatever the value's length.
func TestALineFieldKeepsTheCaretOnScreen(t *testing.T) {
	f := newField(lineMode)
	f.SetValue(strings.Repeat("x", 40))
	f.Focus()

	rows, _ := f.Rows(10, true)
	if len(rows) != 1 {
		t.Fatalf("a one-line Field rendered %d rows", len(rows))
	}
	if w := ansi.StringWidth(rows[0]); w > 10 {
		t.Errorf("the row is %d columns, want no more than the 10 it was given", w)
	}
	if !strings.Contains(rows[0], caretStyle.Render(" ")) {
		t.Error("the caret's cell is not in the row — the value slid out from under it")
	}
}

// A wrapping Field reports which row the caret landed on, which is what lets a
// caller with less height than the Field has rows window around it rather than
// cut the row being typed in.
func TestAWrapFieldReportsTheCaretsRow(t *testing.T) {
	f := newField(wrapMode)
	f.SetValue(strings.Repeat("x", 25))
	f.Focus()

	rows, caretRow := f.Rows(10, true)
	if len(rows) != 3 {
		t.Fatalf("25 runes at width 10 wrapped to %d rows, want 3", len(rows))
	}
	if caretRow != 2 {
		t.Errorf("the caret is reported on row %d, want the last one it is actually on", caretRow)
	}
}

// Update reports an edit, not a keystroke. The list screen resets its selection
// on the first and must not on the second — a query walked with ^A and ^E has
// not changed, and the row you had picked should still be picked.
func TestUpdateReportsAnEditRatherThanAKeystroke(t *testing.T) {
	f := newField(lineMode)
	f.SetValue("ports")
	f.Focus()

	if _, edited := f.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); !edited {
		t.Error("typing a character did not report an edit")
	}
	if _, edited := f.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}); edited {
		t.Error("^A reported an edit — the caret moved, the value did not")
	}
}

// The hint is text the caret sits on, not text it sits in front of. Given a
// cell of its own the hint would step a column right the moment the Field took
// the keyboard, which reads as the Field having been typed into.
func TestTheHintTakesTheCaretRatherThanACellInFrontOfIt(t *testing.T) {
	f := newField(wrapMode)
	f.hint = "Type a command"

	blurred, _ := f.Rows(40, true)
	f.Focus()
	focused, _ := f.Rows(40, true)

	if w, w2 := ansi.StringWidth(blurred[0]), ansi.StringWidth(focused[0]); w != w2 {
		t.Errorf("the hint is %d columns blurred and %d focused — it stepped aside for the caret", w, w2)
	}
	if !strings.Contains(focused[0], caretStyle.Render("T")) {
		t.Error("the caret is not on the hint's first character")
	}
}

// A Field windows by column, not by rune. Counting runes holds only while every
// rune is one column wide: CJK and emoji windowed to a rune count overflow the
// row, which wraps — and a one-line Field that returns two rows loses the caret
// with the row its caller drops.
func TestALineFieldWindowsWideRunesByColumn(t *testing.T) {
	for name, value := range map[string]string{
		"CJK":   strings.Repeat("北", 40),
		"emoji": strings.Repeat("🥔", 40),
		"mixed": strings.Repeat("a北", 20),
	} {
		f := newField(lineMode)
		f.SetValue(value)
		f.Focus()

		rows, caretRow := f.Rows(10, true)
		if len(rows) != 1 || caretRow != 0 {
			t.Errorf("%s: a one-line Field returned %d rows with the caret on row %d — the caller keeps only the first",
				name, len(rows), caretRow)
			continue
		}
		if w := ansi.StringWidth(rows[0]); w > 10 {
			t.Errorf("%s: the row is %d columns, want no more than the 10 it was given", name, w)
		}
		if !strings.Contains(rows[0], caretStyle.Render(" ")) {
			t.Errorf("%s: the caret's cell is not in the row", name)
		}
	}
}

// The same thing where it would be felt, against a whole frame.
func TestAWideRuneQueryKeepsItsCaretInTheFrame(t *testing.T) {
	m := New(fixtureDeps())
	m.SetSize(40, 24)
	press(m, []string{strings.Repeat("北", 30)})

	if !strings.Contains(m.View().Content, caretStyle.Render(" ")) {
		t.Error("the caret's cell is gone — a CJK query overflowed the row it was windowed into")
	}
}

// The search field is a Field like any other, so it windows its value like any
// other: a query longer than the row slides under its own caret rather than
// being rendered whole for the frame's clamp to cut, which would take the caret
// with it.
func TestALongQueryKeepsItsCaretInTheFrame(t *testing.T) {
	m := New(fixtureDeps())
	m.SetSize(40, 24)
	press(m, []string{strings.Repeat("abcdefghij", 6)}) // 60 runes into a 40-column frame

	// The whole cell, not the colour that opens it: a truncated row keeps the
	// escape sequence and drops the column it paints.
	if !strings.Contains(m.View().Content, caretStyle.Render(" ")) {
		t.Error("the caret's cell is gone — a query longer than the row took it off the edge")
	}
}
