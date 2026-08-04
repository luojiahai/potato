// Command screenshots renders the README's screens as ANSI text, for
// scripts/screenshots.sh to turn into the PNGs in docs/media.
//
// The library it draws is a fixture, not whatever happens to be in
// ~/.potato/commands.json: a screenshot has to be reproducible, and the one in
// the README should show potato holding a plausible day's worth of commands
// rather than the author's. The clock is fixed too, so "2h ago" stays "2h ago".
//
// It talks to the TUI the way the frame goldens do — build the Model, size it,
// press keys — because the real binary needs a terminal and would hand its
// frame to one rather than to a file.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/state"
	"github.com/luojiahai/potato/internal/tui"
	"github.com/luojiahai/potato/internal/version"
)

// now is the fixed clock the fixture's last-used times are relative to.
var now = time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

func ago(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }

func text(s string) *string { return &s }

func fixture() (library.Library, state.State) {
	lib := library.Library{
		Version: 2,
		Commands: []library.Command{
			{
				ID: "id-deploy", Name: "deploy web",
				Description: text("Roll out the web tier"),
				Template:    "ssh {{host=prod-1}} 'cd /srv/web && git pull && systemctl restart web'",
			},
			{
				ID: "id-logs", Name: "tail app logs",
				Description: text("Follow one app's log, filtered"),
				Template:    "tail -f /var/log/{{app}}/app.log | grep -i {{pattern=error}}",
			},
			{
				ID: "id-psql", Name: "db shell",
				Description: text("Open a psql session"),
				Template:    `psql "postgres://{{user=app}}@{{host=db-1}}:5432/{{db=app}}"`,
			},
			{
				ID: "id-ports", Name: "port hogs",
				Description: text("Show what is listening"),
				Template:    "lsof -iTCP -sTCP:LISTEN -P -n",
			},
			{
				ID: "id-sync", Name: "sync build",
				Description: text("Push a build to a host"),
				Template:    "rsync -avz --delete --progress ./{{src=dist}}/ {{user=deploy}}@{{host}}:/srv/{{app}}/",
			},
			{
				ID: "id-prune", Name: "prune docker",
				Description: text("Reclaim disk from docker"),
				Template:    "docker system prune -af --volumes",
			},
			{
				ID: "id-undo", Name: "undo commit",
				Description: text("Keep the changes, drop the commit"),
				Template:    "git reset --soft HEAD~1",
			},
		},
	}
	// Only some of the library has been used, so the list shows both halves of
	// the empty-query order: most recent first, then the rest in file order.
	st := state.State{
		"id-deploy": {LastUsedAt: ago(2 * time.Hour), Args: map[string]string{"host": "prod-1"}},
		"id-logs":   {LastUsedAt: ago(20 * time.Hour), Args: map[string]string{"app": "checkout"}},
		"id-psql":   {LastUsedAt: ago(48 * time.Hour), Args: map[string]string{"user": "app", "host": "db-1", "db": "app"}},
	}
	return lib, st
}

// frame renders one screen: a terminal size, and the keys to press on the way
// in. Heights are the shortest that hold the screen whole, so neither
// screenshot carries a band of empty rows.
type frame struct {
	name    string
	rows    int
	columns int
	keys    []string
}

func render(f frame, versionLabel string) string {
	version.Version = versionLabel
	lib, st := fixture()
	m := tui.New(tui.Deps{
		Library:     lib,
		State:       st,
		SaveLibrary: func(library.Library) error { return nil },
		SaveState:   func(state.State) error { return nil },
		Copy:        func(string) {},
		Now:         func() time.Time { return now },
	})
	m.SetSize(f.columns, f.rows)
	m.Init()
	for _, key := range f.keys {
		switch key {
		case "enter":
			m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		default:
			for _, r := range key {
				m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			}
		}
	}
	return m.View().Content
}

// reverseRun matches a run potato paints by swapping a foreground into the
// background — which, in the frame, is only ever the caret (see caretStyle).
var reverseRun = regexp.MustCompile(`\x1b\[7;38;2;(\d+;\d+;\d+)m`)

// unreverse spells the caret out as the pair of colours it stands for.
//
// freeze does not implement SGR 7. A reversed run reaches it as a plain
// foreground, so the caret — a space — comes out as nothing at all, and the
// screenshot loses the one mark that says the search field has the keyboard.
// The terminal is the other half of the swap, so the glyph takes the
// background the image is being drawn on.
func unreverse(frame, background string) (string, error) {
	hex := strings.TrimPrefix(background, "#")
	if len(hex) != 6 {
		return "", fmt.Errorf("background %q is not a #rrggbb colour", background)
	}
	channels := make([]string, 3)
	for i := range channels {
		v, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if err != nil {
			return "", fmt.Errorf("background %q is not a #rrggbb colour", background)
		}
		channels[i] = strconv.FormatUint(v, 10)
	}
	ink := strings.Join(channels, ";")
	// ${1}, not $1: the group is followed by the sequence's own "m", and Go
	// would read the two together as a group named "1m".
	return reverseRun.ReplaceAllString(frame, "\x1b[38;2;"+ink+";48;2;${1}m"), nil
}

func main() {
	out := flag.String("out", "", "directory to write the ANSI dumps to")
	label := flag.String("version", "dev", "version to draw on the header rule")
	background := flag.String("background", "#000000", "the colour the frame will be drawn on")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "screenshots: -out is required")
		os.Exit(1)
	}

	frames := []frame{
		// The list, on a terminal tall enough for the whole library and the
		// detail strip under it.
		{name: "list", rows: 20, columns: 80},
		// The arg screen, reached the way a user reaches it: type until the
		// command it belongs to is the selection, then Enter.
		{name: "arguments", rows: 10, columns: 80, keys: []string{"app logs", "enter"}},
	}
	for _, f := range frames {
		frame, err := unreverse(render(f, *label), *background)
		if err != nil {
			fmt.Fprintf(os.Stderr, "screenshots: %s\n", err)
			os.Exit(1)
		}
		path := filepath.Join(*out, f.name+".ansi")
		if err := os.WriteFile(path, []byte(frame), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "screenshots: %s\n", err)
			os.Exit(1)
		}
	}
}
