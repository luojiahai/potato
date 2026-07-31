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
	// wrapMode folds the value at the width and grows downward — the add/edit
	// fields, where the command being typed is the tallest thing on screen.
	wrapMode fieldMode = iota
	// lineMode keeps the value on one row and slides it under the caret — the
	// search field and the arg rows, which sit in rows they cannot grow out of.
	lineMode
)

// painter maps a value to the runs it renders as, given whether the Field has
// the keyboard. It is the one hook for what a Field looks like: the command
// field picks its Placeholders out in gold, an arg row carries the selection
// fill across its value, and everything else is plain command text.
type painter func(value string, focused bool) []run

func plainPaint(value string, _ bool) []run {
	return []run{{text: value, style: textStyle}}
}

// withPrompt puts the `$ ` gutter in front of a command's runs — the detail
// strip's block and the arg screen's preview are the same thing rendered in two
// places, and the gutter is part of what makes them read that way.
func withPrompt(runs []run) []run {
	return append([]run{{text: "$ ", style: dimStyle}}, runs...)
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
	model textinput.Model
	mode  fieldMode
	// paint is how the value looks; nil is plain command text.
	paint painter
	// hint is what an empty Field shows in its place, and "" is nothing. It is
	// text the caret sits on rather than a row of its own — see hintRows.
	hint string
}

// newField builds a Field. The bubbles model inside it is stripped to the parts
// potato reads: no prompt, and none of the styling it would need to draw itself.
// It is never asked to — Rows is the only thing that renders a Field, and it
// reads the value and the cursor and paints them itself. Configuring styles the
// model would only use from View would be describing a frame nothing draws, and
// the second description of potato's palette that this file exists to remove.
//
// field and form capitalise their methods where the screens beside them do not.
// They are the two types here with an interface a caller has to learn rather
// than a body a screen reads top to bottom, and the capital marks that surface.
// The lowercase alternative also collides: `focus` is already a type in list.go
// and `field` is this one.
func newField(mode fieldMode) field {
	model := textinput.New()
	model.Prompt = ""
	return field{model: model, mode: mode}
}

func (f *field) Value() string     { return f.model.Value() }
func (f *field) SetValue(s string) { f.model.SetValue(s) }
func (f *field) Position() int     { return f.model.Position() }
func (f *field) Focused() bool     { return f.model.Focused() }
func (f *field) Blur()             { f.model.Blur() }

// Focus hands the Field the keyboard. The blink command bubbles returns is
// dropped: focus alone leaves the caret solid and the next keystroke re-arms
// the blink, which is what makes a Field taking the keyboard look the same
// whether it happened on launch or on a tab round-trip.
func (f *field) Focus() { f.model.Focus() }

// Update feeds the Field a message and reports whether the value changed —
// which is not the same as having been sent a key. Cursor motion, a chord the
// field does not claim, and the blink's own message all leave the value alone,
// and a caller that resets something on an edit needs to tell those apart.
func (f *field) Update(msg tea.Msg) (tea.Cmd, bool) {
	before := f.model.Value()
	var cmd tea.Cmd
	f.model, cmd = f.model.Update(msg)
	return cmd, f.model.Value() != before
}

// Rows renders the Field into a width, and reports which of them the caret
// landed on so a caller with less height than the Field has rows can window
// around it. A lineMode Field always returns exactly one row.
//
// on is the blink's lit half. It is separate from focus because the two answer
// different questions: focus says whether this Field has a caret at all, on
// says whether the caret is showing this frame.
func (f *field) Rows(width int, on bool) ([]string, int) {
	width = max(width, 1)
	value := f.model.Value()
	focused := f.model.Focused()

	if value == "" && f.hint != "" {
		return f.hintRows(width, focused, on)
	}

	paint := f.paint
	if paint == nil {
		paint = plainPaint
	}

	if f.mode == lineMode {
		// The window is taken before the paint, so the runs describe what is on
		// screen rather than what would be if the row were long enough.
		windowed, caret := windowValue(value, f.model.Position(), width, focused)
		return wrapStyledHard(paint(windowed, focused), width, caret, on)
	}

	caret := -1
	if focused {
		caret = min(f.model.Position(), len([]rune(value)))
	}
	return wrapStyledHard(paint(value, focused), width, caret, on)
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
// one-line field scrolls rather than wraps. It returns the visible slice and the
// caret's rune index within it.
//
// It counts columns, not runes. Counting runes is the same arithmetic only while
// every rune is one column wide: a value of CJK or emoji windowed to a rune
// count is up to twice as many columns as it was given, and the row it is handed
// to wraps — so a one-line Field returned two rows, the caller took the first,
// and the caret was in the one it threw away. That is the very thing this
// windowing exists to prevent.
func windowValue(value string, pos, width int, focused bool) (string, int) {
	rs := []rune(value)
	width = max(1, width)

	if !focused {
		end, used := 0, 0
		for end < len(rs) {
			w := runeWidth(rs[end])
			if used+w > width {
				break
			}
			used += w
			end++
		}
		return string(rs[:end]), -1
	}

	// bubbles clamps the cursor into the value on every SetValue and SetCursor,
	// so this cannot bite today. It is the one assumption this file makes about
	// the model inside it, and a caret index past the end would window around a
	// rune that is not there.
	caret := min(pos, len(rs))

	// The caret's own cell is reserved before anything else: the rune it sits on,
	// or one blank column when it is parked past the last one. Without that, a
	// window filled from the left can end exactly at the caret and leave it
	// nothing to sit on — the block is drawn past the text instead of over it.
	cell := 1
	if caret < len(rs) {
		cell = runeWidth(rs[caret])
	}
	used := min(cell, width)
	start, end := caret, caret
	if caret < len(rs) && used == cell {
		end = caret + 1
	}

	// Grow left first, so a caret at the end of a long value shows the columns
	// behind it, then spend whatever is left going right.
	for start > 0 {
		w := runeWidth(rs[start-1])
		if used+w > width {
			break
		}
		used += w
		start--
	}
	for end < len(rs) {
		w := runeWidth(rs[end])
		if used+w > width {
			break
		}
		used += w
		end++
	}
	return string(rs[start:end]), caret - start
}

func runeWidth(r rune) int { return ansi.StringWidth(string(r)) }
