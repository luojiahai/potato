// Shared chrome for potato's screens: the hairline-rule language every section
// is headed with, the layout that pins a screen's two blocks, and the footer.
//
// Structure comes from rules and alignment rather than from boxes. A framed
// panel spends two rows and four columns on every region it draws, and fences
// whatever space it cannot fill — which is how a three-command library used to
// render sixteen rows of empty box. A rule costs one row, no columns, and
// leaves the slack around it as plain unframed space, which reads as space
// rather than as something unfinished.
//
// Brand golds: a dim warm brown for the rules, which are structure and should
// recede — a full-width rule in the old border gold was louder than the box
// edges it replaces; the brighter gold for controls (footer chords, pointer,
// focus); the brightest for the things worth spotting inside a command —
// placeholders and fuzzy-match hits. Command text is a warm off-white so
// content reads as content rather than as a second accent competing with the
// chrome.

package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/luojiahai/potato/internal/placeholders"
	"github.com/luojiahai/potato/internal/version"
)

// One warm family, every colour given in truecolor. The ANSI indices this
// palette replaced were whatever the user's terminal theme said they were, so
// half of potato's colours used to be outside potato's control and could land
// anywhere against the golds — a cool cyan on a Nord theme, a green-ish yellow
// on Solarized. Naming the values fixes the relationships on every terminal.
const (
	ruleColor      = "#5c4a2e" // hairline rules
	accentColor    = "#ffaf5f" // controls: footer chords, pointers, focus
	highlightColor = "#ffd75f" // placeholders and fuzzy-match hits
	textColor      = "#e4dccf" // command text
	mutedColor     = "#8c8478" // descriptions, hints, secondary rows
	dangerColor    = "#d1584a" // destructive confirms and validation
	surfaceColor   = "#3a2c14" // the selected row's fill
	inkColor       = "#1a1208" // text on a gold fill
)

var (
	ruleStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(ruleColor))
	accentStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(accentColor))
	highlightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(highlightColor))
	textStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(textColor))
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color(mutedColor))
	dangerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(dangerColor))
	boldStyle      = lipgloss.NewStyle().Bold(true)
	// The caret is a solid cell on the character, the way a terminal's own
	// block cursor works — a fixed pair of colours rather than `Reverse`,
	// which would swap whatever the run underneath carries and turn the caret
	// gold over a placeholder and off-white over literal text.
	caretStyle = lipgloss.NewStyle().Background(lipgloss.Color(accentColor)).Foreground(lipgloss.Color(inkColor))
	flashStyle = lipgloss.NewStyle().Background(lipgloss.Color(accentColor)).Foreground(lipgloss.Color(inkColor))
)

// contentIndent is the two columns every content row is inset by, so that text
// lands in the same column as a list row's name — the selection pointer
// occupies exactly this much.
const contentIndent = "  "

// sectionStyle labels a region: muted, so the label names the rule without
// competing with the content under it.
func sectionStyle() lipgloss.Style { return boldStyle.Foreground(lipgloss.Color(mutedColor)) }

// focusStyle is sectionStyle for the field that has the keyboard. With no
// borders left to carry focus, the rule and its label do it.
func focusStyle() lipgloss.Style { return boldStyle.Foreground(lipgloss.Color(accentColor)) }

// titleStyle names a thing rather than a region — a Command's own name over
// the detail strip, in the same off-white as the content it heads.
func titleStyle() lipgloss.Style { return boldStyle.Foreground(lipgloss.Color(textColor)) }

// brandStyle is the app's own name on the header rule — the one label that is
// neither a region nor a Command, and the only place the accent gold heads a
// rule rather than marking something you can press.
func brandStyle() lipgloss.Style { return boldStyle.Foreground(lipgloss.Color(accentColor)) }

// versionLabel is what the header rule carries at its right end. The repo link
// that used to sit beside it lives in `potato --help`, which is where you look
// when you want it; the running version is worth a glance every launch.
func versionLabel() string {
	if version.Version == "dev" {
		return version.Version
	}
	return "v" + version.Version
}

// onSelected applies the selection bar's fill to a style. Every run in a
// selected row has to carry it — lipgloss cannot cascade a background over
// already-rendered escape sequences, so a run that skips it punches a hole in
// the bar.
func onSelected(style lipgloss.Style, selected bool) lipgloss.Style {
	if !selected {
		return style
	}
	return style.Background(lipgloss.Color(surfaceColor))
}

var lipglossPlain = lipgloss.NewStyle()

// ---------- rules ----------

// rule draws one hairline across the full width, with an optional label at the
// left and an optional annotation right-aligned at the end of it. The label
// sits a space in from the leading dash so the rule reads as a section head
// rather than as a caption that happens to have a line through it.
func rule(width int, label string, labelStyle lipgloss.Style, right string) string {
	width = max(width, 0)
	var b strings.Builder
	used := 0
	if label != "" {
		b.WriteString(ruleStyle.Render("─ "))
		used += 2
		label = ansi.Truncate(label, max(0, width-used-1), "…")
		b.WriteString(labelStyle.Render(label))
		used += ansi.StringWidth(label)
		b.WriteString(ruleStyle.Render(" "))
		used++
	}
	// The annotation yields to the rule: on a width that cannot carry both it
	// is dropped rather than pushed past the edge.
	rightWidth := 0
	if right != "" {
		rightWidth = ansi.StringWidth(right) + 1
	}
	if used+rightWidth > width {
		right, rightWidth = "", 0
	}
	b.WriteString(ruleStyle.Render(strings.Repeat("─", max(0, width-used-rightWidth))))
	if right != "" {
		b.WriteString(dimStyle.Render(" " + right))
	}
	return b.String()
}

// section heads a block of content rows with a labelled rule and insets them by
// the shared content indent.
func section(width int, label string, labelStyle lipgloss.Style, right string, rows []string) []string {
	out := make([]string, 0, len(rows)+1)
	out = append(out, rule(width, label, labelStyle, right))
	return append(out, indent(rows)...)
}

// indent insets content rows to the column a list row's name sits in.
func indent(rows []string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = contentIndent + row
	}
	return out
}

// ---------- layout ----------

// pin lays out a screen's body: content from the top, a status line flush
// against the footer, and blank rows between them.
//
// Every screen is exactly the session's body height — see Model.measure. The
// blanks are not the framed void the boxes used to draw: they are the bottom of
// a block that holds still while you type, in a terminal that would otherwise
// reflow under it on every keystroke.
//
// The status line — the edit screen's validation warning — keeps its rows when
// the two blocks together will not fit. The top block is the one that gives,
// since it is the one with somewhere to scroll.
func pin(top, bottom []string, height int) []string {
	if height <= 0 {
		return nil
	}
	if len(bottom) > height {
		bottom = bottom[:height]
	}
	room := height - len(bottom)
	if len(top) > room {
		top = top[:room]
	}
	out := make([]string, 0, height)
	out = append(out, top...)
	for len(out) < room {
		out = append(out, "")
	}
	return append(out, bottom...)
}

// ---------- footer ----------

type footerKey struct{ chord, label string }

// footer renders the two rows every screen ends with: the rule that anchors the
// keys to the bottom of the screen, and either the key hints or a flash toast.
func footer(keys []footerKey, flash string, width int) []string {
	out := []string{rule(width, "", lipglossPlain, "")}
	if flash != "" {
		return append(out, flashStyle.Render(" "+flash+" "))
	}
	// A narrow terminal cannot carry six chords. Drop them from the inside out
	// rather than off the end: the first is what the screen is for and the last
	// is the way out of it, and a footer that fits by hiding `esc` is worse
	// than one that hides `^D`. Past two chords, drop the labels and keep the
	// chords themselves — `↵ · esc` still says which keys do something.
	for len(keys) > 2 && footerWidth(keys) > width {
		keys = append(keys[:len(keys)-2], keys[len(keys)-1])
	}
	labelled := footerWidth(keys) <= width
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(dimStyle.Render(" · "))
		}
		b.WriteString(accentStyle.Bold(true).Render(k.chord))
		if labelled {
			b.WriteString(dimStyle.Render(" " + k.label))
		}
	}
	return append(out, b.String())
}

func footerWidth(keys []footerKey) int {
	w := 0
	for i, k := range keys {
		if i > 0 {
			w += 3 // " · "
		}
		w += ansi.StringWidth(k.chord) + 1 + ansi.StringWidth(k.label)
	}
	return w
}

// ---------- fields ----------

// valueRowsAt renders a field's value wrapped to the width, with the block
// caret drawn in when the field has focus, and reports which row the caret
// landed on so a field taller than the space left for it can be windowed
// around it.
func valueRowsAt(runs []run, value string, pos, width int, focused bool) ([]string, int) {
	caret := -1
	if focused {
		caret = min(pos, len([]rune(value)))
	}
	return wrapStyledHard(runs, width, caret)
}

// window slides a block of rows so that row `at` stays visible, the way a
// single-line field scrolls its value.
func window(rows []string, at, height int) []string {
	if height <= 0 || len(rows) <= height {
		return rows
	}
	start := 0
	if at >= height {
		start = at - height + 1
	}
	start = min(start, len(rows)-height)
	return rows[start : start+height]
}

// ---------- misc ----------

func timeAgo(iso string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	minutes := int(now.Sub(t).Minutes())
	if minutes < 1 {
		return "just now"
	}
	if minutes < 60 {
		return fmt.Sprintf("%dm ago", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	}
	return fmt.Sprintf("%dd ago", hours/24)
}

// trimRightVisible drops trailing spaces while preserving the escape
// sequences that follow them — every rendered line is trimmed, and a styled
// run's reset sits after the padding it closes.
func trimRightVisible(s string) string {
	toks := tokenize(s)
	for {
		idx := -1
		for i := len(toks) - 1; i >= 0; i-- {
			if !strings.HasPrefix(toks[i], "\x1b") {
				idx = i
				break
			}
		}
		if idx < 0 || toks[idx] != " " {
			break
		}
		toks = append(toks[:idx], toks[idx+1:]...)
	}
	return strings.Join(toks, "")
}

// tokenize splits a string into single runes and whole escape sequences
// (CSI ... final byte, and OSC ... BEL/ST).
func tokenize(s string) []string {
	var out []string
	rs := []rune(s)
	for i := 0; i < len(rs); {
		if rs[i] != 0x1b {
			out = append(out, string(rs[i]))
			i++
			continue
		}
		j := i + 1
		switch {
		case j < len(rs) && rs[j] == '[':
			j++
			for j < len(rs) && !(rs[j] >= '@' && rs[j] <= '~') {
				j++
			}
			j++
		case j < len(rs) && rs[j] == ']':
			for j < len(rs) && rs[j] != 0x07 {
				if rs[j] == 0x1b && j+1 < len(rs) && rs[j+1] == '\\' {
					j++
					break
				}
				j++
			}
			j++
		default:
			j++
		}
		if j > len(rs) {
			j = len(rs)
		}
		out = append(out, string(rs[i:j]))
		i = j
	}
	return out
}

// wrapLines folds text to the given width, word-wrapping with hard breaks for
// over-long tokens.
func wrapLines(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	return strings.Split(ansi.Wrap(text, width, ""), "\n")
}

// run is a styled slice of a line — the unit both the arg screen's live
// preview and the edit screen's template are built from.
type run struct {
	text  string
	style lipgloss.Style
}

// wrapStyled folds a styled line to width. Wrap points are computed on the
// plain text so the styling can never disagree with the geometry, then the
// runs are sliced at those points — the same offsets the highlighting uses.
func wrapStyled(runs []run, width int) []string {
	var plain []rune
	var owner []int
	for i, r := range runs {
		for _, c := range r.text {
			plain = append(plain, c)
			owner = append(owner, i)
		}
	}

	var out []string
	pos := 0
	for _, line := range wrapLines(string(plain), width) {
		n := len([]rune(line))
		var b strings.Builder
		for i := pos; i < pos+n && i < len(plain); i++ {
			start := i
			for i+1 < pos+n && i+1 < len(plain) && owner[i+1] == owner[start] {
				i++
			}
			b.WriteString(runs[owner[start]].style.Render(string(plain[start : i+1])))
		}
		out = append(out, b.String())
		pos += n
		// the wrapper drops the whitespace it broke on
		for pos < len(plain) && plain[pos] == ' ' {
			pos++
		}
	}
	return out
}

// wrapStyledHard folds a styled line at the panel's edge without dropping a
// rune, overlays a block caret on the one at caret (negative draws none), and
// reports the row that caret landed on.
//
// Editing surfaces wrap this way rather than on word boundaries. Word wrapping
// discards the space it broke on, and a caret sitting on a discarded rune has
// no cell to be drawn in — it would vanish exactly while you were typing at
// the panel's edge. Breaking where the panel ends keeps every rune addressable,
// and suits command text, which is not prose.
func wrapStyledHard(runs []run, width, caret int) ([]string, int) {
	width = max(width, 1)

	// owner indexes into runs per rune, so consecutive runes can be batched
	// into one styled write — lipgloss.Style holds a slice and a func, so the
	// styles themselves cannot be compared for equality.
	var text []rune
	var owner []int
	for i, r := range runs {
		for _, c := range r.text {
			text = append(text, c)
			owner = append(owner, i)
		}
	}
	if caret >= 0 {
		// A caret parked past the last character gets a cell of its own — the
		// only case where it occupies a column nothing else wants, and it
		// displaces nothing because there is nothing to its right.
		if caret >= len(text) {
			text = append(text, ' ')
			owner = append(owner, 0)
			caret = len(text) - 1
		}
		// Point the caret's rune at a run of its own, so the batching below
		// breaks around it without needing to know it is there.
		owner[caret] = len(runs)
		runs = append(runs, run{style: caretStyle})
	}
	if len(text) == 0 {
		return []string{""}, 0
	}

	var out []string
	var line strings.Builder
	var pending []rune
	pendingOwner := -1
	used := 0
	caretRow := 0

	flush := func() {
		if len(pending) > 0 {
			line.WriteString(runs[pendingOwner].style.Render(string(pending)))
			pending = pending[:0]
		}
	}
	for i, r := range text {
		w := ansi.StringWidth(string(r))
		if used+w > width && used > 0 {
			flush()
			out = append(out, line.String())
			line.Reset()
			used = 0
		}
		if i == caret {
			caretRow = len(out)
		}
		if owner[i] != pendingOwner {
			flush()
			pendingOwner = owner[i]
		}
		pending = append(pending, r)
		used += w
	}
	flush()
	return append(out, line.String()), caretRow
}

func renderCommand(template string, values map[string]string) string {
	return placeholders.Render(template, values)
}
