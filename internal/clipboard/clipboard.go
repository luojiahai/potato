// Package clipboard hands text to the platform's own clipboard tool: the half
// of a copy that can say whether it worked. The other half is OSC 52, which
// the TUI sends through Bubble Tea on every copy — the only mechanism that
// reaches a clipboard over SSH and inside tmux, and one nothing answers.
package clipboard

import (
	"os/exec"
	"strings"
)

var nativeTools = [][]string{
	{"pbcopy"},
	{"wl-copy"},
	{"xclip", "-selection", "clipboard"},
}

// Copy runs the first native clipboard tool that takes the text, and reports
// whether one did.
func Copy(text string) bool {
	for _, tool := range nativeTools {
		cmd := exec.Command(tool[0], tool[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}
