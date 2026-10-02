// Package risk assigns each listening process a tier that decides how much
// confirmation a kill requires.
//
// Classification is deliberately simple substring matching on process names.
// It is a guard rail against killing the wrong thing, not a security boundary.
package risk

import (
	"strings"

	"github.com/rpratap2111/portfind/internal/inspector"
)

// Tiers, from least to most confirmation required.
const (
	Low    = "LOW"    // dev-server-shaped processes: quick y/N
	Medium = "MEDIUM" // unrecognized processes and system services
	High   = "HIGH"   // databases, and anything launched over SSH
)

// highRiskNames are matched as substrings of the process name, so "mysql"
// also covers "mysqld" and "postgres" covers "postgres.exe" workers.
var highRiskNames = []string{"postgres", "mysql", "mongod", "redis-server", "sqlservr"}

// highRiskParents are matched exactly: a process started by one of these is
// likely someone's remote session.
var highRiskParents = []string{"sshd", "ssh"}

// lowRiskNames are matched as substrings of the process name.
var lowRiskNames = []string{"node", "python", "ruby", "java", "dlv"}

// lowRiskParents are matched exactly. `go run` builds a temp binary with an
// arbitrary name and runs it as a child of go.exe, so the parent is the tell.
var lowRiskParents = []string{"go"}

// highRiskImages are matched as substrings of a container's image and name.
// Images are named after the product ("mongo", "redis"), not the server
// binary ("mongod", "redis-server").
var highRiskImages = []string{"postgres", "mysql", "mariadb", "mongo", "redis", "mssql", "sqlserver", "elasticsearch"}

// ClassifyContainer returns the tier for a Docker container. Databases are
// HIGH; everything else is MEDIUM, never LOW, because stopping a container
// takes down everything in it and always deserves a typed confirmation.
func ClassifyContainer(name, image string) string {
	if containsAny(strings.ToLower(name+" "+image), highRiskImages) {
		return High
	}
	return Medium
}

// Classify returns the tier for a process given its name and its parent's
// name (either may be empty when unknown). HIGH rules are checked first, so a
// dev server started over SSH is still HIGH.
func Classify(process, parent string) string {
	process = strings.ToLower(process)
	parent = strings.ToLower(parent)
	switch {
	case containsAny(process, highRiskNames), equalsAny(parent, highRiskParents):
		return High
	case containsAny(process, lowRiskNames), equalsAny(parent, lowRiskParents):
		return Low
	default:
		return Medium
	}
}

// Annotate sets RiskTier on each entry in place.
func Annotate(entries []inspector.PortEntry) {
	for i := range entries {
		e := &entries[i]
		if e.IsContainer() {
			e.RiskTier = ClassifyContainer(e.Process, e.Image)
			continue
		}
		e.RiskTier = Classify(e.Process, e.ParentProcess)
	}
}

func containsAny(s string, subs []string) bool {
	if s == "" {
		return false
	}
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func equalsAny(s string, candidates []string) bool {
	for _, c := range candidates {
		if s == c {
			return true
		}
	}
	return false
}
