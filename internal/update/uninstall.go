// potato uninstall (spec §8): remove the rc line, delete the binary and
// generated init files, keep user data (--purge wipes everything).

package update

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/luojiahai/potato/internal/paths"
)

// RemoveInitLines drops any line that sources a potato init file; everything
// else is unchanged.
func RemoveInitLines(content string, needles []string) string {
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	for _, line := range lines {
		drop := false
		for _, needle := range needles {
			if strings.Contains(line, needle) {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func RunUninstall(args []string) error {
	purge := slices.Contains(args, "--purge")
	needles := []string{".potato/init.", filepath.Join(paths.Potato(), "init.")}

	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	for _, rc := range []string{filepath.Join(home, ".zshrc"), filepath.Join(home, ".bashrc")} {
		raw, err := os.ReadFile(rc)
		if err != nil {
			continue
		}
		content := string(raw)
		cleaned := RemoveInitLines(content, needles)
		if cleaned != content {
			if err := os.WriteFile(rc, []byte(cleaned), 0o644); err != nil {
				return err
			}
			fmt.Printf("removed potato line from %s\n", rc)
		}
	}

	if purge {
		if err := os.RemoveAll(paths.Potato()); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", paths.Potato())
		return nil
	}

	if err := os.RemoveAll(paths.BinDir()); err != nil {
		return err
	}
	for _, shell := range []string{"zsh", "bash", "sh"} {
		_ = os.Remove(paths.Init(shell))
	}
	fmt.Println("potato uninstalled.")
	fmt.Printf("your data is kept at %s and %s\n", paths.Commands(), paths.State())
	fmt.Println("run with --purge to remove it too.")
	return nil
}
