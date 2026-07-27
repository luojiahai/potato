// Package tui is the potato TUI (spec §3): a split-pane list with fuzzy
// search, a single-form arg screen with live preview, and in-app CRUD.
//
// Identity is the Command's id (a UUID): screens carry ids, State is keyed by
// id, and a rename keeps both the id and the file slot. Names are what the
// user sees, searches, and must keep unique.
package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/state"
)

// Deps are the effects the TUI needs, injected so tests can observe them.
type Deps struct {
	Library     library.Library
	State       state.State
	Migrated    bool
	SaveLibrary func(library.Library)
	SaveState   func(state.State)
	Copy        func(string)
	Now         func() time.Time
}

// screen is one of potato's four surfaces. Each is constructed fresh on
// transition, which is what resets its query, focus and field values.
type screen interface {
	update(m *Model, msg tea.Msg) tea.Cmd
	view(m *Model) []string
	keys(m *Model) []footerKey
}

type flashExpiredMsg struct{}

type Model struct {
	deps    Deps
	lib     library.Library
	st      state.State
	width   int
	height  int
	flash   string
	handoff string
	screen  screen
}

func New(deps Deps) *Model {
	m := &Model{
		deps:   deps,
		lib:    deps.Library,
		st:     deps.State,
		width:  80,
		height: 24,
	}
	m.screen = newListScreen(m)
	return m
}

// Handoff is the rendered command the user chose, or "" if they cancelled.
func (m *Model) Handoff() string { return m.handoff }

// SetSize adopts a terminal size, ignoring the degenerate one. A pty that
// reports no window size (`script`, some CI runners) delivers 0×0; the Ink
// build fell back to 80×24 there via `stdout.columns ?? 80`, and so do we —
// rendering into zero columns has no sensible answer.
func (m *Model) SetSize(width, height int) {
	if width > 0 {
		m.width = width
	}
	if height > 0 {
		m.height = height
	}
}

func (m *Model) Init() tea.Cmd {
	// A v1→v2 upgrade happened on this launch: announce it with a transient
	// footer toast, populated from the first frame (migration ran pre-render).
	if m.deps.Migrated {
		return m.setFlash("upgraded your library to v2", 4000*time.Millisecond)
	}
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil
	case flashExpiredMsg:
		// Parity with the Ink build: a pending timer clears whatever flash is
		// showing, even one raised after it.
		m.flash = ""
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			// Ink's exitOnCtrlC: quit with no hand-off, so the --out file is
			// written empty and the wrapper treats it as cancelled.
			return m, tea.Quit
		}
	}
	return m, m.screen.update(m, msg)
}

func (m *Model) View() tea.View {
	var lines []string
	if m.showBanner() {
		lines = append(lines, banner()...)
	}
	lines = append(lines, m.screen.view(m)...)
	lines = append(lines, footer(m.screen.keys(m), m.flash)...)

	// the app's own paddingX of 1, then the per-line right trim
	for i, line := range lines {
		lines[i] = trimRightVisible(" " + line)
	}

	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

// showBanner hides the wordmark on short or narrow terminals to keep the
// screens usable (app paddingX 1 + banner paddingX 1 on both sides = 4 extra
// columns).
func (m *Model) showBanner() bool {
	return m.height >= 19 && m.width >= bannerWidth()+4
}

// bodyHeight is the space between the banner and the footer.
func (m *Model) bodyHeight() int {
	h := m.height - 2 // footer margin row + footer row
	if m.showBanner() {
		h -= bannerRowCount + 1
	}
	return max(0, h)
}

func (m *Model) innerWidth() int { return m.width - 2 }

// ---------- shared actions ----------

func (m *Model) setFlash(msg string, d time.Duration) tea.Cmd {
	m.flash = msg
	return tea.Tick(d, func(time.Time) tea.Msg { return flashExpiredMsg{} })
}

func (m *Model) flashDefault(msg string) tea.Cmd {
	return m.setFlash(msg, 1500*time.Millisecond)
}

func (m *Model) rememberUse(id string, args map[string]string) {
	next := state.RecordUse(m.st, id, args, m.deps.Now())
	m.st = next
	if m.deps.SaveState != nil {
		m.deps.SaveState(next)
	}
}

func (m *Model) updateLibrary(next library.Library) {
	m.lib = next
	if m.deps.SaveLibrary != nil {
		m.deps.SaveLibrary(next)
	}
}

func (m *Model) run(id string, values map[string]string) tea.Cmd {
	m.rememberUse(id, values)
	entry := library.FindByID(m.lib, id)
	m.handoff = renderCommand(entry.Command, values)
	return tea.Quit
}

func (m *Model) copy(id string, values map[string]string) tea.Cmd {
	m.rememberUse(id, values)
	entry := library.FindByID(m.lib, id)
	if m.deps.Copy != nil {
		m.deps.Copy(renderCommand(entry.Command, values))
	}
	return m.flashDefault("copied to clipboard")
}

// ---------- program ----------

// Run opens the TUI and returns the rendered command to hand off, or "" if the
// user cancelled.
func Run(deps Deps) (string, error) {
	m := New(deps)
	program := tea.NewProgram(m)
	final, err := program.Run()
	if err != nil {
		return "", err
	}
	if done, ok := final.(*Model); ok {
		return done.handoff, nil
	}
	return "", nil
}
