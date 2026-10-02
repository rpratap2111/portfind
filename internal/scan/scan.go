// Package scan runs the full pipeline shared by every front-end: list
// listening ports, then annotate each entry with its project, its Docker
// container (if any) and its risk tier.
package scan

import (
	"github.com/rpratap2111/portfind/internal/docker"
	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/provenance"
	"github.com/rpratap2111/portfind/internal/risk"
)

// Result is one fully annotated scan.
type Result struct {
	Entries  []inspector.PortEntry
	Warnings []error // non-fatal, per-process problems; surface these to the user
}

// Run scans with ins and annotates the entries. A non-nil error means the
// scan itself failed and no entries are available.
func Run(ins inspector.PortInspector) (Result, error) {
	res, err := ins.Scan()
	if err != nil {
		return Result{}, err
	}
	warnings := append(res.Warnings, provenance.Annotate(res.Entries)...)
	// Docker comes after provenance (a container's Compose project replaces
	// whatever Docker's own process resolved to) and before risk (containers
	// are tiered by image).
	entries, dockerWarnings := docker.Annotate(res.Entries)
	warnings = append(warnings, dockerWarnings...)
	risk.Annotate(entries)
	return Result{Entries: entries, Warnings: warnings}, nil
}
