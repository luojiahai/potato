package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The ring used to be written out once per screen, so these assertions could
// only be made against one screen at a time and were in practice made against
// neither. Tab belongs to the Form; this is where it is tested.

func threeFields() form {
	return newForm(newField(fieldWrap), newField(fieldWrap), newField(fieldWrap))
}

func TestTabWalksTheRingAndWrapsAround(t *testing.T) {
	f := threeFields()
	tab := tea.KeyPressMsg{Code: tea.KeyTab}

	for i, want := range []int{1, 2, 0} {
		f.Update(tab)
		if got := f.Focused(); got != want {
			t.Fatalf("tab %d left the keyboard on Field %d, want %d", i+1, got, want)
		}
	}
}

func TestShiftTabWalksItBackwards(t *testing.T) {
	f := threeFields()
	shiftTab := tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}

	// Straight off the front on the first press: the ring wraps both ways, so
	// Shift-Tab from the first Field is the last one.
	for i, want := range []int{2, 1, 0} {
		f.Update(shiftTab)
		if got := f.Focused(); got != want {
			t.Fatalf("shift-tab %d left the keyboard on Field %d, want %d", i+1, got, want)
		}
	}
}

// Only the Field with the keyboard takes a keystroke. A blurred one taking
// characters would be a Form whose Tab order was decorative.
func TestOnlyTheFocusedFieldTakesAKeystroke(t *testing.T) {
	f := threeFields()

	f.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	f.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})

	for i, want := range []string{"a", "b", ""} {
		if got := f.Field(i).Value(); got != want {
			t.Errorf("Field %d holds %q, want %q", i, got, want)
		}
	}
}

// A Form with no Fields is what the arg screen would be for a Command with no
// Placeholders. It cannot happen — the list screen only opens that screen for a
// Command that has some — but a ring with nothing in it is one modulo away from
// a panic, and the guard is cheaper than the reasoning.
func TestAnEmptyFormTakesAKeystrokeWithoutPanicking(t *testing.T) {
	f := newForm()

	f.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if got := f.Focused(); got != 0 {
		t.Errorf("an empty Form focused Field %d", got)
	}
}
