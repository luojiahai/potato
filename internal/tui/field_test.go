package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// A Field's rules — where the caret clamps, which way the value moves when it
// outgrows its room, what an empty one shows — used to be observable only
// through a whole 80×24 frame, and the frame goldens are compared de-ANSI'd and
// cannot see a caret at all. These reach the Field directly.

// A one-line Field slides its value under the caret rather than growing, so the
// caret is on screen whatever the value's length.
func TestALineFieldKeepsTheCaretOnScreen(t *testing.T) {
	f := newField(fieldLine)
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
	f := newField(fieldWrap)
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
	f := newField(fieldLine)
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
	f := newField(fieldWrap)
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

// The search field is a Field like any other, which is what gives it the
// windowing it never had: it used to render its whole value and let the frame's
// clamp cut the overflow, and the caret went with it.
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
