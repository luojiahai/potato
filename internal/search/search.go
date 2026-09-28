// Package search is the fuzzy search over the Library:
// subsequence match on name + description + command text, name hits weighted
// highest, then description, then command. An empty query is MRU first
// (State.LastUsedAt), never-used in file order.
package search

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/luojiahai/potato/internal/library"
	"github.com/luojiahai/potato/internal/state"
)

// subsequenceScore scores a subsequence match; higher is better, and ok=false
// means no match. Consecutive runs score higher, with a slight bias toward
// shorter targets.
func subsequenceScore(query, text string) (float64, bool) {
	t := []rune(strings.ToLower(text))
	ti := 0
	score := 0.0
	lastMatch := -2
	for _, r := range strings.ToLower(query) {
		found := indexRuneFrom(t, r, ti)
		if found < 0 {
			return 0, false
		}
		if found == lastMatch+1 {
			score += 3
		} else {
			score++
		}
		lastMatch = found
		ti = found + 1
	}
	return score - float64(len(t))/100, true
}

func indexRuneFrom(t []rune, r rune, from int) int {
	for i := from; i < len(t); i++ {
		if t[i] == r {
			return i
		}
	}
	return -1
}

// NameHits marks the runes of name that query's greedy subsequence match lands
// on — the same walk the scorer takes — for match highlighting in the TUI
// list. It has one entry per rune of name, so it indexes the way the name
// does. It is nil for an empty query or one that misses the name; the row may
// still have matched on its description or command.
func NameHits(query, name string) []bool {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	t := []rune(strings.ToLower(name))
	hits := make([]bool, len(t))
	ti := 0
	for _, r := range strings.ToLower(query) {
		found := indexRuneFrom(t, r, ti)
		if found < 0 {
			return nil
		}
		hits[found] = true
		ti = found + 1
	}
	return hits
}

func Commands(commands []library.Command, s state.State, query string) []library.Command {
	if strings.TrimSpace(query) == "" {
		used := []library.Command{}
		rest := []library.Command{}
		for _, command := range commands {
			if s[command.ID].LastUsedAt != "" {
				used = append(used, command)
			} else {
				rest = append(rest, command)
			}
		}
		sort.SliceStable(used, func(i, j int) bool {
			return parseTime(s[used[j].ID].LastUsedAt).Before(parseTime(s[used[i].ID].LastUsedAt))
		})
		return append(used, rest...)
	}

	type scored struct {
		command library.Command
		score   float64
	}
	out := []scored{}
	for _, command := range commands {
		best := math.Inf(-1)
		if score, ok := subsequenceScore(query, command.Name); ok {
			best = math.Max(best, score*100)
		}
		if command.Description != nil {
			if score, ok := subsequenceScore(query, *command.Description); ok {
				best = math.Max(best, score*10)
			}
		}
		if score, ok := subsequenceScore(query, command.Template); ok {
			best = math.Max(best, score)
		}
		if !math.IsInf(best, -1) {
			out = append(out, scored{command: command, score: best})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })

	list := make([]library.Command, 0, len(out))
	for _, s := range out {
		list = append(list, s.command)
	}
	return list
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
