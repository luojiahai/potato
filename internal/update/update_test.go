package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestParseSha256Sums(t *testing.T) {
	text := strings.Join([]string{
		"abc123  potato-darwin-arm64.tar.gz",
		"def456  potato-linux-x64.tar.gz",
		"",
	}, "\n")
	got := ParseSha256Sums(text)
	if got["potato-darwin-arm64.tar.gz"] != "abc123" || got["potato-linux-x64.tar.gz"] != "def456" {
		t.Errorf("got %v", got)
	}
	if len(got) != 2 {
		t.Errorf("got %d entries, want 2", len(got))
	}
}

// The published asset names are frozen: installed binaries resolve them by
// these exact strings, so Go's amd64 must keep going out as x64.
func TestTargetTriple(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"darwin", "arm64", "darwin-arm64"},
		{"darwin", "amd64", "darwin-x64"},
		{"linux", "amd64", "linux-x64"},
		{"linux", "arm64", "linux-arm64"},
	}
	for _, tc := range cases {
		got, err := TargetTriple(tc.goos, tc.goarch)
		if err != nil {
			t.Errorf("%s/%s: %v", tc.goos, tc.goarch, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s/%s = %q, want %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
	if _, err := TargetTriple("windows", "amd64"); err == nil {
		t.Error("expected an error for an unsupported platform")
	}
}

func TestRemoveInitLines(t *testing.T) {
	rc := strings.Join([]string{"# my rc", `alias ll="ls -l"`, "source ~/.potato/init.zsh", "export FOO=1", ""}, "\n")
	want := strings.Join([]string{"# my rc", `alias ll="ls -l"`, "export FOO=1", ""}, "\n")
	if got := RemoveInitLines(rc, []string{".potato/init."}); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRemoveInitLinesLeavesOtherRcsAlone(t *testing.T) {
	rc := "# nothing potato here\n"
	if got := RemoveInitLines(rc, []string{".potato/init."}); got != rc {
		t.Errorf("got %q, want it unchanged", got)
	}
}

// tarball builds a release-shaped .tar.gz from name → contents.
func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBinaryFindsThePotatoEntry(t *testing.T) {
	archive := tarball(t, map[string]string{"README": "not me", "potato": "the binary"})
	got, err := extractBinary(archive)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "the binary" {
		t.Errorf("extracted %q, want the potato entry", got)
	}
}

func TestExtractBinaryRefusesAnArchiveWithoutOne(t *testing.T) {
	if _, err := extractBinary(tarball(t, map[string]string{"README": "no binary here"})); err == nil {
		t.Error("an archive with no potato entry was accepted")
	}
	if _, err := extractBinary([]byte("not a tarball")); err == nil {
		t.Error("bytes that are not a tarball were accepted")
	}
}
