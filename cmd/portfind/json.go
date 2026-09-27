package main

import (
	"encoding/json"
	"io"

	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/scan"
)

// jsonReport is the --json output. Every key is always present; values that
// portfind couldn't determine are null rather than omitted, so scripts can
// rely on the shape.
type jsonReport struct {
	Ports    []jsonPort `json:"ports"`
	Warnings []string   `json:"warnings"` // e.g. processes Windows denied access to
}

type jsonPort struct {
	Port          int     `json:"port"`
	PID           int     `json:"pid"`
	Process       string  `json:"process"`
	Project       *string `json:"project"`
	AgeSeconds    *int64  `json:"age_seconds"`
	Risk          string  `json:"risk"`
	Command       *string `json:"command"`
	ParentPID     *int    `json:"parent_pid"`
	ParentProcess *string `json:"parent_process"`
}

// writeJSON writes a scan as indented JSON.
func writeJSON(w io.Writer, res scan.Result) error {
	report := jsonReport{Ports: []jsonPort{}, Warnings: []string{}}
	for _, e := range res.Entries {
		report.Ports = append(report.Ports, toJSONPort(e))
	}
	for _, err := range res.Warnings {
		report.Warnings = append(report.Warnings, err.Error())
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false) // command lines contain < > & more often than HTML does
	return enc.Encode(report)
}

func toJSONPort(e inspector.PortEntry) jsonPort {
	p := jsonPort{
		Port:          e.Port,
		PID:           e.PID,
		Process:       e.Process,
		Risk:          e.RiskTier,
		Project:       nonEmpty(e.ProjectName),
		Command:       nonEmpty(e.Command),
		ParentProcess: nonEmpty(e.ParentProcess),
	}
	if e.AgeSeconds >= 0 {
		age := e.AgeSeconds
		p.AgeSeconds = &age
	}
	if e.ParentPID > 0 {
		ppid := e.ParentPID
		p.ParentPID = &ppid
	}
	return p
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
