package search

import (
	"slices"
	"testing"

	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/state"
)

// Search matches over name + description + command, name weighted highest,
// then description, then command. Empty query: MRU first (State keyed by id),
// never-used follow in file (array) order.

func describe(s string) *string { return &s }

var commands = []library.Command{
	{ID: "c1", Name: "deploy prod", Template: "ssh prod-1 deploy.sh", Description: describe("Roll out to production")},
	{ID: "c2", Name: "tail logs", Template: "aws logs tail /ecs/api --follow", Description: describe("Tail ECS logs")},
	{ID: "c3", Name: "docker nuke", Template: "docker system prune -af", Description: describe("Remove unused containers")},
	{ID: "c4", Name: "list ports", Template: "lsof -iTCP -sTCP:LISTEN", Description: describe("Show listening processes")},
}

func names(commands []library.Command) []string {
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		out = append(out, command.Name)
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
	commands := []library.Command{
		{ID: "a", Name: "a", Template: "echo listening"},
		{ID: "b", Name: "b", Template: "echo x", Description: describe("listening things")},
	}
	got := Commands(commands, state.State{}, "listening")
	if got[0].Name != "b" {
		t.Errorf("got %q first, want 'b'", got[0].Name)
	}
}

// hitsAt reads a NameHits result back as the rune indices it marks.
func hitsAt(hits []bool) []int {
	at := []int{}
	for i, hit := range hits {
		if hit {
			at = append(at, i)
		}
	}
	return at
}

func TestNameHits(t *testing.T) {
	hits := NameHits("dpl", "deploy prod")
	if len(hits) != len([]rune("deploy prod")) {
		t.Fatalf("got %d entries, want one per rune of the name", len(hits))
	}
	if got := hitsAt(hits); !slices.Equal(got, []int{0, 2, 3}) {
		t.Errorf("hits at %v, want [0 2 3]", got)
	}
}

func TestNameHitsIsCaseInsensitive(t *testing.T) {
	if got := hitsAt(NameHits("DP", "deploy prod")); !slices.Equal(got, []int{0, 2}) {
		t.Errorf("hits at %v, want [0 2]", got)
	}
}

func TestNameHitsMisses(t *testing.T) {
	for _, query := range []string{"zzz", "", "  "} {
		if hits := NameHits(query, "deploy prod"); hits != nil {
			t.Errorf("query %q reported hits %v", query, hits)
		}
	}
}
