package tui

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/luojiahai/potato/internal/search"
)

// The frame goldens are compared de-ANSI'd, so nothing there would notice if
// the colour fell out entirely, or drifted back onto the terminal's palette.
// These assert what the goldens cannot see.

var sgr = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// screens renders every surface potato has, so a colour assertion covers all
// of them rather than whichever one the list screen happens to show.
func screens(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, keys := range map[string][]string{
		"list":    nil,
		"nomatch": {"zzz"},
		"args":    {"enter"},
		"edit":    {"tab", "a"},
		"delete":  {"tab", "d"},
	} {
		m := New(fixtureDeps())
		m.SetSize(80, 24)
		press(m, keys)
		out[name] = m.View().Content
	}
	return out
}

// The search field always has the keyboard, and the glyph and the caret say so
// — neither survives being de-ANSI'd, so the goldens cannot see either. This
// asserts they hold through tab, which must not take the keyboard from it.
func TestTheSearchGlyphStaysLit(t *testing.T) {
	m := New(fixtureDeps())
	m.SetSize(80, 24)
	if !strings.Contains(m.View().Content, focusStyle().Render("⌕ ")) {
		t.Error("the search glyph is not lit while the field has the keyboard")
	}

	press(m, []string{"tab"})
	if !strings.Contains(m.View().Content, focusStyle().Render("⌕ ")) {
		t.Error("the search glyph went out — tab must not take the keyboard from the field")
	}
	caret := strings.SplitN(caretStyle.Render("x"), "x", 2)[0]
	if !strings.Contains(m.View().Content, caret) {
		t.Error("the search field lost its caret")
	}
}

func TestViewCarriesTheBrandColours(t *testing.T) {
	m := New(fixtureDeps())
	m.SetSize(80, 24)
	frame := m.View().Content

	if len(sgr.FindAllString(frame, -1)) == 0 {
		t.Fatal("no SGR sequences in the frame")
	}
	for label, colour := range map[string]string{
		"rule":      ruleColor,
		"accent":    accentColor,
		"text":      textColor,
		"muted":     mutedColor,
		"highlight": highlightColor,
	} {
		if !strings.Contains(frame, truecolorFg(colour)) {
			t.Errorf("the %s colour (%s) is not on the wire", label, colour)
		}
	}
}

// A terminal that reports a light background gets the light palette. The answer
// arrives as a message rather than at startup, so the swap has to reach the
// styles the frame is already being drawn with.
//
// The palette is package state, so it is put back on the way out: every other
// test here reads the dark one, and none of them says so.
func TestALightTerminalGetsTheLightPalette(t *testing.T) {
	t.Cleanup(func() { applyPalette(darkPalette) })

	m := New(fixtureDeps())
	m.SetSize(80, 24)
	send(m, tea.BackgroundColorMsg{Color: color.White})

	frame := m.View().Content
	if !strings.Contains(frame, truecolorFg(lightPalette.text)) {
		t.Errorf("the light palette's command text (%s) is not on the wire", lightPalette.text)
	}
	if strings.Contains(frame, truecolorFg(darkPalette.text)) {
		t.Errorf("the dark palette's command text (%s) survived the swap", darkPalette.text)
	}
}

// Every colour potato draws has to be one it chose. An ANSI palette index is
// whatever the user's terminal theme says it is, so a single one of them
// leaking back in puts part of the palette outside potato's control — and next
// to the fixed golds it can land anywhere.
//
// The caret is the one deliberate exception, and this test cannot see it: it is
// an accent foreground under SGR 7, so the glyph inside the block comes out in
// whatever the terminal calls its default background. That is the cost of the
// fields painting the same cell bubbles paints in the search field, and it was
// chosen. Reverse carries no palette index, so nothing here fires on it.
func TestNoColourIsLeftToTheTerminalTheme(t *testing.T) {
	for name, frame := range screens(t) {
		for _, match := range sgr.FindAllStringSubmatch(frame, -1) {
			params := strings.Split(match[1], ";")
			for i := 0; i < len(params); i++ {
				n, err := strconv.Atoi(params[i])
				if err != nil {
					continue
				}
				switch {
				// 38/48 introduce an explicit colour; skip what they carry so
				// its channel values are not read as palette indices
				case n == 38 || n == 48:
					if i+1 < len(params) && params[i+1] == "2" {
						i += 4 // 2;r;g;b
						continue
					}
					t.Errorf("%s: %q selects a palette colour, want truecolor", name, match[0])
					i = len(params)
				case n >= 30 && n <= 37, n >= 40 && n <= 47,
					n >= 90 && n <= 97, n >= 100 && n <= 107:
					t.Errorf("%s: %q is ANSI index %d — the terminal theme picks that colour", name, match[0], n)
				}
			}
		}
	}
}

func truecolorFg(hex string) string {
	var r, g, b int
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

// Fuzzy-match hits are painted rune by rune from the positions search hands
// back — a map keyed by rune index, one entry per hit. Nothing else in the
// suite can see whether they land: the frame goldens are compared de-ANSI'd,
// and TestViewCarriesTheBrandColours only asks whether highlightColor is on
// the wire at all, which it is either way because Placeholders wear it too.
// So this walks the seam itself and asks which runes came back lit.
func TestEveryNameMatchIsHighlighted(t *testing.T) {
	lit := hitStyle().GetForeground()
	for _, tc := range []struct{ query, name string }{
		{"li", "list ports"},   // hits at the front
		{"port", "list ports"}, // hits past the number of hits
		{"y", "deploy prod"},   // one hit, well past it
		{"dp", "deploy prod"},  // one at the front, one past
		{"tl", "tail logs"},
	} {
		want, ok := search.NameMatchIndices(tc.query, tc.name)
		if !ok {
			t.Fatalf("%q does not match %q — the fixture is wrong", tc.query, tc.name)
		}
		runs := nameRuns(tc.query, tc.name)
		if len(runs) != len([]rune(tc.name)) {
			t.Fatalf("nameRuns(%q, %q) returned %d runs, want one per rune (%d)",
				tc.query, tc.name, len(runs), len([]rune(tc.name)))
		}
		for i, r := range runs {
			if got := r.style.GetForeground() == lit; got != want[i] {
				t.Errorf("nameRuns(%q, %q): rune %d (%q) lit=%v, want %v",
					tc.query, tc.name, i, r.text, got, want[i])
			}
		}
	}
}
