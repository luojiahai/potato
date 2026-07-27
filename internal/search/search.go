// Package search is the fuzzy search over the Library (spec §3.1):
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
	q := []rune(strings.ToLower(query))
	t := []rune(strings.ToLower(text))
	ti := 0
	score := 0.0
	lastMatch := -2
	for _, r := range q {
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

// NameMatchIndices returns the greedy subsequence match positions of query
// within name — the same walk the scorer takes — for match highlighting in
// the TUI list. ok=false means no name match (the row may still have matched
// via description/command) or an empty query.
func NameMatchIndices(query, name string) (map[int]bool, bool) {
	if strings.TrimSpace(query) == "" {
		return nil, false
	}
	q := []rune(strings.ToLower(query))
	t := []rune(strings.ToLower(name))
	indices := map[int]bool{}
	ti := 0
	for _, r := range q {
		found := indexRuneFrom(t, r, ti)
		if found < 0 {
			return nil, false
		}
		indices[found] = true
		ti = found + 1
	}
	return indices, true
}

func Commands(commands []library.Entry, s state.State, query string) []library.Entry {
	if strings.TrimSpace(query) == "" {
		used := []library.Entry{}
		rest := []library.Entry{}
		for _, entry := range commands {
			if s[entry.ID].LastUsedAt != "" {
				used = append(used, entry)
			} else {
				rest = append(rest, entry)
			}
		}
		sort.SliceStable(used, func(i, j int) bool {
			return parseTime(s[used[j].ID].LastUsedAt).Before(parseTime(s[used[i].ID].LastUsedAt))
		})
		return append(used, rest...)
	}

	type scored struct {
		entry library.Entry
		score float64
	}
	out := []scored{}
	for _, entry := range commands {
		best := math.Inf(-1)
		if score, ok := subsequenceScore(query, entry.Name); ok {
			best = math.Max(best, score*100)
		}
		if entry.Description != nil {
			if score, ok := subsequenceScore(query, *entry.Description); ok {
				best = math.Max(best, score*10)
			}
		}
		if score, ok := subsequenceScore(query, entry.Command); ok {
			best = math.Max(best, score)
		}
		if !math.IsInf(best, -1) {
			out = append(out, scored{entry: entry, score: best})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })

	entries := make([]library.Entry, 0, len(out))
	for _, s := range out {
		entries = append(entries, s.entry)
	}
	return entries
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
