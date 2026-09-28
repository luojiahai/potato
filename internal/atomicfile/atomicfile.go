// Package atomicfile is how potato writes a file: a reader sees the old
// contents or the new ones, never a partial write, and a write that fails
// leaves the old contents where they were.
package atomicfile

import (
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
)

// Write replaces the file at path with data, creating its directory if need
// be. The bytes go to a temp file beside the target, which is synced and then
// renamed over it; a rename within one directory is atomic.
//
// A path that is a symlink is written through: the file it points at is
// replaced, or created if it does not exist yet, and the link is left a link.
//
// A file that exists keeps its mode. A new one gets perm less the umask, as
// any file potato creates would.
//
// A failure is reported against path, the file the caller asked for, and never
// against the temp file.
func Write(path string, data []byte, perm os.FileMode) (err error) {
	defer func(named string) {
		if err != nil {
			err = &fs.PathError{Op: "write", Path: named, Err: cause(err)}
		}
	}(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	path = resolve(path)
	tmp, err := create(path, perm)
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
	if info, statErr := os.Stat(path); statErr == nil {
		if err = tmp.Chmod(info.Mode().Perm()); err != nil {
			return err
		}
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// resolve follows a symlink to the path it points at, whether or not anything
// is there yet.
func resolve(path string) string {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		return target
	}
	target, err := os.Readlink(path)
	if err != nil {
		return path
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return target
}

// create opens a new temp file beside path. It is opened with perm rather than
// chmodded to it afterwards, so the kernel applies the umask.
func create(path string, perm os.FileMode) (*os.File, error) {
	for {
		name := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"."+strconv.FormatUint(rand.Uint64(), 36))
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if !errors.Is(err, fs.ErrExist) {
			return f, err
		}
	}
}

func cause(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr.Err
	}
	return err
}
