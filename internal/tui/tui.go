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

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/state"
)

// Deps are the effects the TUI needs, injected so tests can observe them.
//
// The two saves report failure. They used to return nothing, and the closures
// wiring them to the disk discarded the error — so a Library that failed to
// write looked exactly like one that wrote, and the edit screen flashed "Saved"
// on the strength of a call it could not see fail. Now that library.Save also
// refuses a Library it would not be able to read back, a save has a second way
// to fail that the user has to hear about.
type Deps struct {
	Library     library.Library
	State       state.State
	SaveLibrary func(library.Library) error
	SaveState   func(state.State) error
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
	screen   screen
	// caret is the blink clock for every Field in the frame. It used to be the
	// clock for all but one: the search field blinked on the one inside its
	// textinput, which bubbles keeps unexported and unreadable, so the rest ran
	// a copy of it and a test stood over the two to prove they had not drifted.
	// Now potato paints every caret and nothing reads the one inside — there is
	// this clock and no other. Only one Field has the keyboard at a time, so one
	// clock is all there is to keep.
	caret cursor.Model
}

func New(deps Deps) *Model {
	m := &Model{
		deps:   deps,
		lib:    deps.Library,
		st:     deps.State,
		width:  80,
		height: 24,
		caret:  cursor.New(),
	}
	// A new cursor starts hidden, so without this the first frame would have no
	// caret in it at all. The command it returns is dropped, the way every Field
	// drops the one Focus hands back (see field.Focus): the caret is solid until
	// the first keystroke re-arms it, so a Field that has just been handed the
	// keyboard looks the same on launch as on a tab round-trip.
	m.caret.Focus()
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

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var blink tea.Cmd
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
		// A caret that blinked while you were typing under it would read as a
		// dropped keystroke, so every key restarts the clock solid — the same
		// rule bubbles applies to the search field, which re-arms its own blink
		// on every move of the caret.
		m.caret.IsBlinked = false
		blink = m.caret.Blink()
	default:
		// The blink's own message, on its way back. The cursor is choosy about
		// which ones it answers to, so the field's copy of it further down is
		// free to see this too and ignore it.
		m.caret, blink = m.caret.Update(msg)
	}
	return m, tea.Batch(blink, m.screen.update(m, msg))
}

// caretOn is the blink's lit half — whether a field drawing its own caret
// should paint it this frame.
func (m *Model) caretOn() bool { return !m.caret.IsBlinked }

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
	// Library has no size limit and a terminal does; twenty-four rows is
	// enough to scan a large Library, little enough that opening potato does
	// not push everything you were reading off the top of the screen.
	inlineBodyCap = 24
)

// bodyHeight is the fixed row count every screen renders into. It depends on
// the terminal alone, never on what is being drawn into it: rendering inline,
// a frame that grew and shrank with its content would reflow the terminal
// under it on every keystroke — filter three commands down to one and
// everything below potato jumps up. One height for the terminal, every screen
// pads to it, and the block never moves while it is open.
func (m *Model) bodyHeight() int {
	// the footer's rule and key row, and the shell prompt the frame sits under
	return max(1, min(m.height-3, inlineBodyCap))
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

// quit ends the program, drawing one empty frame on the way out so the inline
// block is erased rather than left in the scrollback.
func (m *Model) quit() tea.Cmd {
	m.quitting = true
	return tea.Quit
}

func (m *Model) rememberUse(id string, args map[string]string) error {
	return m.updateState(state.RecordUse(m.st, id, args, m.deps.Now()))
}

// updateLibrary and updateState adopt the new value and write it out, reporting
// what the write did rather than flashing it. Raising the flash is finish's, so
// an action that writes both files raises exactly one — see finish.
//
// The in-memory value is kept either way. A write that failed has not lost the
// user their edit, and rolling it back would throw away what they typed to
// report a problem with the disk — so the frame shows the change and the flash
// says it is not saved yet.
func (m *Model) updateLibrary(next library.Library) error {
	m.lib = next
	if m.deps.SaveLibrary == nil {
		return nil
	}
	return m.deps.SaveLibrary(next)
}

func (m *Model) updateState(next state.State) error {
	m.st = next
	if m.deps.SaveState == nil {
		return nil
	}
	return m.deps.SaveState(next)
}

// finish reports an action's outcome: the first failure among the writes it
// made, or the action's own confirmation if they all wrote.
//
// It takes errors rather than the flashes for them because setting a flash is a
// side effect, not a value — m.flash is assigned the moment setFlash is called.
// An action that writes both files and has both fail would otherwise leave the
// *last* failure's text on screen while returning the *first* one's timer, and
// on a delete that means being told about state.json — the disposable cache —
// while commands.json is the file that did not get written. One failure is
// chosen here, then raised, so the text and the timer are always the same one.
//
// A failure outlasts the ordinary flash: "Deleted 'x'" is a confirmation you can
// miss without cost, and this is not.
func (m *Model) finish(message string, writes ...error) tea.Cmd {
	for _, err := range writes {
		if err != nil {
			return m.setFlash("⚠ Not saved: "+err.Error(), 6000*time.Millisecond)
		}
	}
	return m.flashDefault(message)
}

func (m *Model) run(id string, values map[string]string) tea.Cmd {
	command, ok := library.Find(m.lib, id)
	if !ok {
		return nil
	}
	// A failed State write is not worth reporting here: the hand-off is what
	// the user came for and potato is closing, so the flash would be erased by
	// the same frame that draws it. State is disposable; the command is not.
	_ = m.rememberUse(id, values)
	m.handoff = renderCommand(command.Template, values)
	return m.quit()
}

func (m *Model) copy(id string, values map[string]string) tea.Cmd {
	command, ok := library.Find(m.lib, id)
	if !ok {
		return nil
	}
	saved := m.rememberUse(id, values)
	if m.deps.Copy != nil {
		m.deps.Copy(renderCommand(command.Template, values))
	}
	return m.finish("Copied to clipboard", saved)
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
