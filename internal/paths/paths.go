// Package paths locates potato's entire footprint under ~/.potato (spec
// §6.2); POTATO_INSTALL overrides the root.
package paths

import (
	"os"
	"path/filepath"
)

func Potato() string {
	if dir := os.Getenv("POTATO_INSTALL"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return filepath.Join(home, ".potato")
}

func Commands() string { return filepath.Join(Potato(), "commands.json") }
func State() string    { return filepath.Join(Potato(), "state.json") }
func BinDir() string   { return filepath.Join(Potato(), "bin") }
func Bin() string      { return filepath.Join(BinDir(), "potato") }

// Init is the path of the generated shell glue for one shell.
func Init(shell string) string { return filepath.Join(Potato(), "init."+shell) }
