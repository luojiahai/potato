// Package update implements potato update (spec §8): CLI-only, latest-only.
// It verifies sha256 against the released SHA256SUMS with install.sh's rigor,
// atomically renames over the running binary's realpath, and regenerates init
// files. It never touches the rc.
package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/luojiahai/potato/internal/paths"
	"github.com/luojiahai/potato/internal/shell"
	"github.com/luojiahai/potato/internal/version"
)

const Repo = "luojiahai/potato"

var sumsLine = regexp.MustCompile(`^([0-9a-fA-F]+)\s+\*?(.+)$`)

func ParseSha256Sums(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		if m := sumsLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			out[m[2]] = m[1]
		}
	}
	return out
}

// TargetTriple names the release asset for a platform. Go's own arch name is
// amd64, but the published assets have always said x64 and installed binaries
// resolve them by that name — so the wire name is frozen.
func TargetTriple(goos, goarch string) (string, error) {
	arch := goarch
	if arch == "amd64" {
		arch = "x64"
	}
	if (goos == "darwin" || goos == "linux") && (arch == "x64" || arch == "arm64") {
		return goos + "-" + arch, nil
	}
	return "", fmt.Errorf("unsupported platform: %s-%s", goos, goarch)
}

func latestTag() (string, error) {
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := client.Get(fmt.Sprintf("https://github.com/%s/releases/latest", Repo))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	location := res.Header.Get("location")
	m := regexp.MustCompile(`/tag/([^/]+)$`).FindStringSubmatch(location)
	if m == nil {
		return "", fmt.Errorf("could not resolve the latest release from github.com/%s", Repo)
	}
	return m[1], nil
}

func download(url string) ([]byte, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed (%d): %s", res.StatusCode, url)
	}
	return io.ReadAll(res.Body)
}

func Run() error {
	if version.Version == "dev" {
		return fmt.Errorf("update only works on an installed release build")
	}
	tag, err := latestTag()
	if err != nil {
		return err
	}
	latest := strings.TrimPrefix(tag, "v")
	if latest == version.Version {
		fmt.Printf("potato %s is already up to date.\n", version.Version)
		return nil
	}

	triple, err := TargetTriple(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	asset := fmt.Sprintf("potato-%s.tar.gz", triple)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", Repo, tag)
	fmt.Printf("updating potato %s → %s…\n", version.Version, latest)

	archive, err := download(base + "/" + asset)
	if err != nil {
		return err
	}
	sumsText, err := download(base + "/SHA256SUMS")
	if err != nil {
		return err
	}

	expected, ok := ParseSha256Sums(string(sumsText))[asset]
	if !ok {
		return fmt.Errorf("SHA256SUMS has no entry for %s", asset)
	}
	sum := sha256.Sum256(archive)
	actual := hex.EncodeToString(sum[:])
	if actual != strings.ToLower(expected) {
		return fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", asset, expected, actual)
	}

	work, err := os.MkdirTemp("", "potato-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	extracted := filepath.Join(work, "potato")
	if err := extractBinary(archive, extracted); err != nil {
		return err
	}
	if err := os.Chmod(extracted, 0o755); err != nil {
		return err
	}

	// Atomic swap over the RUNNING binary's realpath (spec §8) — the
	// executable path, not the env-derived install dir, which may differ in
	// this shell; stage next to the target first so the rename never crosses
	// filesystems.
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	target, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	staged := filepath.Join(filepath.Dir(target), ".potato.new")
	if err := copyFile(extracted, staged); err != nil {
		return err
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		return err
	}
	if err := shell.WriteInitFiles(paths.Bin(), paths.Potato()); err != nil {
		return err
	}
	fmt.Printf("potato %s installed at %s\n", latest, target)
	return nil
}

// extractBinary pulls the single `potato` entry out of the release tarball.
func extractBinary(archive []byte, dest string) error {
	gz, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return fmt.Errorf("tar failed: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return fmt.Errorf("tar failed: no potato binary in the archive")
		}
		if err != nil {
			return fmt.Errorf("tar failed: %w", err)
		}
		if filepath.Base(header.Name) != "potato" || header.Typeflag != tar.TypeReg {
			continue
		}
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, reader)
		return err
	}
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o755)
}
