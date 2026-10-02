package kill

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rpratap2111/portfind/internal/inspector"
)

func TestTargetOf(t *testing.T) {
	e := inspector.PortEntry{Port: 8080, PID: 700, Process: "shop-web-1", ContainerID: "aaa111"}
	if got, want := TargetOf(e), (Target{PID: 700, Port: 8080, Process: "shop-web-1", ContainerID: "aaa111"}); got != want {
		t.Fatalf("TargetOf = %+v, want %+v", got, want)
	}
}

func TestCheckAllowedContainers(t *testing.T) {
	checkAllowCases(t, []allowCase{
		// On Linux, docker-proxy belongs to root, so the row has PID 0.
		{"container behind an unseen PID", Target{PID: 0, Port: 5432, Process: "pg", ContainerID: "bbb"}, true},
		// A container may be named after a critical host process.
		{"container named like a critical process", Target{PID: 700, Port: 22, Process: "sshd", ContainerID: "ccc"}, true},
		{"container named svchost", Target{PID: 700, Port: 135, Process: "svchost", ContainerID: "ddd"}, true},
	})
}

// panicInspector fails the test if the process-kill path is taken.
type panicInspector struct{ t *testing.T }

func (p panicInspector) Scan() (inspector.ScanResult, error) {
	p.t.Fatal("Scan called for a container target")
	return inspector.ScanResult{}, nil
}

func (p panicInspector) IsListening(int, int) (bool, error) {
	p.t.Fatal("IsListening called for a container target: the host process must not be touched")
	return false, nil
}

func TestTerminateStopsTheContainerNotTheProcess(t *testing.T) {
	var stopped string
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"Id":"aaa111","Names":["/shop-web-1"],"Image":"nginx","Ports":[{"PublicPort":8080,"Type":"tcp"}]}]`))
	})
	mux.HandleFunc("/containers/aaa111/stop", func(w http.ResponseWriter, r *http.Request) {
		stopped = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))

	// PID 4 would be refused, and 0 can't be signalled: neither matters here.
	err := Terminate(panicInspector{t}, Target{PID: 4, Port: 8080, Process: "shop-web-1", ContainerID: "aaa111"})
	if err != nil {
		t.Fatal(err)
	}
	if stopped != "/containers/aaa111/stop" {
		t.Fatalf("stop request = %q", stopped)
	}
}
