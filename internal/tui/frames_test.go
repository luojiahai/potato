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

// longCommandLibrary holds one Command too long for the detail strip, so the
// strip's cap and its truncation marker are both exercised.
func longCommandLibrary() library.Library {
	return library.Library{Version: 2, Commands: []library.Entry{{
		ID: "id-long", Name: "rsync everything",
		Command: "rsync -avz --delete --exclude '.git' --exclude 'node_modules' --exclude '*.log' " +
			"--partial --progress -e 'ssh -p {{port=22}}' ./{{src=dist}}/ {{user=deploy}}@{{host}}:/srv/{{app}}/releases/",
	}}}
}

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
		// the geometry tiers: the frame is as tall as its content until the
		// row ceiling stops it, the detail strip yields on a short terminal,
		// and a row's command preview yields on a narrow one
		{name: "list-40x80-capped", rows: 40, columns: 80, lib: ptr(longLibrary())},
		{name: "list-24x80", rows: 24, columns: 80},
		{name: "list-13x80-detail", rows: 13, columns: 80},
		{name: "list-12x80-nodetail", rows: 12, columns: 80},
		{name: "list-40x50-narrow", rows: 40, columns: 50},
		{name: "list-24x50-narrow", rows: 24, columns: 50},
		// narrow enough that the footer has to drop chords to fit
		{name: "list-24x40-narrow", rows: 24, columns: 40},

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

		// a Command longer than the space it is given: the detail strip caps
		// and marks what it cut, and the edit screen scrolls to the caret
		{name: "list-24x80-long", rows: 24, columns: 80, lib: ptr(longCommandLibrary())},
		{name: "edit-12x80-long", rows: 12, columns: 80, lib: ptr(longCommandLibrary()), keys: []string{"ctrl+e"}},
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
			// Rendering inline, the frame is as tall as its content — but it
			// must still leave the shell prompt it was launched from on the
			// screen. A frame that fills the terminal scrolls that prompt away.
			if lines := strings.Count(got, "\n") + 1; lines > tc.rows-1 {
				t.Errorf("frame is %d lines, terminal is %d — nothing left for the prompt", lines, tc.rows)
			}
			// A row wider than the terminal wraps, and one wrapped row pushes
			// every row under it down — the frame loses its last line off the
			// bottom of the screen. The footer used to do exactly this at 50
			// columns, and being compared de-ANSI'd line by line, the goldens
			// could not see it.
			for i, line := range strings.Split(got, "\n") {
				if w := ansi.StringWidth(line); w > tc.columns {
					t.Errorf("line %d is %d columns wide, terminal is %d: %q", i+1, w, tc.columns, line)
				}
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

// The detail strip is where you check what a Command will ask you for before
// you press Enter, so a Command with five arguments has to show five.
//
// The frame's height is measured from a probe render of this very screen, so
// anything here that sizes itself from the frame's height is reading a number
// that is not settled yet: the probe measures one layout and the screen then
// draws a smaller one into it. That went unnoticed because the goldens are
// regenerated from the renderer — a truncated strip is only a diff against the
// frame before it, and it passes its own test either way.
func TestTheDetailStripShowsEveryArgument(t *testing.T) {
	deps := fixtureDeps()
	deps.Library = longCommandLibrary()
	deps.State = state.State{}
	m := New(deps)
	m.SetSize(80, 24)

	frame := render(t, m)
	for _, arg := range []string{"port = 22", "src = dist", "user = deploy", "host", "app"} {
		if !strings.Contains(frame, "  "+arg) {
			t.Errorf("the strip does not list %q:\n%s", arg, frame)
		}
	}
	if strings.Contains(frame, "…\n") {
		t.Errorf("the strip truncated a Command the frame had room for:\n%s", frame)
	}
}

// Potato is one height for as long as it is open. Rendering inline, a frame
// that grew or shrank would reflow the terminal under it — filter three
// commands down to one and everything below potato jumps up a row.
func TestTheFrameHoldsItsHeight(t *testing.T) {
	open := New(fixtureDeps())
	open.SetSize(80, 24)
	want := strings.Count(render(t, open), "\n") + 1

	for name, keys := range map[string][]string{
		"a query that matches nothing": {"zzz"},
		"a query that matches one":     {"tail"},
		"the selection moved":          {"down"},
		"the add form":                 {"ctrl+a"},
		"the add form, typed into":     {"ctrl+a", "backup", "tab", "tab", "tar -czf {{out=x.tgz}} ."},
		"the edit form":                {"ctrl+e"},
		"the arg screen":               {"enter"},
		"the delete confirm":           {"ctrl+d"},
	} {
		m := New(fixtureDeps())
		m.SetSize(80, 24)
		press(m, keys)
		if got := strings.Count(render(t, m), "\n") + 1; got != want {
			t.Errorf("%s: frame is %d rows, want %d", name, got, want)
		}
	}
}

// A pty that reports no window size delivers 0×0; rendering must fall back to
// the default rather than panic on a negative border run. The sizes either side
// of it are the ones where a column budget goes negative — a rule with nothing
// left for its label, a row with nothing left for its name.
func TestDegenerateTerminalSizes(t *testing.T) {
	screens := map[string][]string{
		"list":    nil,
		"args":    {"enter"},
		"edit":    {"ctrl+e"},
		"confirm": {"ctrl+d"},
		"nomatch": {"zzz"},
	}
	for _, size := range [][2]int{{0, 0}, {1, 1}, {2, 2}, {4, 3}, {8, 6}, {20, 10}, {-5, -5}} {
		for name, keys := range screens {
			m := New(fixtureDeps())
			m.SetSize(size[0], size[1])
			press(m, keys)
			got := render(t, m)
			if got == "" {
				t.Errorf("%v rendered nothing on the %s screen", size, name)
				continue
			}
			for i, line := range strings.Split(got, "\n") {
				if w := ansi.StringWidth(line); size[0] > 0 && w > size[0] {
					t.Errorf("%v %s: line %d is %d columns wide: %q", size, name, i+1, w, line)
				}
			}
		}
	}
}

// The list is budgeted a number of rows, and the detail strip under it is
// counted into the same body. A list that returned more rows than it was given
// would push the strip's last line off the frame — which is what reserving both
// overflow counters did on a terminal with only two rows to give the list.
func TestTheListKeepsWithinItsRowBudget(t *testing.T) {
	for rows := 8; rows <= 24; rows++ {
		m := New(fixtureDeps())
		m.SetSize(80, rows)
		frame := render(t, m)
		if !strings.Contains(frame, "deploy prod") {
			continue // too short for a list at all
		}
		// The strip names the selected Command, so its rule is the marker. An
		// overrun cuts from the bottom of the frame, so what to check for is
		// the strip's *last* row — the arguments — not its first.
		if !strings.Contains(frame, "─ deploy prod ") {
			continue // too short for the strip
		}
		if !strings.Contains(frame, "host = prod-1") {
			t.Errorf("%d rows: the list overran and the strip lost its last row:\n%s", rows, frame)
		}
	}
}
