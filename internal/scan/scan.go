// Package scan runs the full pipeline shared by every front-end: list
// listening ports, then annotate each entry with its project and risk tier.
package scan

import (
	"portfind/internal/inspector"
	"portfind/internal/provenance"
	"portfind/internal/risk"
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
	risk.Annotate(res.Entries)
	return Result{Entries: res.Entries, Warnings: warnings}, nil
}
