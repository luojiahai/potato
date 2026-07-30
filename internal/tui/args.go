// The arg screen: one form for every Placeholder in a Command, with a live
// preview of what will run.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
}

// argPaint carries the selection fill across a focused row's value — the same
// bar the list screen marks its selection with, rather than a second focus
// language for the same idea.
func argPaint(value string, focused bool) []run {
	return []run{{text: value, style: onSelected(textStyle, focused)}}
}

func newArgsScreen(m *Model, command *library.Command) *argsScreen {
	ps := placeholders.Parse(command.Template)
	s := &argsScreen{
		id:       command.ID,
		name:     command.Name,
		command:  command.Template,
		ps:       ps,
		lastArgs: m.st[command.ID].Args,
	}
	fields := make([]field, 0, len(ps))
	for _, p := range ps {
		f := newField(fieldLine)
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
			m.screen = newListScreen(m)
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

// labelWidth is the gutter every arg row shares, so the values line up down
// the panel however long the individual Placeholder names are.
func (s *argsScreen) labelWidth() int {
	w := 0
	for _, p := range s.ps {
		if n := ansi.StringWidth(p.Name); n > w {
			w = n
		}
	}
	return w + 2
}

// row renders one arg: its name in the shared gutter, the value with the caret
// when focused, and the hint right-aligned. The focused row is filled across
// the full width — the same bar the list screen marks its selection with,
// rather than a second focus language for the same idea — so the row carries
// its own content indent instead of being indented from outside, where the
// leading spaces would fall outside the fill and break the bar.
func (s *argsScreen) row(i, width int, on bool) string {
	p := s.ps[i]
	focused := i == s.form.Focused()
	inner := max(1, width-2)

	hint := ""
	if _, ok := s.lastArgs[p.Name]; ok {
		hint = "(Last used)"
	} else if p.HasDefault {
		hint = fmt.Sprintf("(Default: %s)", p.Default)
	}

	gutter := s.labelWidth()
	label := ansi.Truncate(p.Name, gutter-1, "…")
	labelPad := max(0, gutter-ansi.StringWidth(label))

	// The hint yields to the value: it is the first thing dropped when the
	// panel is too narrow to carry both.
	valueWidth := max(1, inner-gutter)
	hintWidth := ansi.StringWidth(hint) + 2
	if hint != "" && valueWidth-hintWidth < 8 {
		hint, hintWidth = "", 0
	}
	valueWidth = max(1, valueWidth-hintWidth)

	rows, _ := s.form.Field(i).Rows(valueWidth, on)
	rendered := rows[0]
	// Measured on the rendered run, not the value: a caret parked past the
	// last character occupies a cell of its own, and charging the gap for the
	// value alone would push the row one column past the edge.
	gap := max(0, valueWidth-ansi.StringWidth(rendered))

	fill := onSelected(lipglossPlain, focused)
	out := fill.Render(contentIndent) + onSelected(dimStyle, focused).Render(label) +
		fill.Render(strings.Repeat(" ", labelPad)) + rendered + fill.Render(strings.Repeat(" ", gap))
	if hint != "" {
		out += onSelected(dimStyle, focused).Render("  " + hint)
	}
	return out
}

func (s *argsScreen) view(m *Model) []string {
	width := m.innerWidth()

	top := []string{rule(width, "Arguments · "+s.name, sectionStyle(), "")}
	on := m.caretOn()
	for i := range s.ps {
		top = append(top, s.row(i, width, on))
	}

	// The filled-in values are the only part of the preview the user just
	// decided, so they carry the highlight and the rest reads as plain text.
	preview := append([]run{{text: "$ ", style: dimStyle}}, renderRuns(s.command, s.values())...)

	// `will run` sits directly under the arg rows it is the result of, rather
	// than absorbing the free height the way the old panel did — a sixteen-row
	// box around a one-line command.
	top = append(top, "")
	top = append(top, section(width, "Will run", sectionStyle(), "", wrapStyled(preview, width-2))...)
	return pin(top, nil, m.bodyHeight())
}
