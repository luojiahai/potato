package tui

import (
	"regexp"
	"strings"
	"testing"
)

// The parity frames are compared de-ANSI'd, so nothing there would notice if
// the colour fell out entirely. These assert the brand golds are still on the
// wire: the muted frame colour and the brighter control accent.

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestViewCarriesTheBrandColours(t *testing.T) {
	m := New(fixtureDeps())
	m.SetSize(80, 24)
	frame := m.View().Content

	if len(sgr.FindAllString(frame, -1)) == 0 {
		t.Fatal("no SGR sequences in the frame")
	}
	for label, code := range map[string]string{
		"frame #d78700":  "\x1b[38;2;215;135;0m",
		"accent #ffaf5f": "\x1b[38;2;255;175;95m",
	} {
		if !strings.Contains(frame, code) {
			t.Errorf("missing the %s colour", label)
		}
	}
}

// The version line is an OSC 8 hyperlink; the layout only survives because the
// width measurement ignores the escape sequence.
func TestBannerCarriesAnOSC8Hyperlink(t *testing.T) {
	m := New(fixtureDeps())
	m.SetSize(80, 24)
	if !strings.Contains(m.View().Content, "\x1b]8;;https://github.com/luojiahai/potato\a") {
		t.Error("the repo link is not an OSC 8 hyperlink")
	}
}
