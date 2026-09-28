// Package atomicfile is how potato writes a file: a reader sees the old
// contents or the new ones, never a partial write, and a write that fails
// leaves the old contents where they were.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write replaces the file at path with data, creating its directory if need
// be. The bytes go to a temp file beside the target, which is synced and then
// renamed over it; a rename within one directory is atomic.
//
// A path that is a symlink is written through: the file it points at is
// replaced and the link is left a link, so a library kept in a dotfiles repo
// and linked into ~/.potato stays in the repo.
func Write(path string, data []byte, perm os.FileMode) (err error) {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
