package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/state"
)

// Frame goldens. testdata/frames/*.txt are the rendered screens at fixed
// terminal sizes with a fixed clock; every frame here must reproduce one byte
// for byte, de-ANSI'd.
//
// These began as captures from the Ink build, held to prove the Go rewrite
// matched the TypeScript TUI it replaced. That job is done — the goldens were
// re-baselined onto the Go renderer when the screens were redesigned, and they
// now guard the Go build against unintended layout drift rather than fidelity
// to a deleted implementation. Regenerate with `go test ./internal/tui
// -update-frames` after a deliberate visual change, and read the diff.

func fixtureDeps() Deps {
	description := func(s string) *string { return &s }
	return Deps{
		Library: library.Library{
			Version: 2,
			Commands: []library.Entry{
				{
					ID: "id-deploy", Name: "deploy prod",
					Description: description("Roll out to production"),
					Command:     "ssh {{host=prod-1}} 'deploy.sh'",
				},
				{
					ID: "id-ports", Name: "list ports",
					Description: description("Show listening processes"),
					Command:     "lsof -iTCP -sTCP:LISTEN",
				},
				{ID: "id-tail", Name: "tail logs", Command: "tail -f {{file}} | grep {{pattern=error}}"},
			},
		},
		State: state.State{
			"id-deploy": {LastUsedAt: "2026-07-24T08:00:00Z", Args: map[string]string{"host": "prod-7"}},
		},
		Now: func() time.Time { return time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC) },
	}
}

// press turns a scenario's key names into the messages the model consumes.
func press(m *Model, keys []string) {
	for _, name := range keys {
		switch name {
		case "enter":
			send(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		case "tab":
			send(m, tea.KeyPressMsg{Code: tea.KeyTab})
		case "down":
			send(m, tea.KeyPressMsg{Code: tea.KeyDown})
		case "ctrl+a":
			send(m, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
		case "ctrl+e":
			send(m, tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
		case "ctrl+d":
			send(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
		default:
			for _, r := range name {
				send(m, tea.KeyPressMsg{Code: r, Text: string(r)})
			}
		}
	}
}

func send(m *Model, msg tea.Msg) { m.Update(msg) }

func render(t *testing.T, m *Model) string {
	t.Helper()
	return ansi.Strip(m.View().Content)
}

var updateFrames = flag.Bool("update-frames", false,
	"rewrite testdata/frames from the current renderer")

// emptyLibrary and longLibrary are the two shapes the fixture cannot show: the
// screen a new user lands on, and one with more commands than the panel holds.
func emptyLibrary() library.Library { return library.Library{Version: 2} }

func longLibrary() library.Library {
	lib := library.Library{Version: 2}
	for i := 0; i < 15; i++ {
		lib.Commands = append(lib.Commands, library.Entry{
			ID:      fmt.Sprintf("id-%d", i),
			Name:    fmt.Sprintf("command number %d", i),
			Command: fmt.Sprintf("echo %d", i),
		})
	}
	return lib
}

func TestFrames(t *testing.T) {
	down := func(n int) []string {
		keys := make([]string, n)
		for i := range keys {
			keys[i] = "down"
		}
		return keys
	}

	cases := []struct {
		name     string
		rows     int
		columns  int
		keys     []string
		migrated bool
		lib      *library.Library
	}{
		// the three brand-block tiers and their boundaries
		{name: "list-40x80-fullbanner", rows: 40, columns: 80},
		{name: "list-30x80-fullbanner", rows: 30, columns: 80},
		{name: "list-29x80-compact", rows: 29, columns: 80},
		{name: "list-24x80", rows: 24, columns: 80},
		{name: "list-19x80-compact", rows: 19, columns: 80},
		{name: "list-18x80-nobanner", rows: 18, columns: 80},
		{name: "list-40x50-compact-narrow", rows: 40, columns: 50},
		{name: "list-24x50-narrow", rows: 24, columns: 50},

		{name: "list-24x80-query", rows: 24, columns: 80, keys: []string{"ports"}},
		{name: "list-24x80-nomatch", rows: 24, columns: 80, keys: []string{"zzz"}},
		{name: "list-24x80-down", rows: 24, columns: 80, keys: []string{"down"}},
		{name: "list-24x80-migrated", rows: 24, columns: 80, migrated: true},

		// the empty and overflowing states
		{name: "list-24x80-empty", rows: 24, columns: 80, lib: ptr(emptyLibrary())},
		{name: "list-14x80-scroll-top", rows: 14, columns: 80, lib: ptr(longLibrary())},
		{name: "list-14x80-scroll-middle", rows: 14, columns: 80, lib: ptr(longLibrary()), keys: down(8)},
		{name: "list-14x80-scroll-end", rows: 14, columns: 80, lib: ptr(longLibrary()), keys: down(14)},

		// the confirm is inline on the list, not a screen of its own
		{name: "list-24x80-confirm-delete", rows: 24, columns: 80, keys: []string{"ctrl+d"}},

		{name: "args-24x80", rows: 24, columns: 80, keys: []string{"enter"}},
		{name: "args-24x80-tab", rows: 24, columns: 80, keys: []string{"tail", "enter", "tab"}},
		{name: "args-14x80-short", rows: 14, columns: 80, keys: []string{"tail", "enter"}},
		{name: "edit-new-24x80", rows: 24, columns: 80, keys: []string{"ctrl+a"}},
		{name: "edit-new-24x80-refused", rows: 24, columns: 80, keys: []string{"ctrl+a", "enter"}},
		{name: "edit-new-24x80-typed", rows: 24, columns: 80, keys: []string{"ctrl+a", "backup", "tab", "tab", "tar -czf {{out=x.tgz}} ."}},
		{name: "edit-existing-24x80", rows: 24, columns: 80, keys: []string{"ctrl+e"}},
		{name: "edit-existing-14x80-short", rows: 14, columns: 80, keys: []string{"ctrl+e"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := fixtureDeps()
			deps.Migrated = tc.migrated
			if tc.lib != nil {
				deps.Library = *tc.lib
				deps.State = state.State{}
			}
			m := New(deps)
			m.SetSize(tc.columns, tc.rows)
			m.Init()
			press(m, tc.keys)

			got := render(t, m)
			// A frame that is not exactly as tall as the terminal will scroll
			// or leave a gap, whatever else it gets right.
			if lines := strings.Count(got, "\n") + 1; lines != tc.rows {
				t.Errorf("frame is %d lines, want %d", lines, tc.rows)
			}
			if *updateFrames {
				writeFrame(t, tc.name, got)
				return
			}
			if want := goldenFrame(t, tc.name); got != want {
				t.Errorf("frame differs from the golden\n%s", lineDiff(want, got))
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

func framePath(name string) string {
	return filepath.Join("..", "..", "testdata", "frames", name+".txt")
}

func goldenFrame(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(framePath(name))
	if err != nil {
		t.Fatalf("missing golden (regenerate with -update-frames): %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}

func writeFrame(t *testing.T, name, frame string) {
	t.Helper()
	if err := os.WriteFile(framePath(name), []byte(frame), 0o644); err != nil {
		t.Fatalf("writing golden: %v", err)
	}
}

func lineDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	var b strings.Builder
	for i := 0; i < max(len(wantLines), len(gotLines)); i++ {
		w, g := "", ""
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			b.WriteString("  " + itoa(i+1) + " want |" + w + "|\n")
			b.WriteString("  " + itoa(i+1) + "  got |" + g + "|\n")
		}
	}
	return b.String()
}

func itoa(n int) string {
	if n < 10 {
		return " " + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// A pty that reports no window size delivers 0×0; rendering must fall back to
// the default rather than panic on a negative border run.
func TestDegenerateTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {1, 1}, {4, 3}, {-5, -5}} {
		m := New(fixtureDeps())
		m.SetSize(size[0], size[1])
		if got := render(t, m); got == "" {
			t.Errorf("%v rendered nothing", size)
		}
		press(m, []string{"ctrl+a"})
		if got := render(t, m); got == "" {
			t.Errorf("%v rendered nothing on the edit screen", size)
		}
	}
}
