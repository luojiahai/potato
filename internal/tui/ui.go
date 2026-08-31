// Shared chrome for potato's screens: the hairline-rule language every section
// is headed with, the layout that pins a screen's two blocks, and the footer.
//
// Structure comes from rules and alignment rather than from boxes. A framed
// panel spends two rows and four columns on every region it draws, and fences
// whatever space it cannot fill — a three-command library inside one would be
// sixteen rows of empty box. A rule costs one row, no columns, and leaves the
// slack around it as plain unframed space, which reads as space rather than as
// something unfinished.
//
// The colours are roles rather than fixed hues. The rules are structure and
// should recede — a full-width rule in the accent shouts across the frame where
// a box edge only outlines one; the accent marks the controls (footer chords,
// pointer, focus); the highlight marks the things worth spotting inside a
// command — placeholders and fuzzy-match hits. Command text is its own quiet
// colour so content reads as content rather than as a second accent competing
// with the chrome. Two palettes fill those roles, one per terminal ground; see
// palette below.

package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/luojiahai/potato/internal/placeholders"
	"github.com/luojiahai/potato/internal/version"
)

// palette is the eight colour roles potato paints with. Every value is given in
// truecolor — never an ANSI index, which is whatever the user's terminal theme
// says it is and can land anywhere against the rest: a cool cyan on a Nord
// theme, a green-ish yellow on Solarized. Naming the values fixes the
// relationships on every terminal.
type palette struct {
	rule      string // hairline rules
	accent    string // controls: footer chords, pointers, focus
	highlight string // placeholders and fuzzy-match hits
	text      string // command text
	muted     string // descriptions, hints, secondary rows
	danger    string // destructive confirms and validation
	surface   string // the selected row's fill
	ink       string // text on an accent or a danger fill
}

// darkPalette is one warm family of golds and browns, for the dark ground most
// terminals have.
var darkPalette = palette{
	rule:      "#5c4a2e",
	accent:    "#ffaf5f",
	highlight: "#ffd75f",
	text:      "#e4dccf",
	muted:     "#8c8478",
	danger:    "#d1584a",
	surface:   "#3a2c14",
	ink:       "#1a1208",
}

// lightPalette pitches the same family for a light ground: the golds darken to
// browns, and the fills swap ends so ink is the pale one. The invariant it is
// chosen for is that every text role clears WCAG AA — 4.5:1 — both on white and
// on the warm off-white a Solarized-light terminal paints, and ink clears it on
// the accent and danger fills it is written over.
var lightPalette = palette{
	rule:      "#d8c8a6",
	accent:    "#9a5300",
	highlight: "#7d5e00",
	text:      "#453a28",
	muted:     "#6e6555",
	danger:    "#b02f20",
	surface:   "#f0e0bd",
	ink:       "#fff8ee",
}

// The live palette and the styles derived from it. Every renderer in the
// package reads these names; applyPalette is the only thing that writes them.
var (
	ruleColor      string
	accentColor    string
	highlightColor string
	textColor      string
	mutedColor     string
	dangerColor    string
	surfaceColor   string
	inkColor       string

	ruleStyle      lipgloss.Style
	accentStyle    lipgloss.Style
	highlightStyle lipgloss.Style
	textStyle      lipgloss.Style
	dimStyle       lipgloss.Style
	dangerStyle    lipgloss.Style
	flashStyle     lipgloss.Style
	// The caret is the cell bubbles paints in the search field, built the same
	// way it builds it: an accent foreground under `Reverse`, which lands as an
	// accent block with the glyph in the terminal's own background colour.
	// Written out here rather than left to the one field that gets it for free,
	// so the caret does not change identity when you move from the query to a
	// form — the fields draw their own runs, and this is what they draw it as.
	//
	// `Reverse` is safe on a style that only sets a foreground: it swaps this
	// pair, not whatever the run underneath carries, so the caret is the same
	// block over a gold placeholder as over literal text. It is also the one
	// cell in potato whose glyph colour the terminal picks — the price of the
	// two carets being one cell.
	caretStyle lipgloss.Style
)

// boldStyle carries weight and no colour, so it is the one style a palette swap
// leaves alone.
var boldStyle = lipgloss.NewStyle().Bold(true)

// potato paints dark until the terminal says otherwise. A terminal that never
// answers the background query — and many do not — keeps this, which is the
// right way to be wrong: a dark ground is what most of them have.
func init() { applyPalette(darkPalette) }

// applyPalette adopts a palette and rebuilds every style derived from it. The
// styles hold their colours by value, so changing the colour names is not
// enough — a style built before the swap would go on painting the old one.
func applyPalette(p palette) {
	ruleColor, accentColor, highlightColor, textColor = p.rule, p.accent, p.highlight, p.text
	mutedColor, dangerColor, surfaceColor, inkColor = p.muted, p.danger, p.surface, p.ink

	fg := func(hex string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
	}
	ruleStyle = fg(ruleColor)
	accentStyle = fg(accentColor)
	highlightStyle = fg(highlightColor)
	textStyle = fg(textColor)
	dimStyle = fg(mutedColor)
	dangerStyle = fg(dangerColor)
	caretStyle = fg(accentColor).Inline(true).Reverse(true)
	flashStyle = fg(inkColor).Background(lipgloss.Color(accentColor))
}

// contentIndent is the two columns every content row is inset by, so that text
// lands in the same column as a list row's name — the selection pointer
// occupies exactly this much.
const contentIndent = "  "

// contentIndentWidth is what that inset costs a Layout, which is told its
// columns as numbers rather than as strings. Derived rather than written out so
// that the indent and the pointer cannot be widened apart from each other.
const contentIndentWidth = len(contentIndent)

// sectionStyle labels a region: muted, so the label names the rule without
// competing with the content under it.
func sectionStyle() lipgloss.Style { return boldStyle.Foreground(lipgloss.Color(mutedColor)) }

// focusStyle is sectionStyle for the field that has the keyboard. With no
// borders left to carry focus, the rule and its label do it.
func focusStyle() lipgloss.Style { return boldStyle.Foreground(lipgloss.Color(accentColor)) }

// titleStyle names a thing rather than a region — a Command's own name over
// the detail strip, in the same off-white as the content it heads.
func titleStyle() lipgloss.Style { return boldStyle.Foreground(lipgloss.Color(textColor)) }

// hitStyle is what a fuzzy-match hit in a name is painted in: titleStyle lifted
// to the highlight and underlined, so the matched runes read as the same text
// turned up rather than as separate text.
//
// The underline is what carries the mark rather than decorating it. A name and
// its hits differ in hue and barely in luminance, so to a reader who cannot
// separate the two colours the hue alone marks nothing at all.
func hitStyle() lipgloss.Style {
	return boldStyle.Foreground(lipgloss.Color(highlightColor)).Underline(true)
}

// brand is what the header rule is labelled with. The potato is the app's mark
// — the same one the README and the repo lead with — and the one piece of
// colour in the frame that is not potato's own gold, since a terminal paints an
// emoji from the font and ignores the foreground it is given.
const brand = "🥔 Potato"

// brandStyle is the app's own name on the header rule — the one label that is
// neither a region nor a Command, and the only place the accent gold heads a
// rule rather than marking something you can press.
func brandStyle() lipgloss.Style { return boldStyle.Foreground(lipgloss.Color(accentColor)) }

// versionLabel is what the header rule carries at its right end, and all it
// carries: the repo link lives in `potato --help`, which is where you look when
// you want it, while the running version is worth a glance every launch.
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
// Every screen is exactly the session's body height — see Model.bodyHeight. The
// blanks are unframed, so they read as the bottom of a block that holds still
// while you type, in a terminal that would otherwise reflow under it on every
// keystroke.
//
// The status line — whatever a screen pins against the footer, a refusal or a
// warning about the key being held — keeps its rows when the two blocks
// together will not fit. The top block is the one that gives, since it is the
// one with somewhere to scroll.
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

// footerKey is a binding's help text — see footerKeys in keys.go. It is an
// alias rather than a type of its own so a footer row is the binding's own
// words, with no second copy to drift from.
type footerKey = key.Help

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
		b.WriteString(accentStyle.Bold(true).Render(k.Key))
		if labelled {
			b.WriteString(dimStyle.Render(" " + k.Desc))
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
		w += ansi.StringWidth(k.Key) + 1 + ansi.StringWidth(k.Desc)
	}
	return w
}

// ---------- fields ----------

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
		return "Just now"
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
//
// A reverse-video space is kept. It is the caret parked past the last character
// of a field, and it is a painted cell rather than padding: a field's row ends
// at its value, so the caret is the last thing on the line for as long as you
// are typing at the end of one. Trimmed, it left an empty styled run behind and
// drew nothing — the caret was invisible in every edit field, and visible in
// the search field only because the result count sits to its right.
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
		if idx < 0 || toks[idx] != " " || painted(toks[:idx]) {
			break
		}
		toks = append(toks[:idx], toks[idx+1:]...)
	}
	return strings.Join(toks, "")
}

// painted reports whether the run a space belongs to is reverse-video, by
// reading the escape that opened it. lipgloss emits a run's whole style in one
// SGR immediately before it and a reset immediately after, so the sequences
// sitting directly behind the space are that style and nothing else.
func painted(before []string) bool {
	for i := len(before) - 1; i >= 0; i-- {
		if !strings.HasPrefix(before[i], "\x1b") {
			return false
		}
		params, ok := sgrParams(before[i])
		if !ok {
			continue
		}
		for _, p := range params {
			// A reset closes the run behind it, so nothing before it can be
			// what this space is wearing.
			if p == "0" || p == "" {
				return false
			}
			if p == "7" {
				return true
			}
		}
	}
	return false
}

// sgrParams splits a CSI ... m sequence into its parameters, reporting false
// for any other escape.
func sgrParams(tok string) ([]string, bool) {
	if !strings.HasPrefix(tok, "\x1b[") || !strings.HasSuffix(tok, "m") {
		return nil, false
	}
	return strings.Split(strings.TrimSuffix(strings.TrimPrefix(tok, "\x1b["), "m"), ";"), true
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
// reports the row that caret landed on. on is the blink's lit half: on its dark
// half the caret keeps its cell and its column, and only stops being painted.
//
// Editing surfaces wrap this way rather than on word boundaries. Word wrapping
// discards the space it broke on, and a caret sitting on a discarded rune has
// no cell to be drawn in — it would vanish exactly while you were typing at
// the panel's edge. Breaking where the panel ends keeps every rune addressable,
// and suits command text, which is not prose.
func wrapStyledHard(runs []run, width, caret int, on bool) ([]string, int) {
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
		//
		// The cell is taken on the blink's dark half too. Handing it back would
		// shorten the row by a column twice a second, and everything that
		// measures itself against the rendered width — an arg row's fill and
		// its hint — would step sideways in time with the blink.
		if caret >= len(text) {
			if len(runs) == 0 {
				runs = append(runs, run{style: textStyle})
			}
			text = append(text, ' ')
			owner = append(owner, len(runs)-1)
			caret = len(text) - 1
		}
		// Point the caret's rune at a run of its own, so the batching below
		// breaks around it without needing to know it is there. On the dark
		// half it stays with the run underneath, which is what that rune looks
		// like with nothing on it.
		if on {
			owner[caret] = len(runs)
			runs = append(runs, run{style: caretStyle})
		}
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
