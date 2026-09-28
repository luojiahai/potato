package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

// entries lists a directory, so a test can see a temp file left behind.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(list))
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func TestWriteCreatesTheFileAndItsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "commands.json")
	if err := Write(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "hello\n" {
		t.Errorf("contents = %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestWriteReplacesAndLeavesNoTempBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	for _, content := range []string{"first", "second"} {
		if err := Write(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := read(t, path); got != "second" {
		t.Errorf("contents = %q, want the second write", got)
	}
	if got := entries(t, dir); len(got) != 1 {
		t.Errorf("directory holds %v, want only state.json", got)
	}
}

func TestWriteSetsTheModeItIsGiven(t *testing.T) {
	path := filepath.Join(t.TempDir(), "potato")
	if err := Write(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestWriteThroughASymlinkKeepsTheLink(t *testing.T) {
	dotfiles := t.TempDir()
	real := filepath.Join(dotfiles, "commands.json")
	if err := os.WriteFile(real, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "commands.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	if err := Write(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := read(t, real); got != "new" {
		t.Errorf("the file the link points at = %q, want the new contents", got)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced by a regular file")
	}
}

// A rename onto a directory fails after the temp file is written, which is the
// latest a write can fail. The target stays as it was and the temp is gone.
func TestAFailedWriteLeavesTheTargetAndNoTemp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "commands.json")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Write(target, []byte("new"), 0o644); err == nil {
		t.Fatal("writing over a non-empty directory succeeded")
	}
	if got := read(t, filepath.Join(target, "keep")); got != "x" {
		t.Errorf("the target was disturbed: %q", got)
	}
	if got := entries(t, dir); len(got) != 1 {
		t.Errorf("directory holds %v, want only the target", got)
	}
}
