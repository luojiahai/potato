// Package tui is the potato TUI (spec §3): a fuzzy-search list with a detail
// strip, a single-form arg screen with live preview, and in-app CRUD.
//
// It renders inline rather than on the alternate screen — a block of fixed
// height under the prompt you launched it from, erased on the way out. Potato
// is a picker you open for two seconds, and its output is the command left at
// your prompt, not a window you were in.
//
// Identity is the Command's id (a UUID): screens carry ids, State is keyed by
// id, and a rename keeps both the id and the file slot. Names are what the
// user sees, searches, and must keep unique.
package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
	// quitting makes the next frame an empty one, erasing the inline block.
	quitting bool
	// body is the fixed row count every screen renders into; see measure.
	body   int
	screen screen
}

func New(deps Deps) *Model {
	m := &Model{
		deps:   deps,
		lib:    deps.Library,
		st:     deps.State,
		width:  80,
		height: 24,
	}
	m.measure()
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
	m.measure()
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
		if key.Matches(msg, keymap.global.Cancel) {
			// Ink's exitOnCtrlC: quit with no hand-off, so the --out file is
			// written empty and the wrapper treats it as cancelled.
			return m, m.quit()
		}
	}
	return m, m.screen.update(m, msg)
}

func (m *Model) View() tea.View {
	// The last frame before the program returns is an empty one, which erases
	// the block potato drew. Rendering inline means whatever is on screen at
	// exit stays in the scrollback, and potato's output is the command now
	// sitting at your prompt — not a picker frozen above it.
	//
	// The event loop renders after the Update that asked to quit and only then
	// reads the quit message, so this frame is drawn.
	if m.quitting {
		view := tea.NewView("")
		return view
	}

	lines := m.screen.view(m)
	lines = append(lines, footer(m.screen.keys(m), m.flash, m.innerWidth())...)

	// The app's own paddingX of 1, then the clamp, then the per-line right trim.
	//
	// The clamp is a backstop, not the layout: every screen sizes its own rows
	// to the width it was given. But a row wider than the terminal does not
	// just look wrong — it wraps, and one wrapped row pushes the last row of
	// the frame off the bottom of the screen. On a terminal too narrow to hold
	// even a footer chord there is no graceful answer, and cutting the row is a
	// better one than losing the frame.
	for i, line := range lines {
		lines[i] = trimRightVisible(ansi.Truncate(" "+line, m.width, ""))
	}

	view := tea.NewView(strings.Join(lines, "\n"))
	// Inline, not the alternate screen: potato draws only the rows it needs,
	// where you typed it, and takes them back on the way out.
	view.AltScreen = false
	return view
}

const (
	// inlineBodyCap is the most rows any screen may draw above the footer. A
	// Library has no size limit and a terminal does; twenty rows is about what
	// `fzf --height 40%` claims on a normal terminal — enough to scan, little
	// enough that opening potato does not push what you were reading off the
	// top of the screen.
	inlineBodyCap = 20
	// inlineBodyFloor keeps the frame tall enough to add a Command in. The
	// frame is one height for every screen, and the list is not the screen
	// that needs the most room: twelve rows is the add form with one
	// placeholder — three field sections, the blanks between them, and the
	// placeholder list under the command. Sized to the list alone, a small
	// Library would give itself a frame with nowhere to put the form that
	// grows it, and the placeholder list would be the first thing squeezed out.
	inlineBodyFloor = 12
)

// measure fixes the frame's height for as long as the Library and the terminal
// stay the way they are.
//
// Rendering inline, a frame that grew and shrank with its content would reflow
// the terminal under it on every keystroke — filter three commands down to one
// and everything below potato jumps up. So the height is measured once, from
// the whole Library with no query against it, and then held: every screen pads
// to it, a query that matches nothing is the same size as one that matches
// everything, and the block never moves while it is open.
//
// It is measured rather than fixed at a constant so that a small Library still
// gets a small frame. Six commands do not need twenty rows.
func (m *Model) measure() {
	// The probe renders the list with no query — the whole Library — against
	// the ceiling, which is the tallest the frame could ever need to be.
	m.body = m.ceiling()
	want := len(newListScreen(m).content(m))
	m.body = min(m.ceiling(), max(min(inlineBodyFloor, m.ceiling()), want))
}

// ceiling is the tallest body this terminal allows. It depends on the terminal
// alone, never on what is being drawn into it — a layout that sized a region
// from bodyHeight would be reading a number measure had not finished computing,
// and would render into a frame sized for a different layout than the one it
// then drew.
func (m *Model) ceiling() int {
	// the footer's rule and key row, and the shell prompt the frame sits under
	return max(1, min(m.height-3, inlineBodyCap))
}

func (m *Model) bodyHeight() int { return m.body }

func (m *Model) innerWidth() int { return m.width - 2 }

// ---------- shared actions ----------

func (m *Model) setFlash(msg string, d time.Duration) tea.Cmd {
	m.flash = msg
	return tea.Tick(d, func(time.Time) tea.Msg { return flashExpiredMsg{} })
}

func (m *Model) flashDefault(msg string) tea.Cmd {
	return m.setFlash(msg, 1500*time.Millisecond)
}

// quit ends the program, drawing one empty frame on the way out so the inline
// block is erased rather than left in the scrollback.
func (m *Model) quit() tea.Cmd {
	m.quitting = true
	return tea.Quit
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
	// adding or deleting a Command is the one thing that resizes the frame,
	// and it is a keystroke the user meant
	m.measure()
	if m.deps.SaveLibrary != nil {
		m.deps.SaveLibrary(next)
	}
}

func (m *Model) run(id string, values map[string]string) tea.Cmd {
	m.rememberUse(id, values)
	entry := library.FindByID(m.lib, id)
	m.handoff = renderCommand(entry.Command, values)
	return m.quit()
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
