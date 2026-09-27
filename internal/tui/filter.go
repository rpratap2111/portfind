package tui

import (
	"strconv"
	"strings"

	"github.com/rpratap2111/portfind/internal/inspector"
)

// filterEntries returns the entries matching query, preserving order. The
// query is split on whitespace and every term must match, so "node 30" finds
// node processes on ports containing "30".
func filterEntries(entries []inspector.PortEntry, query string) []inspector.PortEntry {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return entries
	}
	var out []inspector.PortEntry
	for _, e := range entries {
		if matchesAll(e, terms) {
			out = append(out, e)
		}
	}
	return out
}

func matchesAll(e inspector.PortEntry, terms []string) bool {
	for _, t := range terms {
		if !matches(e, t) {
			return false
		}
	}
	return true
}

// matches checks one lower-cased term. Ports match by substring, since a
// fuzzy "36" matching port 3306 would be noise; names match fuzzily.
func matches(e inspector.PortEntry, term string) bool {
	return strings.Contains(strconv.Itoa(e.Port), term) ||
		fuzzyMatch(strings.ToLower(e.Process), term) ||
		fuzzyMatch(strings.ToLower(e.ProjectName), term)
}

// fuzzyMatch reports whether pattern's characters appear in s in order,
// e.g. "dwa" matches "demo-web-app".
func fuzzyMatch(s, pattern string) bool {
	if s == "" {
		return false
	}
	rest := s
	for _, r := range pattern {
		i := strings.IndexRune(rest, r)
		if i < 0 {
			return false
		}
		rest = rest[i+len(string(r)):]
	}
	return true
}
