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
	deps.Copy = func(text string) { rec.copied = append(rec.copied, text) }
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
	press(m, []string{"enter"}) // open the form for 'deploy prod'
	args := m.screen.(*argsScreen)
	args.inputs[0].SetValue("prod-9")
	press(m, []string{"enter"})

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

// What the add form is responsible for is the wiring: which field becomes which
// part of the Draft, that a save is attempted, and that the screen moves on.
// Whether the Library minted a fresh id and kept its array order is the
// Library's own promise, tested in library_test.go.
func TestAddHandsTheFormsFieldsToTheLibrary(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"tab", "a"})
	edit := m.screen.(*editScreen)
	edit.inputs[fieldName].SetValue("new one")
	edit.inputs[fieldDescription].SetValue("a description")
	edit.inputs[fieldCommand].SetValue("echo new")
	press(m, []string{"enter"})

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
	press(m, []string{"tab", "e"})
	m.screen.(*editScreen).inputs[fieldName].SetValue("renamed")
	press(m, []string{"enter"})

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
	press(m, []string{"tab", "a"})
	edit := m.screen.(*editScreen)
	edit.inputs[fieldName].SetValue("doomed")
	edit.inputs[fieldCommand].SetValue("echo x")
	press(m, []string{"enter"})

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
	press(m, []string{"tab", "a"})
	edit := m.screen.(*editScreen)
	edit.inputs[fieldName].SetValue("list ports")
	edit.inputs[fieldCommand].SetValue("echo x")
	press(m, []string{"enter"})

	if len(rec.libraries) != 0 {
		t.Error("a duplicate name was saved")
	}
	if !strings.Contains(render(t, m), "already exists") {
		t.Error("no flash explaining the refusal")
	}
}

func TestEditRefusesAnEmptyName(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"tab", "a"})
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
	press(m, []string{"tab", "d"})

	if _, ok := m.screen.(*listScreen); !ok {
		t.Fatalf("`d` left the list for %T", m.screen)
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
		press(m, []string{"tab", "d", key})
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
	press(m, []string{"tab", "a", "abc"})

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
	press(m, []string{"tab", "d", "y"})

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
	press(m, []string{"tab", "d", "n"})
	if len(rec.libraries) != 0 {
		t.Error("cancelling still wrote the Library")
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

// The bug this key map was rebuilt for: iTerm2's natural-text-editing preset
// sends 0x01 for ⌘← and 0x05 for ⌘→, which is byte for byte ^A and ^E. Nothing
// can tell them apart, so the search field has to mean line-start and line-end
// by them — the list's verbs live where the keyboard is not spelling anything.
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
	if got := list.input.Position(); got != 0 {
		t.Errorf("^A left the caret at %d, want line start", got)
	}

	press(m, []string{"ctrl+e"})
	if _, still := m.screen.(*listScreen); !still {
		t.Fatalf("^E in the search field opened %T", m.screen)
	}
	if got, want := list.input.Position(), len("ports"); got != want {
		t.Errorf("^E left the caret at %d, want %d", got, want)
	}
	if list.input.Value() != "ports" {
		t.Errorf("the query changed under a cursor motion: %q", list.input.Value())
	}
}

// The same two keys in the editor, where they have always meant this.
func TestCtrlAIsLineStartInTheEditor(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"tab", "a"})
	edit, ok := m.screen.(*editScreen)
	if !ok {
		t.Fatalf("`a` on the list screen opened %T, want the editor", m.screen)
	}
	edit.inputs[fieldName].SetValue("abc")
	press(m, []string{"ctrl+a"})
	if _, still := m.screen.(*editScreen); !still {
		t.Fatal("^A inside the editor opened another screen")
	}
	if edit.inputs[fieldName].Position() != 0 {
		t.Errorf("^A did not move to line start: position %d", edit.inputs[fieldName].Position())
	}
}

// The two zones, and the promise each makes: the list zone answers to letters
// and drops what it does not claim; the search zone never takes one.
func TestTabHandsTheKeyboardToTheListAndBack(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"tab"})
	list := m.screen.(*listScreen)
	if list.focus != focusList {
		t.Fatal("tab did not hand the keyboard to the list")
	}
	if list.input.Focused() {
		t.Error("the search field kept the caret after tab")
	}

	press(m, []string{"esc"})
	if _, still := m.screen.(*listScreen); !still {
		t.Fatalf("esc from the list zone left for %T, want the search field", m.screen)
	}
	if list.focus != focusSearch || !list.input.Focused() {
		t.Fatal("esc did not hand the keyboard back to the search field")
	}
	press(m, []string{"e"})
	if list.input.Value() != "e" {
		t.Errorf("query = %q, want the letter typed rather than an action", list.input.Value())
	}
}

func TestListFocusIgnoresKeysItDoesNotClaim(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"down", "tab", "z", "5", "?"})
	list := m.screen.(*listScreen)
	if _, still := m.screen.(*listScreen); !still {
		t.Fatalf("an unclaimed key opened %T", m.screen)
	}
	if list.input.Value() != "" {
		t.Errorf("a blurred field took a keystroke: %q", list.input.Value())
	}
	if list.sel != 1 {
		t.Errorf("sel = %d, want the selection left where it was", list.sel)
	}
}

// The guard that stops a future letter binding leaking into the search zone.
func TestSearchFocusNeverStealsALetter(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"adeyjkq"})
	if _, still := m.screen.(*listScreen); !still {
		t.Fatalf("a letter typed into the search field opened %T", m.screen)
	}
	if got := m.screen.(*listScreen).input.Value(); got != "adeyjkq" {
		t.Errorf("query = %q, want every letter to have reached the field", got)
	}
}

func TestJAndKWalkTheListOnlyInListFocus(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"j"})
	list := m.screen.(*listScreen)
	if list.input.Value() != "j" || list.sel != 0 {
		t.Errorf("j in the search field: query %q, sel %d — want it typed", list.input.Value(), list.sel)
	}

	m, _ = harness(t)
	press(m, []string{"tab", "j"})
	if got := m.screen.(*listScreen).sel; got != 1 {
		t.Errorf("j in the list zone left sel at %d, want 1", got)
	}
	press(m, []string{"k"})
	if got := m.screen.(*listScreen).sel; got != 0 {
		t.Errorf("k in the list zone left sel at %d, want 0", got)
	}
}

// An empty Library has nothing to search and one thing to do.
func TestAnEmptyLibraryStartsInTheListZone(t *testing.T) {
	deps := fixtureDeps()
	deps.Library = emptyLibrary()
	m := New(deps)
	m.SetSize(80, 24)

	if m.screen.(*listScreen).focus != focusList {
		t.Fatal("an empty Library did not start in the list zone")
	}
	if frame := render(t, m); !strings.Contains(frame, "a  Add your first command") {
		t.Errorf("the getting-started panel does not offer a one-key add:\n%s", frame)
	}
	press(m, []string{"a"})
	if _, ok := m.screen.(*editScreen); !ok {
		t.Fatalf("`a` on a first run opened %T, want the editor", m.screen)
	}
}

// esc steps out one level: out of the list zone, then out of potato.
func TestEscapeStepsOutOneLevel(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"tab", "esc"})
	if m.quitting {
		t.Fatal("esc from the list zone quit rather than returning to the field")
	}
	press(m, []string{"esc"})
	if !m.quitting {
		t.Error("esc from the search field did not quit")
	}
}

// Either case confirms: a shifted y reports its own text, so it arrives as "Y".
func TestUppercaseYConfirmsTheDelete(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"tab", "d"})
	send(m, tea.KeyPressMsg{Code: 'y', Text: "Y", Mod: tea.ModShift})
	if len(rec.libraries) != 1 {
		t.Fatalf("Y saved %d libraries, want 1", len(rec.libraries))
	}
}

func TestEscapeQuitsWithoutHandingOff(t *testing.T) {
	m, _ := harness(t)
	send(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.Handoff() != "" {
		t.Errorf("handoff = %q, want empty", m.Handoff())
	}
}

// Deleting a Command drops its State command too. State is disposable, so this
// costs nothing when it is missed — which is exactly why it was missed, and why
// it is worth pinning: the command used to survive in state.json forever.
func TestDeleteAlsoForgetsTheCommandsState(t *testing.T) {
	m, rec := harness(t)
	if _, ok := m.st["id-deploy"]; !ok {
		t.Fatal("the fixture has no State for the Command about to be deleted")
	}
	press(m, []string{"tab", "d", "y"})

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
	press(m, []string{"tab", "d", "y"})

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
