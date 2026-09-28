// potato CLI: the TUI by default (--out <file> carries the
// selection back to the shell wrapper), plus import / init / update /
// uninstall subcommands.
package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/luojiahai/potato/internal/clipboard"
	"github.com/luojiahai/potato/internal/importer"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/paths"
	"github.com/luojiahai/potato/internal/shell"
	"github.com/luojiahai/potato/internal/state"
	"github.com/luojiahai/potato/internal/tui"
	"github.com/luojiahai/potato/internal/update"
	"github.com/luojiahai/potato/internal/version"
)

func usage() string {
	return fmt.Sprintf(`potato %s — save, find, and hand off long terminal commands

usage:
  potato                       open the TUI (Enter = run, Ctrl-Y = copy)
  potato --out <file>          TUI; write the selection to <file> (shell glue)
  potato import <file|-> [--merge | --override]
                               merge another library in (--merge, the default,
                               keeps both on a name clash; --override replaces yours)
  potato update                update to the latest release
  potato uninstall [--purge]   remove potato (keep data; --purge wipes it)
  potato init <zsh|bash|sh>    print shell integration (used by the installer)
  potato --version             print the version

https://github.com/%s
`, version.Version, update.Repo)
}

func die(message string) {
	fmt.Fprintf(os.Stderr, "potato: %s\n", message)
	os.Exit(1)
}

func runTUI(outFile string, hasOut bool) {
	if !term.IsTerminal(os.Stdin.Fd()) {
		die("the potato TUI needs a terminal")
	}
	lib, err := library.Load(paths.Commands())
	if err != nil {
		die(err.Error())
	}

	handoff, err := tui.Run(tui.Deps{
		Library: lib,
		State:   state.Load(paths.State()),
		ChangeLibrary: func(change func(library.Library) (library.Library, error)) (library.Library, error) {
			return library.Change(paths.Commands(), change)
		},
		ChangeState: func(change func(state.State) state.State) (state.State, error) {
			return state.Change(paths.State(), change)
		},
		Copy: clipboard.Copy,
		Now:  time.Now,
	})
	if err != nil {
		die(err.Error())
	}

	if hasOut {
		// empty file = cancelled
		if err := os.WriteFile(outFile, []byte(handoff), 0o644); err != nil {
			die(err.Error())
		}
		return
	}
	if handoff != "" {
		// run outside the shell wrapper: can't pre-fill the prompt, so print
		fmt.Println(handoff)
	}
}

// importArgs reads `potato import`'s arguments: one file, or - for stdin, and
// at most one of --merge and --override. Anything else is refused rather than
// guessed at, so a mistyped --override cannot turn into a merge.
func importArgs(args []string) (file string, override bool, err error) {
	merge := false
	for _, arg := range args {
		switch {
		case arg == "--merge":
			merge = true
		case arg == "--override":
			override = true
		case arg != "-" && strings.HasPrefix(arg, "-"):
			return "", false, fmt.Errorf("unknown flag '%s'", arg)
		case file != "":
			return "", false, fmt.Errorf("one file at a time, not '%s' and '%s'", file, arg)
		default:
			file = arg
		}
	}
	if merge && override {
		return "", false, fmt.Errorf("choose one of --merge or --override, not both")
	}
	if file == "" {
		return "", false, fmt.Errorf("no file to import")
	}
	return file, override, nil
}

func runImport(args []string) {
	file, override, err := importArgs(args)
	if err != nil {
		die(err.Error() + "\nusage: potato import <file|-> [--merge | --override]")
	}

	source := file
	var text []byte
	if file == "-" {
		source = "stdin"
		text, err = io.ReadAll(os.Stdin)
	} else {
		text, err = os.ReadFile(file)
	}
	if err != nil {
		die(fmt.Sprintf("cannot read %s: %s", source, err))
	}

	// Version-strict: a v1 incoming file fail-louds ("unsupported version 1").
	// So does a v1 library of our own — potato writes only v2, so a v1 file of
	// its own cannot exist.
	theirs, err := library.Parse(string(text), source)
	if err != nil {
		die(err.Error())
	}

	if override {
		// Replace wholesale: the imported file becomes the Library as-is (its
		// ids kept). Any prior state.json entries are harmless orphans.
		if err := library.Save(paths.Commands(), theirs); err != nil {
			die(err.Error())
		}
		n := len(theirs.Commands)
		plural := "s"
		if n == 1 {
			plural = ""
		}
		fmt.Printf("replaced your Library with %s (%d command%s)\n", source, n, plural)
		return
	}

	ours, err := library.Load(paths.Commands())
	if err != nil {
		die(err.Error())
	}

	result, err := importer.Merge(ours, theirs)
	if err != nil {
		die(err.Error())
	}
	if err := library.Save(paths.Commands(), result.Merged); err != nil {
		die(err.Error())
	}

	if len(result.Added) > 0 {
		fmt.Printf("added: %s\n", strings.Join(result.Added, ", "))
	}
	if len(result.Renamed) > 0 {
		pairs := make([]string, 0, len(result.Renamed))
		for _, r := range result.Renamed {
			pairs = append(pairs, fmt.Sprintf("%s → %s", r.From, r.To))
		}
		fmt.Printf("kept both: %s\n", strings.Join(pairs, ", "))
	}
	if len(result.Added) == 0 && len(result.Renamed) == 0 {
		fmt.Println("nothing to import")
	}
}

func runInit(args []string) {
	if len(args) != 1 {
		die("usage: potato init <zsh|bash|sh>")
	}
	script, ok := shell.Script(args[0], paths.Bin(), paths.Potato())
	if !ok {
		die("usage: potato init <zsh|bash|sh>")
	}
	os.Stdout.WriteString(script)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		runTUI("", false)
		return
	}
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "--out":
		if len(rest) == 0 || rest[0] == "" {
			die("--out needs a file path")
		}
		runTUI(rest[0], true)
	case "import":
		runImport(rest)
	case "init":
		runInit(rest)
	case "update":
		if len(rest) > 0 {
			die("usage: potato update")
		}
		if err := update.Run(); err != nil {
			die(err.Error())
		}
	case "uninstall":
		purge := slices.Equal(rest, []string{"--purge"})
		if !purge && len(rest) > 0 {
			die("usage: potato uninstall [--purge]")
		}
		if err := update.RunUninstall(purge); err != nil {
			die(err.Error())
		}
	case "--version", "-v":
		fmt.Println(version.Version)
	case "--help", "-h":
		os.Stdout.WriteString(usage())
	default:
		fmt.Fprintf(os.Stderr, "potato: unknown command '%s'\n\n%s", cmd, usage())
		os.Exit(1)
	}
}
