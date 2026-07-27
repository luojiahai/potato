// Shared chrome for potato's screens: the framed-panel language, the wordmark
// banner, and the footer. Every screen is built from titled round-border
// panels, with match highlighting, arg-count badges and last-used times.
//
// Brand golds from the banner gradient: the muted bottom row for frames, the
// brighter middle row for controls (footer keys, selection pointer), the
// brightest for the things worth spotting inside a command — placeholders and
// fuzzy-match hits. Gold is the only accent; command text is a warm off-white
// so content reads as content rather than as a second accent competing with
// the chrome.

package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/luojiahai/potato/internal/placeholders"
	"github.com/luojiahai/potato/internal/update"
	"github.com/luojiahai/potato/internal/version"
)

// One warm family, every colour given in truecolor. The ANSI indices this
// palette replaced were whatever the user's terminal theme said they were, so
// half of potato's colours used to be outside potato's control and could land
// anywhere against the golds — a cool cyan on a Nord theme, a green-ish yellow
// on Solarized. Naming the values fixes the relationships on every terminal.
const (
	frameColor     = "#d78700" // panel borders
	accentColor    = "#ffaf5f" // controls: footer chords, pointers, focus
	highlightColor = "#ffd75f" // placeholders and fuzzy-match hits
	textColor      = "#e4dccf" // command text
	mutedColor     = "#8c8478" // descriptions, hints, secondary rows
	dangerColor    = "#d1584a" // destructive confirms and validation
	surfaceColor   = "#3a2c14" // the selected row's fill
	inkColor       = "#1a1208" // text on a gold fill
)

var (
	frameStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(frameColor))
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

// titleStyle is the treatment every panel title shares — bold, in the same
// off-white as the content it names, so the gold border stays the frame and
// the title stays the label.
func titleStyle() lipgloss.Style {
	return boldStyle.Foreground(lipgloss.Color(textColor))
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

// ---------- banner ----------

// ANSI Shadow wordmark, assembled per letter so the columns stay aligned.
// Rows are coloured with a top-to-bottom gradient.
var glyphs = map[rune][]string{
	'p': {"██████╗ ", "██╔══██╗", "██████╔╝", "██╔═══╝ ", "██║     ", "╚═╝     "},
	'o': {" ██████╗ ", "██╔═══██╗", "██║   ██║", "██║   ██║", "╚██████╔╝", " ╚═════╝ "},
	't': {"████████╗", "╚══██╔══╝", "   ██║   ", "   ██║   ", "   ██║   ", "   ╚═╝   "},
	'a': {" █████╗ ", "██╔══██╗", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},
}

var bannerGradient = []string{"#ffd75f", "#ffd75f", "#ffaf5f", "#ffaf5f", "#d78700", "#d78700"}

// The brand block comes in three sizes. The full wordmark costs seven rows —
// six glyph rows and the version line — which is over a quarter of a standard
// 24-row terminal spent on decoration, so it is kept for terminals with rows
// to spare. Everything shorter gets the same information on one row, and the
// shortest gets it from the search panel's title instead.
const (
	bannerRowCount = 6
	bannerFull     = bannerRowCount + 1
	bannerCompact  = 1
)

func bannerRows() []string {
	rows := make([]string, bannerRowCount)
	for i := range rows {
		var b strings.Builder
		for _, c := range "potato" {
			b.WriteString(glyphs[c][i])
		}
		rows[i] = b.String()
	}
	return rows
}

func bannerWidth() int {
	w := 0
	for _, row := range bannerRows() {
		if n := ansi.StringWidth(row); n > w {
			w = n
		}
	}
	return w
}

// osc8 makes a label clickable in terminals that support it; the rest render
// the label as plain text, and width measurement ignores the sequence.
func osc8(url, label string) string {
	return "\x1b]8;;" + url + "\x07" + label + "\x1b]8;;\x07"
}

// versionLine is the strapline under the wordmark, and the whole of the
// compact banner beside it: the running version and a clickable repo link.
func versionLine() string {
	v := version.Version
	if v != "dev" {
		v = "v" + v
	}
	return v + "  ·  " + osc8("https://github.com/"+update.Repo, "github.com/"+update.Repo)
}

// banner renders the brand block at whatever size this terminal earns, each
// row carrying the banner's own paddingX of 1.
func (m *Model) banner() []string {
	switch m.bannerHeight() {
	case bannerFull:
		out := make([]string, 0, bannerFull)
		for i, row := range bannerRows() {
			out = append(out, " "+lipgloss.NewStyle().Foreground(lipgloss.Color(bannerGradient[i])).Render(row))
		}
		return append(out, " "+dimStyle.Render(versionLine()))
	case bannerCompact:
		line := accentStyle.Bold(true).Render("potato") + dimStyle.Render("  "+versionLine())
		return []string{" " + ansi.Truncate(line, max(0, m.innerWidth()-1), "")}
	}
	return nil
}

// ---------- panel ----------

// panelBox is a panel's border geometry. Two panels standing side by side used
// to draw two walls between them — a `╮╭` seam that reads as a rendering fault
// rather than a divider. Joined panels share one wall instead: the left panel
// closes on a junction and the right one opens, so the split pane is a single
// framed surface with a rule down it.
type panelBox struct {
	topRight, bottomRight string
	openLeft              bool
}

var (
	boxPlain  = panelBox{topRight: "╮", bottomRight: "╯"}
	boxSeam   = panelBox{topRight: "┬", bottomRight: "┴"}
	boxJoined = panelBox{topRight: "╮", bottomRight: "╯", openLeft: true}
)

// panel draws a standalone round-bordered box.
func panel(title string, titleStyle, borderStyle lipgloss.Style, width int, content []string, height int) []string {
	return panelWith(boxPlain, title, titleStyle, borderStyle, width, content, height)
}

// panelWith draws a box of the given width and total height, with the title
// overlaid on the top border. Content is padded one column on each side and
// truncated to the inner width; short content is padded with blank rows. An
// open-left box omits its own left wall and is drawn hard against the panel to
// its left, sharing that panel's seam.
func panelWith(b panelBox, title string, titleStyle, borderStyle lipgloss.Style, width int, content []string, height int) []string {
	// border columns plus one padding column on each side — an open-left box
	// spends one fewer, having no left border to draw
	chrome := 4
	if b.openLeft {
		chrome = 3
	}
	// A panel narrower than its own chrome cannot be drawn; clamp rather than
	// render a negative run of border.
	width = max(width, chrome)
	height = max(height, 2)
	inner := width - chrome

	// An open-left box draws no left wall at all, so it must not emit the
	// escape sequences that would have styled one either.
	leftTop, leftBottom := "╭", "╰"
	leftEdge := borderStyle.Render("│")
	if b.openLeft {
		leftTop, leftBottom, leftEdge = "", "", ""
	}

	top := leftTop + strings.Repeat("─", inner+2) + b.topRight
	if title != "" {
		top = overlayTitle(top, " "+title+" ", chrome-2, titleStyle, borderStyle)
	} else {
		top = borderStyle.Render(top)
	}
	out := make([]string, 0, height)
	out = append(out, top)
	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		line = ansi.Truncate(line, inner, "")
		pad := max(0, inner-ansi.StringWidth(line))
		out = append(out, leftEdge+" "+line+strings.Repeat(" ", pad)+" "+borderStyle.Render("│"))
	}
	out = append(out, borderStyle.Render(leftBottom+strings.Repeat("─", inner+2)+b.bottomRight))
	return out
}

// overlayTitle writes the title over the top border starting at the panel's
// content column, leaving the border visible on both sides.
func overlayTitle(top, title string, at int, titleStyle, borderStyle lipgloss.Style) string {
	runes := []rune(top)
	titleWidth := ansi.StringWidth(title)
	if at+titleWidth > len(runes)-1 {
		titleWidth = max(0, len(runes)-1-at)
		title = ansi.Truncate(title, titleWidth, "")
	}
	return borderStyle.Render(string(runes[:at])) +
		titleStyle.Render(title) +
		borderStyle.Render(string(runes[at+titleWidth:]))
}

// ---------- footer ----------

type footerKey struct{ chord, label string }

// footer renders the two rows every screen ends with: a blank margin row and
// either the key hints or a flash toast.
func footer(keys []footerKey, flash string) []string {
	if flash != "" {
		return []string{"", " " + flashStyle.Render(" "+flash+" ")}
	}
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(dimStyle.Render(" · "))
		}
		b.WriteString(accentStyle.Bold(true).Render(k.chord))
		b.WriteString(dimStyle.Render(" " + k.label))
	}
	return []string{"", " " + b.String()}
}

// field renders one labelled input row: a 14-column label gutter carrying the
// focus pointer, then the value and an optional dim hint.
// fieldPanel draws one input as a titled panel of its own. The label rides the
// top border, so it costs no content row and the value gets the panel's full
// width — where the old shared 14-column label gutter charged every field for
// the widest label and indented every value past it.
//
// Focus is carried by the border and title colour alone. With each field in
// its own frame there is nothing left for a `❯` to disambiguate.
func fieldPanel(label, hint string, rows []string, focused bool, width, height int) []string {
	title := label
	if hint != "" {
		title += " " + hint
	}
	border, name := frameStyle, titleStyle()
	if focused {
		border, name = accentStyle, titleStyle().Foreground(lipgloss.Color(accentColor))
	}
	return panel(title, name, border, width, rows, height)
}

// valueRows renders a field's value wrapped to the panel, with the block caret
// drawn in when the field has focus.
func valueRows(runs []run, value string, pos, width int, focused bool) []string {
	caret := -1
	if focused {
		caret = min(pos, len([]rune(value)))
	}
	return wrapStyledHard(runs, width, caret)
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

var lipglossPlain = lipgloss.NewStyle()

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
// rune, and overlays a block caret on the one at caret (negative draws none).
//
// Editing surfaces wrap this way rather than on word boundaries. Word wrapping
// discards the space it broke on, and a caret sitting on a discarded rune has
// no cell to be drawn in — it would vanish exactly while you were typing at
// the panel's edge. Breaking where the panel ends keeps every rune addressable,
// and suits command text, which is not prose.
func wrapStyledHard(runs []run, width, caret int) []string {
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
		return []string{""}
	}

	var out []string
	var line strings.Builder
	var pending []rune
	pendingOwner := -1
	used := 0

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
		if owner[i] != pendingOwner {
			flush()
			pendingOwner = owner[i]
		}
		pending = append(pending, r)
		used += w
	}
	flush()
	return append(out, line.String())
}

func renderCommand(template string, values map[string]string) string {
	return placeholders.Render(template, values)
}

// wrapOrNot folds to width, or leaves the text on one line when width is 0 —
// the shape the layout measures before it knows how wide the panel will be.
func wrapOrNot(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	return wrapLines(text, width)
}
