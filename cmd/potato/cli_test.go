package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// CLI seams: `potato import` (merge / override) run as a real subprocess
// against a POTATO_INSTALL temp dir, plus `potato init` against the goldens
// captured from the Ink build.

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "potato-bin-")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "potato")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	home     string
	exitCode int
	stdout   string
	stderr   string
}

func run(t *testing.T, args []string, home string, stdin string) result {
	t.Helper()
	if home == "" {
		home = t.TempDir()
	}
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "POTATO_INSTALL="+home)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("running potato: %v", err)
	}
	return result{home: home, exitCode: code, stdout: stdout.String(), stderr: stderr.String()}
}

type command struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Template    string `json:"command"`
}

func v2(commands ...command) string {
	if commands == nil {
		commands = []command{}
	}
	raw, err := json.Marshal(map[string]any{"version": 2, "commands": commands})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readLib(t *testing.T, home string) struct {
	Version  int       `json:"version"`
	Commands []command `json:"commands"`
} {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lib struct {
		Version  int       `json:"version"`
		Commands []command `json:"commands"`
	}
	if err := json.Unmarshal(raw, &lib); err != nil {
		t.Fatalf("commands.json is not JSON: %v", err)
	}
	return lib
}

func find(commands []command, name string) *command {
	for i := range commands {
		if commands[i].Name == name {
			return &commands[i]
		}
	}
	return nil
}

func TestImportAddsNewNames(t *testing.T) {
	incoming := filepath.Join(t.TempDir(), "theirs.json")
	writeFile(t, incoming, v2(command{ID: "t1", Name: "hello", Template: "echo hi"}))

	got := run(t, []string{"import", incoming}, "", "")
	if got.exitCode != 0 {
		t.Fatalf("exit %d: %s", got.exitCode, got.stderr)
	}
	if !strings.Contains(got.stdout, "added: hello") {
		t.Errorf("stdout = %q", got.stdout)
	}
	lib := readLib(t, got.home)
	if lib.Version != 2 {
		t.Errorf("version = %d", lib.Version)
	}
	hello := find(lib.Commands, "hello")
	if hello == nil || hello.Template != "echo hi" {
		t.Fatalf("hello = %+v", hello)
	}
	if hello.ID == "t1" {
		t.Error("the incoming id was kept; a fresh one must be minted")
	}
}

func TestImportCollisionKeepsBoth(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "commands.json"), v2(command{ID: "o1", Name: "x", Template: "ours"}))
	incoming := filepath.Join(t.TempDir(), "theirs.json")
	writeFile(t, incoming, v2(command{ID: "t1", Name: "x", Template: "theirs"}))

	got := run(t, []string{"import", incoming}, home, "")
	if got.exitCode != 0 {
		t.Fatalf("exit %d: %s", got.exitCode, got.stderr)
	}
	if !strings.Contains(got.stdout, "kept both: x → x (1)") {
		t.Errorf("stdout = %q", got.stdout)
	}
	lib := readLib(t, home)
	if e := find(lib.Commands, "x"); e == nil || e.Template != "ours" {
		t.Errorf("ours = %+v", e)
	}
	if e := find(lib.Commands, "x (1)"); e == nil || e.Template != "theirs" {
		t.Errorf("theirs = %+v", e)
	}
}

func TestImportNothingToImport(t *testing.T) {
	got := run(t, []string{"import", "-"}, "", v2())
	if got.exitCode != 0 {
		t.Fatalf("exit %d: %s", got.exitCode, got.stderr)
	}
	if !strings.Contains(got.stdout, "nothing to import") {
		t.Errorf("stdout = %q", got.stdout)
	}
}

func TestImportReadsStdin(t *testing.T) {
	got := run(t, []string{"import", "-"}, "", v2(command{ID: "t1", Name: "piped", Template: "echo pipe"}))
	if got.exitCode != 0 {
		t.Fatalf("exit %d: %s", got.exitCode, got.stderr)
	}
	if e := find(readLib(t, got.home).Commands, "piped"); e == nil || e.Template != "echo pipe" {
		t.Errorf("piped = %+v", e)
	}
}

func TestImportOverrideReplacesWholesale(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "commands.json"), v2(command{ID: "o1", Name: "mine", Template: "keep?"}))
	incoming := filepath.Join(t.TempDir(), "theirs.json")
	writeFile(t, incoming, v2(command{ID: "t1", Name: "theirs", Template: "echo t"}))

	got := run(t, []string{"import", incoming, "--override"}, home, "")
	if got.exitCode != 0 {
		t.Fatalf("exit %d: %s", got.exitCode, got.stderr)
	}
	if !strings.Contains(got.stdout, "replaced your Library") {
		t.Errorf("stdout = %q", got.stdout)
	}
	lib := readLib(t, home)
	if len(lib.Commands) != 1 || lib.Commands[0].Name != "theirs" {
		t.Fatalf("commands = %+v", lib.Commands)
	}
	if lib.Commands[0].ID != "t1" {
		t.Error("--override must keep the incoming ids as-is")
	}
}

func TestImportInvalidFileAbortsAllOrNothing(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "commands.json"), v2(command{ID: "o1", Name: "keep", Template: "ls"}))
	incoming := filepath.Join(t.TempDir(), "bad.json")
	writeFile(t, incoming, `{"version": 99, "commands": []}`)

	got := run(t, []string{"import", incoming}, home, "")
	if got.exitCode == 0 {
		t.Fatal("expected a non-zero exit")
	}
	if !strings.Contains(got.stderr, "bad.json") {
		t.Errorf("stderr = %q", got.stderr)
	}
	if e := find(readLib(t, home).Commands, "keep"); e == nil || e.Template != "ls" {
		t.Error("the library was modified by a failed import")
	}
}

func TestImportRejectsAV1IncomingFile(t *testing.T) {
	incoming := filepath.Join(t.TempDir(), "old.json")
	writeFile(t, incoming, `{"version":1,"commands":{"legacy":{"command":"ls"}}}`)

	got := run(t, []string{"import", incoming}, "", "")
	if got.exitCode == 0 {
		t.Fatal("expected a non-zero exit")
	}
	if !strings.Contains(strings.ToLower(got.stderr), "unsupported version 1") {
		t.Errorf("stderr = %q", got.stderr)
	}
}

// A v1 library of our own is refused rather than upgraded. No released version
// of potato ever wrote one — v2 landed before v0.1.1, the first tag — so the
// upgrade path was serving a file that cannot exist. See
// docs/adr/0001-reject-v1-libraries.md. It must fail loud rather than silently
// treat the library as empty, which would look exactly like data loss.
func TestOwnV1LibraryIsRefusedNotMigrated(t *testing.T) {
	home := t.TempDir()
	v1 := `{"version":1,"commands":{"legacy":{"command":"echo old"}}}`
	writeFile(t, filepath.Join(home, "commands.json"), v1)
	incoming := filepath.Join(t.TempDir(), "theirs.json")
	writeFile(t, incoming, v2(command{ID: "t1", Name: "fresh", Template: "echo new"}))

	got := run(t, []string{"import", incoming}, home, "")
	if got.exitCode == 0 {
		t.Fatal("expected a non-zero exit")
	}
	if !strings.Contains(got.stderr, "unsupported version 1") {
		t.Errorf("stderr = %q", got.stderr)
	}

	// The v1 file is left exactly as it was, so nothing has been destroyed and
	// the user can convert or delete it themselves.
	raw, err := os.ReadFile(filepath.Join(home, "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != v1 {
		t.Errorf("the v1 library was rewritten:\n%s", raw)
	}
}

// The installer regenerates the shell glue from the binary on every install
// and every update, so any drift silently rewrites every user's rc hook.
func TestInitMatchesTheInkGoldens(t *testing.T) {
	for _, shell := range []string{"zsh", "bash", "sh"} {
		t.Run(shell, func(t *testing.T) {
			got := run(t, []string{"init", shell}, "/POTATO", "")
			if got.exitCode != 0 {
				t.Fatalf("exit %d: %s", got.exitCode, got.stderr)
			}
			want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "init", "init."+shell))
			if err != nil {
				t.Fatal(err)
			}
			if got.stdout != string(want) {
				t.Errorf("init %s differs from the golden:\ngot:\n%s\nwant:\n%s", shell, got.stdout, want)
			}
		})
	}
}

func TestInitRejectsAnUnknownShell(t *testing.T) {
	got := run(t, []string{"init", "fish"}, "", "")
	if got.exitCode == 0 {
		t.Fatal("expected a non-zero exit")
	}
	if !strings.Contains(got.stderr, "usage: potato init <zsh|bash|sh>") {
		t.Errorf("stderr = %q", got.stderr)
	}
}

func TestUnknownCommandPrintsUsageAndExitsOne(t *testing.T) {
	got := run(t, []string{"nope"}, "", "")
	if got.exitCode != 1 {
		t.Errorf("exit = %d, want 1", got.exitCode)
	}
	if !strings.Contains(got.stderr, "unknown command 'nope'") {
		t.Errorf("stderr = %q", got.stderr)
	}
	if !strings.Contains(got.stdout, "usage:") {
		t.Errorf("stdout = %q", got.stdout)
	}
}

func TestVersionAndHelp(t *testing.T) {
	if got := run(t, []string{"--version"}, "", ""); strings.TrimSpace(got.stdout) != "dev" {
		t.Errorf("--version = %q", got.stdout)
	}
	if got := run(t, []string{"--help"}, "", ""); !strings.Contains(got.stdout, "open the TUI") {
		t.Errorf("--help = %q", got.stdout)
	}
}

// The TUI needs a terminal; a piped stdin must fail loudly rather than hang.
func TestTUIWithoutATerminal(t *testing.T) {
	got := run(t, nil, "", "\n")
	if got.exitCode == 0 {
		t.Fatal("expected a non-zero exit")
	}
	if !strings.Contains(got.stderr, "the potato TUI needs a terminal") {
		t.Errorf("stderr = %q", got.stderr)
	}
}
