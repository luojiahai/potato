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

func Copy(text string) {
	for _, tool := range nativeTools {
		cmd := exec.Command(tool[0], tool[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			break
		}
	}
	os.Stdout.WriteString("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a")
}
