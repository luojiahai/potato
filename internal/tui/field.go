// A Field is one editable value on screen: its text, its caret, and how it
// renders into a width. The search field, the three add/edit fields, and one
// per Placeholder on the arg screen are all Fields.
//
// Potato paints every caret. It used to paint all but one: the search field
// drew its own from inside bubbles' textinput, and the rest went through
// wrapStyledHard, which left two implementations of one cell to keep in step
// and a test whose whole job was to prove they had not drifted. bubbles keeps
// the value, the cursor position and the readline key set — ^A, ^E, ^K, ^U, ^W
// — which is the part worth having; what it draws with them is potato's.
//
// That is also what makes the blink one clock. The search field used to blink
// on the one inside its textinput, which bubbles keeps unexported, so the other
// fields could not borrow it and ran a copy instead. Now nothing reads it: `on`
// arrives from the Model's clock, and every caret in the frame is lit or dark
// together.

package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/luojiahai/potato/internal/placeholders"
)

// fieldMode is how a Field spends the width it is given.
type fieldMode int

const (
	// fieldWrap folds the value at the width and grows downward — the add/edit
	// fields, where the command being typed is the tallest thing on screen.
	fieldWrap fieldMode = iota
	// fieldLine keeps the value on one row and slides it under the caret — the
	// search field and the arg rows, which sit in rows they cannot grow out of.
	fieldLine
)

// paint maps a value to the runs it renders as, given whether the Field has the
// keyboard. It is the one hook for what a Field looks like: the command field
// picks its Placeholders out in gold, an arg row carries the selection fill
// across its value, and everything else is plain command text.
type paint func(value string, focused bool) []run

func plainPaint(value string, _ bool) []run {
	return []run{{text: value, style: textStyle}}
}

// segmentRuns picks a template's Placeholders out in gold. Three renderers want
// it — the command field, the arg screen's preview, and the list's detail block
// — which is why it is a function rather than a loop written out three times.
func segmentRuns(segs []placeholders.Segment) []run {
	runs := make([]run, 0, len(segs))
	for _, seg := range segs {
		style := textStyle
		if seg.Flag {
			style = highlightStyle.Bold(true)
		}
		runs = append(runs, run{text: seg.Text, style: style})
	}
	return runs
}

// templateRuns keeps the `{{name}}` as written — for the surfaces that show a
// Command's template, including the field it is typed into.
func templateRuns(template string) []run {
	return segmentRuns(placeholders.TemplateSegments(template))
}

// renderRuns substitutes the values in and flags what was filled — for the arg
// screen's preview, where the gold marks what the user just decided rather than
// where a Placeholder used to be.
func renderRuns(template string, values map[string]string) []run {
	return segmentRuns(placeholders.RenderSegments(template, values))
}

type field struct {
	input textinput.Model
	mode  fieldMode
	// paint is how the value looks; nil is plain command text.
	paint paint
	// hint is what an empty Field shows in its place, and "" is nothing. It is
	// text the caret sits on rather than a row of its own — see hintRows.
	hint string
}

// newField builds a Field. The bubbles model inside it is stripped to the parts
// potato reads: no prompt, and none of the styling it would need to draw
// itself, because it never does.
func newField(mode fieldMode) field {
	input := textinput.New()
	input.Prompt = ""
	return field{input: input, mode: mode}
}

func (f *field) Value() string     { return f.input.Value() }
func (f *field) SetValue(s string) { f.input.SetValue(s) }
func (f *field) Position() int     { return f.input.Position() }
func (f *field) Focused() bool     { return f.input.Focused() }
func (f *field) Blur()             { f.input.Blur() }

// Focus hands the Field the keyboard. The blink command bubbles returns is
// dropped: focus alone leaves the caret solid and the next keystroke re-arms
// the blink, which is what makes a Field taking the keyboard look the same
// whether it happened on launch or on a tab round-trip.
func (f *field) Focus() { f.input.Focus() }

// Update feeds the Field a message and reports whether the value changed —
// which is not the same as having been sent a key. Cursor motion, a chord the
// field does not claim, and the blink's own message all leave the value alone,
// and a caller that resets something on an edit needs to tell those apart.
func (f *field) Update(msg tea.Msg) (tea.Cmd, bool) {
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return cmd, f.input.Value() != before
}

// Rows renders the Field into a width, and reports which of them the caret
// landed on so a caller with less height than the Field has rows can window
// around it. A fieldLine Field always returns exactly one row.
//
// on is the blink's lit half. It is separate from focus because the two answer
// different questions: focus says whether this Field has a caret at all, on
// says whether the caret is showing this frame.
func (f *field) Rows(width int, on bool) ([]string, int) {
	width = max(width, 1)
	value := f.input.Value()
	focused := f.input.Focused()

	if value == "" && f.hint != "" {
		return f.hintRows(width, focused, on)
	}

	style := f.paint
	if style == nil {
		style = plainPaint
	}

	if f.mode == fieldLine {
		// The window is taken before the paint, so the runs describe what is on
		// screen rather than what would be if the row were long enough.
		windowed, caret := windowValue(value, f.input.Position(), width, focused)
		return wrapStyledHard(style(windowed, focused), width, caret, on)
	}

	caret := -1
	if focused {
		caret = min(f.input.Position(), len([]rune(value)))
	}
	return wrapStyledHard(style(value, focused), width, caret, on)
}

// hintRows draws the hint an empty Field shows in place of its value.
//
// The caret sits on the hint's first character rather than in a cell of its
// own in front of it. Given a cell, the hint stepped a column to the right the
// moment the Field took the keyboard — the text moved to make room for a caret
// that has nothing to sit on yet. A hint is exactly what a caret should be
// allowed to sit on, and the row is the same width either way, so nothing moves
// when focus arrives or leaves.
func (f *field) hintRows(width int, focused, on bool) ([]string, int) {
	if !focused {
		return []string{ansi.Truncate(dimStyle.Render(f.hint), width, "")}, 0
	}
	rs := []rune(f.hint)
	head := dimStyle
	if on {
		head = caretStyle
	}
	return []string{ansi.Truncate(head.Render(string(rs[0]))+dimStyle.Render(string(rs[1:])), width, "")}, 0
}

// windowValue slides a single-row value so the caret stays on screen, the way a
// one-line field scrolls rather than wraps.
func windowValue(value string, pos, width int, focused bool) (string, int) {
	rs := []rune(value)
	if !focused {
		if len(rs) <= width {
			return value, -1
		}
		return string(rs[:width]), -1
	}
	// One column is held back for the caret, which needs a cell of its own
	// once it is parked past the last character.
	width = max(1, width-1)
	caret := min(pos, len(rs))
	start := 0
	if caret >= width {
		start = caret - width + 1
	}
	return string(rs[start:min(len(rs), start+width)]), caret - start
}
