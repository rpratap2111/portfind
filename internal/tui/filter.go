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
// Containers also match the word "docker" and their image name.
func matches(e inspector.PortEntry, term string) bool {
	if strings.Contains(strconv.Itoa(e.Port), term) ||
		fuzzyMatch(strings.ToLower(e.Process), term) ||
		fuzzyMatch(strings.ToLower(e.ProjectName), term) {
		return true
	}
	return e.IsContainer() && (isDockerKeyword(term) || strings.Contains(strings.ToLower(e.Image), term))
}

// isDockerKeyword reports whether term is "docker" or the start of it
// ("dock"). Two letters aren't enough: "do" is too common in names.
func isDockerKeyword(term string) bool {
	return len(term) >= 3 && strings.HasPrefix("docker", term)
}

// sectioned orders entries for display: ordinary processes first, then
// Docker containers, each keeping its port order. The table draws them as two
// headed sections (see tableLayout).
func sectioned(entries []inspector.PortEntry) []inspector.PortEntry {
	containers := 0
	for _, e := range entries {
		if e.IsContainer() {
			containers++
		}
	}
	if containers == 0 || containers == len(entries) {
		return entries
	}
	out := make([]inspector.PortEntry, 0, len(entries))
	for _, e := range entries {
		if !e.IsContainer() {
			out = append(out, e)
		}
	}
	for _, e := range entries {
		if e.IsContainer() {
			out = append(out, e)
		}
	}
	return out
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
