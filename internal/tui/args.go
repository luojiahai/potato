// The arg screen: one form for every Placeholder in a Command, with a live
// preview of what will run.

package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/placeholders"
)

type argsScreen struct {
	id       string
	name     string
	command  string
	ps       []placeholders.Placeholder
	lastArgs map[string]string
	form     form
	// back is the screen this one was opened from, and where esc leads. Held
	// rather than rebuilt so the list is found as it was left.
	back screen
}

// argPaint carries the selection fill across a focused row's value — the same
// bar the list screen marks its selection with, rather than a second focus
// language for the same idea.
func argPaint(value string, focused bool) []run {
	return []run{{text: value, style: onSelected(textStyle, focused)}}
}

func newArgsScreen(m *Model, back screen, command *library.Command) *argsScreen {
	ps := placeholders.Parse(command.Template)
	s := &argsScreen{
		id:       command.ID,
		name:     command.Name,
		command:  command.Template,
		ps:       ps,
		lastArgs: m.st[command.ID].Args,
		back:     back,
	}
	fields := make([]field, 0, len(ps))
	for _, p := range ps {
		f := newField(lineMode)
		f.paint = argPaint
		// pre-fill precedence: last value > default > empty (spec §2)
		if value, ok := s.lastArgs[p.Name]; ok {
			f.SetValue(value)
		} else if p.HasDefault {
			f.SetValue(p.Default)
		}
		fields = append(fields, f)
	}
	s.form = newForm(fields...)
	return s
}

func (s *argsScreen) values() map[string]string {
	out := map[string]string{}
	for i, p := range s.ps {
		out[p.Name] = s.form.Field(i).Value()
	}
	return out
}

// update claims the screen's own three verbs; the ring and the typing are the
// Form's.
func (s *argsScreen) update(m *Model, msg tea.Msg) tea.Cmd {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, keymap.args.Back):
			m.screen = s.back
			return nil
		case key.Matches(keyMsg, keymap.args.Run):
			return m.run(s.id, s.values())
		case key.Matches(keyMsg, keymap.args.Copy):
			return m.copy(s.id, s.values())
		}
	}
	return s.form.Update(msg)
}

func (s *argsScreen) keys(*Model) []footerKey {
	return footerKeys(keymap.args.Run, keymap.args.Copy, keymap.form.Next, keymap.args.Back)
}

// note is where an arg's pre-filled value came from — a different thing from a
// Field's hint, and named apart from it.
func (s *argsScreen) note(p placeholders.Placeholder) string {
	if _, ok := s.lastArgs[p.Name]; ok {
		return "(Last used)"
	}
	if p.HasDefault {
		return fmt.Sprintf("(Default: %s)", p.Default)
	}
	return ""
}

// The arg row's four columns, in the order they are drawn.
const (
	argIndent = iota
	argLabel
	argValue
	argNote
)

// argColumns is the arg row's two arrangements. The note yields to the value:
// it is the first thing given up when the panel is too narrow to carry both.
//
// The notes go as a block, all of them or none — never row by row, which on a
// narrow panel would keep a Placeholder's short note while its neighbour's long
// one goes. Sharing one value column is what lets the values line up, and it is
// the same trade the label gutter and the list row's name column already make:
// a column is a property of the block, not of the row that happens to be widest
// in it.
func argColumns(withNote bool) arrangement {
	a := arrangement{
		argIndent: {spend: spendFixed, n: contentIndentWidth}, // inside the fill, not outside it
		argLabel:  {spend: spendWidest, pad: 2},               // the gutter the values hang from
		argValue:  {spend: spendFlex, needs: contentFloor},
		argNote:   {spend: spendWidest, lead: 2, align: alignRight},
	}
	if !withNote {
		a[argValue].needs = 1
		a[argNote] = column{spend: spendNone}
	}
	return a
}

// rows lays the args out as one block: the label gutter is measured once across
// every Placeholder, and the notes take one column between them so they line up
// down the panel however long any one of them is.
//
// The focused row is filled across the full width — the same bar the list
// screen marks its selection with, rather than a second focus language for the
// same idea — so the row carries its own content indent instead of being
// indented from outside, where the leading spaces would fall outside the fill
// and break the bar. The Layout fills every run and pad it draws; the value is
// the one cell it hands over already rendered, and that one is filled by the
// Field's own painter (see argPaint and cell).
func (s *argsScreen) rows(width int, on bool) []string {
	block := make([]blockRow, 0, len(s.ps))
	withNote := false
	for i, p := range s.ps {
		note := s.note(p)
		if note != "" {
			withNote = true
		}
		cells := make([]cell, 4)
		cells[argIndent] = cell{} // nothing to draw; the fill runs through it
		cells[argLabel] = textCell(p.Name, dimStyle)
		cells[argValue] = fieldCell(s.form.Field(i), on)
		cells[argNote] = textCell(note, dimStyle)
		block = append(block, blockRow{cells: cells, selected: i == s.form.Focused()})
	}
	return layout(width,
		candidate{columns: argColumns(withNote), rows: block},
		candidate{columns: argColumns(false), rows: block},
	)
}

func (s *argsScreen) view(m *Model) []string {
	width := m.innerWidth()

	top := []string{rule(width, "Arguments · "+s.name, sectionStyle(), "")}
	top = append(top, s.rows(width, m.caretOn())...)

	// The filled-in values are the only part of the preview the user just
	// decided, so they carry the highlight and the rest reads as plain text.
	preview := withPrompt(renderRuns(s.command, s.values()))

	// `will run` sits directly under the arg rows it is the result of and takes
	// only the height its content needs. The free height collects below it,
	// rather than swelling a box around a one-line command.
	top = append(top, "")
	top = append(top, section(width, "Will run", sectionStyle(), "", wrapStyled(preview, width-2))...)
	return pin(top, nil, m.bodyHeight())
}
