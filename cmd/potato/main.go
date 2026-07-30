// potato CLI (spec §5): the TUI by default (--out <file> carries the
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

	// The saves hand their error back rather than swallowing it, so the TUI can
	// say a write failed instead of flashing "Saved" over one that did not.
	handoff, err := tui.Run(tui.Deps{
		Library:     lib,
		State:       state.Load(paths.State()),
		SaveLibrary: func(lib library.Library) error { return library.Save(paths.Commands(), lib) },
		SaveState:   func(s state.State) error { return state.Save(paths.State(), s) },
		Copy:        clipboard.Copy,
		Now:         time.Now,
	})
	if err != nil {
		die(err.Error())
	}

	if hasOut {
		// empty file = cancelled (spec §4.1)
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

func runImport(args []string) {
	override := slices.Contains(args, "--override")
	if override && slices.Contains(args, "--merge") {
		die("choose one of --merge or --override, not both")
	}
	file := ""
	for _, arg := range args {
		if arg == "-" || !strings.HasPrefix(arg, "--") {
			file = arg
			break
		}
	}
	if file == "" {
		die("usage: potato import <file|-> [--merge | --override]")
	}

	source := file
	var text []byte
	var err error
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
	// So does a v1 library of our own — potato has never released a version that
	// wrote one. See docs/adr/0001-reject-v1-libraries.md.
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

	result := importer.Merge(ours, theirs)
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
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	script, ok := shell.Script(name, paths.Bin(), paths.Potato())
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
		if err := update.Run(); err != nil {
			die(err.Error())
		}
	case "uninstall":
		if err := update.RunUninstall(rest); err != nil {
			die(err.Error())
		}
	case "--version", "-v":
		fmt.Println(version.Version)
	case "--help", "-h":
		os.Stdout.WriteString(usage())
	default:
		fmt.Fprintf(os.Stderr, "potato: unknown command '%s'\n\n", cmd)
		os.Stdout.WriteString(usage())
		os.Exit(1)
	}
}
