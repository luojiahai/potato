package tui

import (
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
}

func harness(t *testing.T) (*Model, *recorder) {
	t.Helper()
	rec := &recorder{}
	deps := fixtureDeps()
	deps.SaveLibrary = func(lib library.Library) { rec.libraries = append(rec.libraries, lib) }
	deps.SaveState = func(s state.State) { rec.states = append(rec.states, s) }
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
	if !strings.Contains(render(t, m), "copied to clipboard") {
		t.Error("no flash after copying")
	}
}

func TestAddCreatesACommandWithAFreshID(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+a"})
	edit := m.screen.(*editScreen)
	edit.inputs[0].SetValue("new one")
	edit.inputs[1].SetValue("echo new")
	press(m, []string{"enter"})

	if len(rec.libraries) != 1 {
		t.Fatalf("saved %d libraries, want 1", len(rec.libraries))
	}
	saved := rec.libraries[0]
	if len(saved.Commands) != 4 {
		t.Fatalf("got %d commands, want 4", len(saved.Commands))
	}
	added := saved.Commands[3]
	if added.Name != "new one" || added.Command != "echo new" {
		t.Errorf("added = %+v", added)
	}
	if added.ID == "" || added.ID == "id-deploy" {
		t.Errorf("id = %q", added.ID)
	}
	if _, ok := m.screen.(*listScreen); !ok {
		t.Errorf("screen = %T, want the list", m.screen)
	}
}

func TestEditRenameKeepsTheIDAndTheSlot(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+e"})
	m.screen.(*editScreen).inputs[0].SetValue("renamed")
	press(m, []string{"enter"})

	saved := rec.libraries[0]
	if saved.Commands[0].ID != "id-deploy" {
		t.Errorf("id changed: %q", saved.Commands[0].ID)
	}
	if saved.Commands[0].Name != "renamed" {
		t.Errorf("name = %q", saved.Commands[0].Name)
	}
	if len(saved.Commands) != 3 {
		t.Errorf("slot not kept: %d commands", len(saved.Commands))
	}
}

func TestEditRefusesADuplicateName(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+a"})
	edit := m.screen.(*editScreen)
	edit.inputs[0].SetValue("list ports")
	edit.inputs[1].SetValue("echo x")
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
	press(m, []string{"ctrl+a"})
	press(m, []string{"enter"})
	if len(rec.libraries) != 0 {
		t.Error("an empty Command was saved")
	}
	if !strings.Contains(render(t, m), "name is required") {
		t.Error("no flash explaining the refusal")
	}
}

func TestDeleteConfirmRemovesTheCommand(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+d", "y"})

	if len(rec.libraries) != 1 {
		t.Fatalf("saved %d libraries, want 1", len(rec.libraries))
	}
	for _, entry := range rec.libraries[0].Commands {
		if entry.ID == "id-deploy" {
			t.Error("the Command was not removed")
		}
	}
	if !strings.Contains(render(t, m), "deleted 'deploy prod'") {
		t.Error("no flash after deleting")
	}
}

func TestDeleteCancelKeepsTheCommand(t *testing.T) {
	m, rec := harness(t)
	press(m, []string{"ctrl+d", "n"})
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

// ^A and ^E are add and edit on the list screen, and line-start / line-end in
// the editing screens — the one place the two meanings must not collide.
func TestCtrlAIsAddOnTheListAndLineStartInTheEditor(t *testing.T) {
	m, _ := harness(t)
	press(m, []string{"ctrl+a"})
	edit, ok := m.screen.(*editScreen)
	if !ok {
		t.Fatalf("^A on the list screen opened %T, want the editor", m.screen)
	}
	edit.inputs[0].SetValue("abc")
	press(m, []string{"ctrl+a"})
	if _, still := m.screen.(*editScreen); !still {
		t.Fatal("^A inside the editor opened another screen")
	}
	if edit.inputs[0].Position() != 0 {
		t.Errorf("^A did not move to line start: position %d", edit.inputs[0].Position())
	}
}

func TestEscapeQuitsWithoutHandingOff(t *testing.T) {
	m, _ := harness(t)
	send(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.Handoff() != "" {
		t.Errorf("handoff = %q, want empty", m.Handoff())
	}
}

func TestMigratedLaunchShowsAToast(t *testing.T) {
	deps := fixtureDeps()
	deps.Migrated = true
	m := New(deps)
	m.SetSize(80, 24)
	m.Init()
	if !strings.Contains(render(t, m), "upgraded your library to v2") {
		t.Error("no migration toast")
	}
}
