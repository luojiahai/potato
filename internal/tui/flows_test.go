package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/state"
)

// Screen wiring the parity frames cannot see: hand-off, clipboard, CRUD, and
// what reaches State. Identity is the Command id; a rename keeps id + slot.

type recorder struct {
	libraries []library.Library
	states    []state.State
	copied    []string
	// failWith is what both saves return, so a test can be the adapter that
	// cannot write. The value is still recorded — a failed save is one that was
	// attempted.
	failWith error
	// failStateWith overrides failWith for the State write alone, so a test can
	// tell the two failures apart when an action writes both files and both fail.
	failStateWith error
}

func harness(t *testing.T) (*Model, *recorder) {
	t.Helper()
	rec := &recorder{}
	deps := fixtureDeps()
	deps.SaveLibrary = func(lib library.Library) error {
		rec.libraries = append(rec.libraries, lib)
		return rec.failWith
	}
	deps.SaveState = func(s state.State) error {
		rec.states = append(rec.states, s)
		if rec.failStateWith != nil {
			return rec.failStateWith
		}
		return rec.failWith
	}
	// true stands in for a native clipboard tool that took the text, which is the
	// path the "Copied to clipboard" assertions are about.
	deps.Copy = func(text string) bool {
		rec.copied = append(rec.copied, text)
		return true
	}
	m := New(deps)
	m.SetSize(80, 24)
	return m, rec
}

func TestEnterOnAPlainCommandHandsOffAndRecordsUse(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ports", "enter"})

	if m.Handoff() != "lsof -iTCP -sTCP:LISTEN" {
		t.Errorf("handoff = %q", m.Handoff())
	}
	if len(rec.states) == 0 {
		t.Fatal("no state was saved")
	}
	last := rec.states[len(rec.states)-1]
	if last["id-ports"].LastUsedAt != "2026-07-24T10:00:00.000Z" {
		t.Errorf("state = %+v", last)
	}
}

func TestEnterOnATemplatedCommandOpensTheArgForm(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"enter"})

	if _, ok := m.screen.(*argsScreen); !ok {
		t.Fatalf("screen = %T, want the arg form", m.screen)
	}
	if m.Handoff() != "" {
		t.Errorf("handed off early: %q", m.Handoff())
	}
	// pre-fill precedence: last value > default > empty
	frame := render(t, m)
	if !strings.Contains(frame, "prod-7") {
		t.Errorf("arg not pre-filled from State:\n%s", frame)
	}
}

func TestArgFormRunsWithTheSuppliedValues(t *testing.T) {
	m, rec := harness(t)
	// The arg arrives pre-filled from State, so this is what changing it looks
	// like: clear the field, type over it, run.
	press(m, []string{"enter", "ctrl+u", "prod-9", "enter"})

	if m.Handoff() != "ssh prod-9 'deploy.sh'" {
		t.Errorf("handoff = %q", m.Handoff())
	}
	last := rec.states[len(rec.states)-1]
	if last["id-deploy"].Args["host"] != "prod-9" {
		t.Errorf("args not remembered: %+v", last["id-deploy"])
	}
}

func TestCtrlYCopiesWithoutHandingOff(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ports"})
	send(m, tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})

	if len(rec.copied) != 1 || rec.copied[0] != "lsof -iTCP -sTCP:LISTEN" {
		t.Errorf("copied = %v", rec.copied)
	}
	if m.Handoff() != "" {
		t.Errorf("copy handed off: %q", m.Handoff())
	}
	if !strings.Contains(render(t, m), "Copied to clipboard") {
		t.Error("no flash after copying")
	}
}

// With no native tool, all that ran is a sequence the terminal never answers.
// The flash says that rather than promising a clipboard nobody checked — the
// user who trusts it pastes whatever was there before.
func TestCopyWithoutANativeToolSaysWhatItActuallyDid(t *testing.T) {
	m, _ := harness(t)
	m.deps.Copy = func(string) bool { return false }
	press(m, []string{"ports"})
	send(m, tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})

	frame := render(t, m)
	if strings.Contains(frame, "Copied to clipboard") {
		t.Errorf("a copy potato could not confirm claimed the clipboard:\n%s", frame)
	}
	if !strings.Contains(frame, "OSC 52") {
		t.Errorf("the flash does not say what actually happened:\n%s", frame)
	}
}

// What the add form is responsible for is the wiring: which field becomes which
// part of the Draft, that a save is attempted, and that the screen moves on.
// Whether the Library minted a fresh id and kept its array order is the
// Library's own promise, tested in library_test.go.
func TestAddHandsTheFormsFieldsToTheLibrary(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+n",
		"new one", "tab", "a description", "tab", "echo new", "enter"})

	if len(rec.libraries) != 1 {
		t.Fatalf("saved %d libraries, want 1", len(rec.libraries))
	}
	saved := rec.libraries[0]
	added, ok := findByName(saved, "new one")
	if !ok {
		t.Fatalf("the typed name is not in the saved Library: %+v", saved.Commands)
	}
	if added.Template != "echo new" {
		t.Errorf("command = %q, want the command field's value", added.Template)
	}
	if added.Description == nil || *added.Description != "a description" {
		t.Errorf("description = %v, want the description field's value", added.Description)
	}
	if _, ok := m.screen.(*listScreen); !ok {
		t.Errorf("screen = %T, want the list", m.screen)
	}
}

func TestEditSavesTheRenamedName(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+o", "ctrl+u", "renamed", "enter"})

	if len(rec.libraries) != 1 {
		t.Fatalf("saved %d libraries, want 1", len(rec.libraries))
	}
	command, ok := library.Find(rec.libraries[0], "id-deploy")
	if !ok {
		t.Fatal("the edited Command is gone from the saved Library")
	}
	if command.Name != "renamed" {
		t.Errorf("name = %q", command.Name)
	}
}

// A save that failed must not be reported as one that worked. The edit is kept
// so nothing the user typed is lost, and the flash says it is not on disk.
func TestAFailedSaveSaysSoInsteadOfFlashingSaved(t *testing.T) {
	m, rec := harness(t)
	rec.failWith = errors.New("read-only file system")
	press(m, []string{"ctrl+n", "doomed", "tab", "tab", "echo x", "enter"})

	frame := render(t, m)
	if strings.Contains(frame, "Added") {
		t.Errorf("a failed save flashed success:\n%s", frame)
	}
	if !strings.Contains(frame, "Not saved") || !strings.Contains(frame, "read-only file system") {
		t.Errorf("the failure is not reported:\n%s", frame)
	}
	// the edit survives in memory, so the user can retry rather than retype
	if _, ok := findByName(m.lib, "doomed"); !ok {
		t.Error("a failed save also threw away the edit")
	}
}

func findByName(lib library.Library, name string) (library.Command, bool) {
	for _, command := range lib.Commands {
		if command.Name == name {
			return command, true
		}
	}
	return library.Command{}, false
}

func TestEditRefusesADuplicateName(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+n", "list ports", "tab", "tab", "echo x", "enter"})

	if len(rec.libraries) != 0 {
		t.Error("a duplicate name was saved")
	}
	if !strings.Contains(render(t, m), "already exists") {
		t.Error("no flash explaining the refusal")
	}
}

func TestEditRefusesAnEmptyName(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+n"})
	press(m, []string{"enter"})
	if len(rec.libraries) != 0 {
		t.Error("an empty Command was saved")
	}
	if !strings.Contains(render(t, m), "Name is required") {
		t.Error("no flash explaining the refusal")
	}
}

// The confirm is inline: it takes over the Command's row and leaves the rest
// of the list screen — crucially the detail panel — standing, so you can see
// what you are deleting while you answer.
func TestDeleteConfirmsInlineWithoutLeavingTheList(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"ctrl+x"})

	if _, ok := m.screen.(*listScreen); !ok {
		t.Fatalf("^X left the list for %T", m.screen)
	}
	frame := render(t, m)
	if !strings.Contains(frame, "⚠ Delete 'deploy prod'? y/n") {
		t.Errorf("no inline confirm:\n%s", frame)
	}
	if !strings.Contains(frame, "ssh {{host=prod-1}} 'deploy.sh'") {
		t.Errorf("the detail panel stopped showing the command being deleted:\n%s", frame)
	}
	if !strings.Contains(frame, "y Delete") {
		t.Error("the footer does not offer the confirm keys")
	}
}

// Anything that is not "yes" keeps the Command — a destructive action does not
// get to read an unrelated keystroke as consent.
func TestDeleteConfirmTreatsAnyOtherKeyAsCancel(t *testing.T) {
	for _, key := range []string{"n", "x", "q"} {
		m, rec := harness(t)
		press(m, []string{"ctrl+x", key})
		if len(rec.libraries) != 0 {
			t.Errorf("%q deleted the Command", key)
		}
		if strings.Contains(render(t, m), "⚠ Delete 'deploy prod'? y/n") {
			t.Errorf("%q left the confirm open", key)
		}
	}
}

// The caret marks the one field that will receive a keystroke, and no other.
func TestOnlyTheFocusedFieldCarriesTheCaret(t *testing.T) {
	// Taken from the style rather than hand-written: lipgloss emits the reverse
	// attribute and the colour in one combined SGR, so neither can be matched
	// on its own.
	caret := strings.SplitN(caretStyle.Render("x"), "x", 2)[0]
	m, _ := harness(t)
	press(m, []string{"ctrl+n", "abc"})

	if got := strings.Count(m.View().Content, caret); got != 1 {
		t.Errorf("frame carries %d carets, want exactly 1", got)
	}
	press(m, []string{"tab"})
	if got := strings.Count(m.View().Content, caret); got != 1 {
		t.Errorf("after tab the frame carries %d carets, want exactly 1", got)
	}
}

func TestDeleteConfirmRemovesTheCommand(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+x", "y"})

	if len(rec.libraries) != 1 {
		t.Fatalf("saved %d libraries, want 1", len(rec.libraries))
	}
	for _, command := range rec.libraries[0].Commands {
		if command.ID == "id-deploy" {
			t.Error("the Command was not removed")
		}
	}
	if !strings.Contains(render(t, m), "Deleted 'deploy prod'") {
		t.Error("no flash after deleting")
	}
}

func TestDeleteCancelKeepsTheCommand(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+x", "n"})
	if len(rec.libraries) != 0 {
		t.Error("cancelling still wrote the Library")
	}
}

// A Placeholder written with no default is required, so both verbs refuse an
// empty one rather than handing the shell a command with a hole in it. An
// argument that may be left empty is written {{name=}} and is not caught here.
func TestTheArgFormRefusesAnEmptyRequiredArgument(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"tail", "enter"}) // tail logs asks for {{file}} and {{pattern=error}}

	press(m, []string{"enter"})
	if m.Handoff() != "" {
		t.Errorf("an empty required argument was handed off: %q", m.Handoff())
	}
	if _, still := m.screen.(*argsScreen); !still {
		t.Fatalf("the refusal left the arg form for %T", m.screen)
	}
	if frame := render(t, m); !strings.Contains(frame, "'file' is required") {
		t.Errorf("the form does not name the argument it is waiting for:\n%s", frame)
	}

	send(m, tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if len(rec.copied) != 0 {
		t.Errorf("copy went ahead with an empty required argument: %v", rec.copied)
	}

	press(m, []string{"log.txt", "enter"})
	if m.Handoff() != "tail -f log.txt | grep error" {
		t.Errorf("handoff = %q, want the filled-in command", m.Handoff())
	}
}

// esc is the reflexive way out of a screen, and an edited form is the one place
// it destroys work. So it asks first, and the second press is what discards.
func TestEscOnAnEditedFormAsksBeforeDiscarding(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"ctrl+o", "ctrl+u", "renamed"})

	press(m, []string{"esc"})
	if _, still := m.screen.(*editScreen); !still {
		t.Fatalf("the first esc threw the edit away for %T", m.screen)
	}
	if frame := render(t, m); !strings.Contains(frame, "esc again to discard") {
		t.Errorf("the form does not say why it stayed:\n%s", frame)
	}

	press(m, []string{"esc"})
	if _, ok := m.screen.(*listScreen); !ok {
		t.Errorf("the second esc landed on %T, want the list", m.screen)
	}
}

// The armed esc lasts one keystroke. Carrying on typing means the way out was
// not what the user was reaching for, and an esc minutes later is a fresh ask.
func TestAKeystrokeDisarmsTheDiscardGuard(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"ctrl+o", "ctrl+u", "renamed", "esc", "x", "esc"})

	if _, still := m.screen.(*editScreen); !still {
		t.Fatalf("an esc after a keystroke discarded the edit for %T", m.screen)
	}
}

// A paste changes the form as surely as typing does, and it arrives as its own
// message rather than as keystrokes. The guard has to see it, or an esc after a
// paste discards text the user was never warned about.
func TestAPasteDisarmsTheDiscardGuard(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"ctrl+o", "ctrl+u", "renamed", "esc"})
	send(m, tea.PasteMsg{Content: "-pasted"})
	press(m, []string{"esc"})

	if _, still := m.screen.(*editScreen); !still {
		t.Fatalf("an esc after a paste discarded the edit for %T", m.screen)
	}
}

// The list is where the user was, so a trip to a form and back has to return
// them to it rather than to a fresh one — a query retyped and a selection walked
// down again is the whole search done twice.
func TestAFormRoundTripKeepsTheQueryAndSelection(t *testing.T) {
	for name, trip := range map[string][]string{
		"the edit form": {"ctrl+o", "esc"},
		"the add form":  {"ctrl+n", "esc"},
		"the arg form":  {"enter", "esc"},
	} {
		m, _ := harness(t)
		press(m, []string{"o", "down"})
		before := m.screen.(*listScreen)

		press(m, trip)

		after, ok := m.screen.(*listScreen)
		if !ok {
			t.Errorf("%s: came back to %T, want the list", name, m.screen)
			continue
		}
		if after != before {
			t.Errorf("%s: came back to a different list screen", name)
		}
		if after.query.Value() != "o" {
			t.Errorf("%s: the query came back as %q", name, after.query.Value())
		}
		if after.sel != 1 {
			t.Errorf("%s: the selection came back as %d, want 1", name, after.sel)
		}
	}
}

func TestTypingFiltersTheList(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"ports"})
	frame := render(t, m)
	if !strings.Contains(frame, "list ports") {
		t.Error("the match is missing")
	}
	if strings.Contains(frame, "deploy prod") {
		t.Error("a non-match survived the filter")
	}
}

// The list selection resets when the query changes, but not on cursor motion.
func TestSelectionResetsOnEditButNotOnMotion(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"down"})
	list := m.screen.(*listScreen)
	if list.sel != 1 {
		t.Fatalf("sel = %d, want 1", list.sel)
	}
	send(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if list.sel != 1 {
		t.Errorf("cursor motion reset the selection to %d", list.sel)
	}
	press(m, []string{"o"})
	if list.sel != 0 {
		t.Errorf("typing did not reset the selection: %d", list.sel)
	}
}

// iTerm2's natural-text-editing preset sends 0x01 for ⌘← and 0x05 for ⌘→, which
// is byte for byte ^A and ^E. Nothing can tell them apart, so the search field
// has to mean line-start and line-end by them — the list's verbs are chords the
// field does not claim: ^N, ^O, ^X.
func TestCtrlAAndCtrlEAreLineStartAndEndInTheSearchField(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"ports"})
	list, ok := m.screen.(*listScreen)
	if !ok {
		t.Fatalf("screen = %T, want the list", m.screen)
	}

	press(m, []string{"ctrl+a"})
	if _, still := m.screen.(*listScreen); !still {
		t.Fatalf("^A in the search field opened %T", m.screen)
	}
	if got := list.query.Position(); got != 0 {
		t.Errorf("^A left the caret at %d, want line start", got)
	}

	press(m, []string{"ctrl+e"})
	if _, still := m.screen.(*listScreen); !still {
		t.Fatalf("^E in the search field opened %T", m.screen)
	}
	if got, want := list.query.Position(), len("ports"); got != want {
		t.Errorf("^E left the caret at %d, want %d", got, want)
	}
	if list.query.Value() != "ports" {
		t.Errorf("the query changed under a cursor motion: %q", list.query.Value())
	}
}

// The same two keys in the editor, where they mean line-start and line-end.
func TestCtrlAIsLineStartInTheEditor(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"ctrl+n"})
	edit, ok := m.screen.(*editScreen)
	if !ok {
		t.Fatalf("^N on the list screen opened %T, want the editor", m.screen)
	}
	press(m, []string{"abc", "ctrl+a"})
	if _, still := m.screen.(*editScreen); !still {
		t.Fatal("^A inside the editor opened another screen")
	}
	if edit.form.Field(fieldName).Position() != 0 {
		t.Errorf("^A did not move to line start: position %d", edit.form.Field(fieldName).Position())
	}
}

// Tab has no job on the list screen — the search field always has the keyboard
// — and it must stay a no-op rather than reach the field, where sanitisation
// would type it as a space.
func TestTabIsANoOpOnTheListScreen(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"tab", "shift+tab"})
	list := m.screen.(*listScreen)
	if !list.query.Focused() {
		t.Error("tab took the caret from the search field")
	}
	if list.query.Value() != "" {
		t.Errorf("tab typed into the query: %q", list.query.Value())
	}
	press(m, []string{"e"})
	if list.query.Value() != "e" {
		t.Errorf("query = %q, want the letter typed rather than an action", list.query.Value())
	}
}

// The guard that stops a future letter binding leaking into the key map: the
// search field always has the keyboard, so no verb may cost it a letter.
func TestSearchFieldNeverLosesALetter(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"adeyjkq"})
	if _, still := m.screen.(*listScreen); !still {
		t.Fatalf("a letter typed into the search field opened %T", m.screen)
	}
	if got := m.screen.(*listScreen).query.Value(); got != "adeyjkq" {
		t.Errorf("query = %q, want every letter to have reached the field", got)
	}
}

// An empty Library has nothing to search and one thing to do, and the panel
// filling the empty list says which chord does it.
func TestAnEmptyLibraryOffersTheAddChord(t *testing.T) {
	deps := fixtureDeps()
	deps.Library = emptyLibrary()
	m := New(deps)
	m.SetSize(80, 24)

	if frame := render(t, m); !strings.Contains(frame, "^N  Add your first command") {
		t.Errorf("the getting-started panel does not offer the add chord:\n%s", frame)
	}
	press(m, []string{"ctrl+n"})
	if _, ok := m.screen.(*editScreen); !ok {
		t.Fatalf("^N on a first run opened %T, want the editor", m.screen)
	}
}

// Either case confirms: a shifted y reports its own text, so it arrives as "Y".
func TestUppercaseYConfirmsTheDelete(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+x"})
	send(m, tea.KeyPressMsg{Code: 'y', Text: "Y", Mod: tea.ModShift})
	if len(rec.libraries) != 1 {
		t.Fatalf("Y saved %d libraries, want 1", len(rec.libraries))
	}
}

func TestEscapeQuitsWithoutHandingOff(t *testing.T) {
	m, _ := harness(t)
	send(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.quitting {
		t.Error("esc did not quit the list screen")
	}
	if m.Handoff() != "" {
		t.Errorf("handoff = %q, want empty", m.Handoff())
	}
}

// ^D stayed the field's forward-delete when Delete became ^X — the mnemonic
// chord belongs to readline, the same trade ^A and ^E made (see keys.go).
func TestCtrlDIsForwardDeleteNotDelete(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"ports", "ctrl+a", "ctrl+d"})
	list := m.screen.(*listScreen)
	if got := list.query.Value(); got != "orts" {
		t.Errorf("query = %q, want %q — ^D must forward-delete in the field", got, "orts")
	}
	if list.confirming != "" {
		t.Error("^D opened the delete confirm")
	}
}

// Deleting a Command drops its State too. State is disposable, so a miss costs
// nothing the user can see — the State simply survives in state.json forever,
// which is why it is worth pinning here.
func TestDeleteAlsoForgetsTheCommandsState(t *testing.T) {
	m, rec := harness(t)
	if _, ok := m.st["id-deploy"]; !ok {
		t.Fatal("the fixture has no State for the Command about to be deleted")
	}
	press(m, []string{"ctrl+x", "y"})

	if len(rec.states) == 0 {
		t.Fatal("no State was saved")
	}
	if _, ok := rec.states[len(rec.states)-1]["id-deploy"]; ok {
		t.Error("the deleted Command's State entry was kept")
	}
}

// A delete writes both files, and when both fail the Library's failure is the
// one the user is shown: commands.json is their data, state.json is a cache
// CONTEXT.md calls safe to delete. Raising each flash as its write returned put
// the *last* failure's text on screen — so the report named the cache and left
// the user thinking their Command was gone.
func TestADeleteThatFailsBothWritesReportsTheLibraryNotTheCache(t *testing.T) {
	m, rec := harness(t)
	rec.failWith = errors.New("commands.json is read-only")
	rec.failStateWith = errors.New("state.json is read-only")
	press(m, []string{"ctrl+x", "y"})

	frame := render(t, m)
	if strings.Contains(frame, "Deleted") {
		t.Errorf("a delete that wrote nothing flashed success:\n%s", frame)
	}
	if !strings.Contains(frame, "commands.json is read-only") {
		t.Errorf("the Library's failure is not the one reported:\n%s", frame)
	}
	if strings.Contains(frame, "state.json is read-only") {
		t.Errorf("the cache's failure won over the Library's:\n%s", frame)
	}
}
