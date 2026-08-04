package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
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
// asserts they hold through the keystrokes that used to blur the field.
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
