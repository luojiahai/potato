package tui

import (
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

// Parity against the Ink build. testdata/parity/*.txt were captured from the
// TypeScript TUI before it was deleted, at fixed terminal sizes with a fixed
// clock; every frame here must reproduce one byte for byte.
//
// One documented normalisation: Ink drew a decorative `▌` at the end of the
// focused value, and the adopted textinput draws a real cursor over the cell
// after it. Both are one column, so the goldens are compared with `▌` mapped
// to a space. That is the whole of the rendering difference.

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

func TestParityWithInk(t *testing.T) {
	cases := []struct {
		name     string
		rows     int
		columns  int
		keys     []string
		migrated bool
	}{
		{name: "list-40x80", rows: 40, columns: 80},
		{name: "list-24x80", rows: 24, columns: 80},
		{name: "list-19x80", rows: 19, columns: 80},
		{name: "list-18x80-nobanner", rows: 18, columns: 80},
		{name: "list-24x50-narrow", rows: 24, columns: 50},
		{name: "list-24x80-query", rows: 24, columns: 80, keys: []string{"ports"}},
		{name: "list-24x80-nomatch", rows: 24, columns: 80, keys: []string{"zzz"}},
		{name: "list-24x80-down", rows: 24, columns: 80, keys: []string{"down"}},
		{name: "list-24x80-migrated", rows: 24, columns: 80, migrated: true},
		{name: "args-24x80", rows: 24, columns: 80, keys: []string{"enter"}},
		{name: "args-24x80-tab", rows: 24, columns: 80, keys: []string{"tail", "enter", "tab"}},
		{name: "edit-new-24x80", rows: 24, columns: 80, keys: []string{"ctrl+a"}},
		{name: "edit-existing-24x80", rows: 24, columns: 80, keys: []string{"ctrl+e"}},
		{name: "delete-24x80", rows: 24, columns: 80, keys: []string{"ctrl+d"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := fixtureDeps()
			deps.Migrated = tc.migrated
			m := New(deps)
			m.SetSize(tc.columns, tc.rows)
			m.Init()
			press(m, tc.keys)

			got := render(t, m)
			want := goldenFrame(t, tc.name)

			if reason, known := inkDegenerate[tc.name]; known {
				// Ink's flexbox overflowed here and rendered something broken.
				// potato-in-Go renders it correctly instead, so the frames
				// differ by design; assert the geometry is still sound and
				// that the difference has not silently disappeared.
				if lines := strings.Count(got, "\n") + 1; lines != tc.rows {
					t.Errorf("frame is %d lines, want %d", lines, tc.rows)
				}
				if got == want {
					t.Errorf("frame now matches the Ink golden — drop %q from inkDegenerate", tc.name)
				}
				t.Logf("known difference: %s", reason)
				return
			}

			if got == want {
				return
			}
			t.Errorf("frame differs from the Ink golden\n%s", lineDiff(want, got))
		})
	}
}

// inkDegenerate lists the captured frames where Ink's flexbox ran out of room
// and produced visibly broken output. Reproducing them would mean emulating
// Yoga's flex-shrink over inline text; the Go build renders them correctly
// instead, and each entry records what the difference is.
var inkDegenerate = map[string]string{
	"list-19x80": "Ink clipped the wordmark's top row and drew the detail panel's " +
		"title and last line into its own borders (issue #55's symptom); the Go " +
		"build fits the banner and the panel in 19 rows",
	"list-24x50-narrow": "Ink squeezed the footer's key hints onto two rows, " +
		"truncating '↵ run' to '↵ ru'; the Go build keeps the footer on one row",
}

func goldenFrame(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "parity", name+".txt"))
	if err != nil {
		t.Fatalf("missing golden: %v", err)
	}
	// the documented cursor-cell normalisation
	lines := strings.Split(strings.ReplaceAll(string(raw), "▌", " "), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
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
