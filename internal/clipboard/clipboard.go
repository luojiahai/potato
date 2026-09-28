// Package clipboard hands text to the platform's own clipboard tool.
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
