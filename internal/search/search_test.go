package search

import (
	"testing"

	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/state"
)

// Spec §3.1: match over name + description + command, name weighted highest,
// then description, then command. Empty query: MRU first (State keyed by id),
// never-used follow in file (array) order.

func describe(s string) *string { return &s }

var commands = []library.Entry{
	{ID: "c1", Name: "deploy prod", Command: "ssh prod-1 deploy.sh", Description: describe("Roll out to production")},
	{ID: "c2", Name: "tail logs", Command: "aws logs tail /ecs/api --follow", Description: describe("Tail ECS logs")},
	{ID: "c3", Name: "docker nuke", Command: "docker system prune -af", Description: describe("Remove unused containers")},
	{ID: "c4", Name: "list ports", Command: "lsof -iTCP -sTCP:LISTEN", Description: describe("Show listening processes")},
}

func names(entries []library.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEmptyQueryMRUFirst(t *testing.T) {
	s := state.State{
		"c3": {LastUsedAt: "2026-07-20T00:00:00Z"},
		"c2": {LastUsedAt: "2026-07-23T00:00:00Z"},
	}
	want := []string{"tail logs", "docker nuke", "deploy prod", "list ports"}
	if got := names(Commands(commands, s, "")); !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEmptyQueryNoStateKeepsFileOrder(t *testing.T) {
	want := []string{"deploy prod", "tail logs", "docker nuke", "list ports"}
	if got := names(Commands(commands, state.State{}, "")); !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSubsequenceMatchOnName(t *testing.T) {
	got := names(Commands(commands, state.State{}, "dpl"))
	found := false
	for _, name := range got {
		if name == "deploy prod" {
			found = true
		}
	}
	if !found {
		t.Errorf("got %v, want it to contain 'deploy prod'", got)
	}
}

func TestNonMatchingFilteredOut(t *testing.T) {
	if got := Commands(commands, state.State{}, "zzzz"); len(got) != 0 {
		t.Errorf("got %v, want none", names(got))
	}
}

func TestNameHitOutranksDescriptionHit(t *testing.T) {
	got := Commands(commands, state.State{}, "tail")
	if got[0].Name != "tail logs" {
		t.Errorf("got %q first, want 'tail logs'", got[0].Name)
	}
}

func TestDescriptionHitOutranksCommandHit(t *testing.T) {
	entries := []library.Entry{
		{ID: "a", Name: "a", Command: "echo listening"},
		{ID: "b", Name: "b", Command: "echo x", Description: describe("listening things")},
	}
	got := Commands(entries, state.State{}, "listening")
	if got[0].Name != "b" {
		t.Errorf("got %q first, want 'b'", got[0].Name)
	}
}

func TestNameMatchIndices(t *testing.T) {
	indices, ok := NameMatchIndices("dpl", "deploy prod")
	if !ok {
		t.Fatal("expected a match")
	}
	for _, i := range []int{0, 2, 3} {
		if !indices[i] {
			t.Errorf("index %d not matched: %v", i, indices)
		}
	}
	if len(indices) != 3 {
		t.Errorf("got %v, want exactly {0,2,3}", indices)
	}
}

func TestNameMatchIndicesIsCaseInsensitive(t *testing.T) {
	indices, ok := NameMatchIndices("DP", "deploy prod")
	if !ok || !indices[0] || !indices[2] || len(indices) != 2 {
		t.Errorf("got %v ok=%v, want {0,2}", indices, ok)
	}
}

func TestNameMatchIndicesMisses(t *testing.T) {
	for _, query := range []string{"zzz", "", "  "} {
		if _, ok := NameMatchIndices(query, "deploy prod"); ok {
			t.Errorf("query %q reported a match", query)
		}
	}
}
