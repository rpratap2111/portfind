package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/scan"
)

func TestWriteJSON(t *testing.T) {
	res := scan.Result{
		Entries: []inspector.PortEntry{
			{Port: 3000, PID: 42, Process: "node", ProjectName: "my-app", AgeSeconds: 90, RiskTier: "LOW",
				Command: `"C:\node.exe" server.js --x<y&z`, ParentPID: 7, ParentProcess: "cmd"},
			{Port: 5432, PID: 8804, Process: "postgres", AgeSeconds: -1, RiskTier: "HIGH"}, // access denied: unknowns
		},
		Warnings: []error{errors.New("port 5432 pid 8804 (postgres): OpenProcess: Access is denied.")},
	}
	var buf bytes.Buffer
	if err := writeJSON(&buf, res); err != nil {
		t.Fatal(err)
	}

	// Decode generically to check the exact shape scripts will see.
	var got struct {
		Ports    []map[string]any `json:"ports"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(got.Ports) != 2 || len(got.Warnings) != 1 {
		t.Fatalf("got %d ports, %d warnings", len(got.Ports), len(got.Warnings))
	}

	node := got.Ports[0]
	wantNode := map[string]any{
		"port": 3000.0, "pid": 42.0, "process": "node", "project": "my-app", "age_seconds": 90.0,
		"risk": "LOW", "command": `"C:\node.exe" server.js --x<y&z`, "parent_pid": 7.0, "parent_process": "cmd",
	}
	for k, want := range wantNode {
		if node[k] != want {
			t.Errorf("node[%q] = %#v, want %#v", k, node[k], want)
		}
	}

	pg := got.Ports[1]
	for _, k := range []string{"project", "age_seconds", "command", "parent_pid", "parent_process"} {
		v, present := pg[k]
		if !present || v != nil {
			t.Errorf("postgres[%q] = %#v (present %v), want an explicit null", k, v, present)
		}
	}
	if len(pg) != len(node) {
		t.Errorf("entries have different key sets: %d vs %d keys", len(pg), len(node))
	}

	if bytes.Contains(buf.Bytes(), []byte(`\u003c`)) {
		t.Error("command lines should not be HTML-escaped")
	}
}

func TestWriteJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, scan.Result{}); err != nil {
		t.Fatal(err)
	}
	// Empty arrays, not null, so `.ports[]` and ForEach work without checks.
	if got := buf.String(); got != "{\n  \"ports\": [],\n  \"warnings\": []\n}\n" {
		t.Fatalf("empty report = %q", got)
	}
}
