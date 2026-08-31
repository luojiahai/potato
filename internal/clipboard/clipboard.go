// Package clipboard implements Copy (spec §4.2): spawn the native clipboard
// tool if present AND always emit OSC 52 — the only mechanism that works over
// SSH and inside tmux.
package clipboard

import (
	"encoding/base64"
	"os"
	"os/exec"
	"strings"
)

var nativeTools = [][]string{
	{"pbcopy"},
	{"wl-copy"},
	{"xclip", "-selection", "clipboard"},
}

// Copy puts text on the clipboard and reports whether a native tool took it.
//
// The two mechanisms differ in what they can be known to have done. A native
// tool either ran or returned an error. OSC 52 is written at the terminal and
// never answered — it is honoured, ignored, or stripped by something in between,
// with nothing to say which — so the caller is told which of the two it got,
// and can phrase what it tells the user accordingly.
func Copy(text string) bool {
	native := false
	for _, tool := range nativeTools {
		cmd := exec.Command(tool[0], tool[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			native = true
			break
		}
	}
	os.Stdout.WriteString("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a")
	return native
}
