// Shared chrome for potato's screens: the framed-panel language, the wordmark
// banner, and the footer. Every screen is built from titled round-border
// panels, with match highlighting, arg-count badges and last-used times.
//
// Brand golds from the banner gradient: the muted bottom row for frames, the
// brighter middle row for controls (footer keys, selection pointer) — chrome
// stays warm, cyan stays reserved for command content.

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

const (
	frameColor  = "#d78700"
	accentColor = "#ffaf5f"
)

var redColor = lipgloss.Color("1")

var (
	frameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(frameColor))
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(accentColor))
	cyanStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	redStyle    = lipgloss.NewStyle().Foreground(redColor)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	boldStyle   = lipgloss.NewStyle().Bold(true)
	flashStyle  = lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0"))
)

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

// bannerRowCount is the wordmark's glyph rows; the version line adds one more.
const bannerRowCount = 6

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

// banner renders the wordmark block — six gradient rows plus the version and
// repo line — each carrying the banner's own paddingX of 1.
func banner() []string {
	out := make([]string, 0, bannerRowCount+1)
	for i, row := range bannerRows() {
		out = append(out, " "+lipgloss.NewStyle().Foreground(lipgloss.Color(bannerGradient[i])).Render(row))
	}
	v := version.Version
	if v != "dev" {
		v = "v" + v
	}
	repoURL := "https://github.com/" + update.Repo
	out = append(out, " "+dimStyle.Render(v+"  ·  "+osc8(repoURL, "github.com/"+update.Repo)))
	return out
}

// ---------- panel ----------

// panel draws a round-bordered box of the given width and total height, with
// the title overlaid on the top border. Content is padded one column on each
// side and truncated to the inner width; short content is padded with blank
// rows.
func panel(title string, titleStyle, borderStyle lipgloss.Style, width int, content []string, height int) []string {
	// A panel narrower than its own chrome cannot be drawn; clamp rather than
	// render a negative run of border.
	width = max(width, 4)
	height = max(height, 2)
	inner := width - 4 // two border columns, two padding columns
	top := "╭" + strings.Repeat("─", width-2) + "╮"
	if title != "" {
		top = overlayTitle(top, " "+title+" ", titleStyle, borderStyle)
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
		pad := inner - ansi.StringWidth(line)
		if pad < 0 {
			pad = 0
		}
		out = append(out, borderStyle.Render("│")+" "+line+strings.Repeat(" ", pad)+" "+borderStyle.Render("│"))
	}
	out = append(out, borderStyle.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return out
}

// overlayTitle writes the title over the top border starting at the content
// column (border + padding = 2), leaving the border visible on both sides.
func overlayTitle(top, title string, titleStyle, borderStyle lipgloss.Style) string {
	runes := []rune(top)
	titleWidth := ansi.StringWidth(title)
	const at = 2
	if at+titleWidth > len(runes)-1 {
		titleWidth = len(runes) - 1 - at
		if titleWidth < 0 {
			titleWidth = 0
		}
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

// ---------- field ----------

// field renders one labelled input row: a 14-column label gutter carrying the
// focus pointer, then the value and an optional dim hint.
func field(label, value string, focused bool, hint string) string {
	pointer := "  "
	if focused {
		pointer = "❯ "
	}
	gutter := pointer + label
	if pad := 14 - ansi.StringWidth(gutter); pad > 0 {
		gutter += strings.Repeat(" ", pad)
	}
	styledLabel := boldStyle.Render(pointer + label)
	if focused {
		styledLabel = boldStyle.Foreground(lipgloss.Color("6")).Render(pointer + label)
	}
	out := styledLabel + strings.Repeat(" ", max(0, 14-ansi.StringWidth(pointer+label))) + value
	if hint != "" {
		out += dimStyle.Render("  " + hint)
	}
	return out
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
