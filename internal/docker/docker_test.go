package docker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/rpratap2111/portfind/internal/inspector"
)

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		in, network, addr string
		ok                bool
	}{
		{"unix:///var/run/docker.sock", "unix", "/var/run/docker.sock", true},
		{"npipe:////./pipe/docker_engine", "npipe", `\\.\pipe\docker_engine`, true},
		{"tcp://127.0.0.1:2375", "tcp", "127.0.0.1:2375", true},
		{"ssh://user@host", "", "", false},
		{"/var/run/docker.sock", "", "", false},
	}
	for _, tt := range tests {
		network, addr, err := parseEndpoint(tt.in)
		if (err == nil) != tt.ok || network != tt.network || addr != tt.addr {
			t.Errorf("parseEndpoint(%q) = %q, %q, %v", tt.in, network, addr, err)
		}
	}
}

// A real /containers/json response, trimmed: one Compose service publishing
// 8080 on IPv4 and IPv6, one database, one container publishing nothing.
const containersJSON = `[
 {"Id":"aaa111","Names":["/shop-web-1"],"Image":"nginx:1.27","Status":"Up 2 hours (healthy)",
  "Labels":{"com.docker.compose.project":"shop","com.docker.compose.service":"web"},
  "Ports":[{"IP":"0.0.0.0","PrivatePort":80,"PublicPort":8080,"Type":"tcp"},
           {"IP":"::","PrivatePort":80,"PublicPort":8080,"Type":"tcp"},
           {"PrivatePort":443,"Type":"tcp"}]},
 {"Id":"bbb222","Names":["/pg"],"Image":"postgres:16","Status":"Up 3 days","Labels":{},
  "Ports":[{"IP":"127.0.0.1","PrivatePort":5432,"PublicPort":5432,"Type":"tcp"},
           {"IP":"0.0.0.0","PrivatePort":53,"PublicPort":5353,"Type":"udp"}]},
 {"Id":"ccc333","Names":["/worker"],"Image":"busybox","Status":"Restarting (1) 4 seconds ago","Labels":{},"Ports":[]}
]`

// fakeEngine serves the Docker API and records stop requests.
type fakeEngine struct {
	containers string
	stopStatus int
	stopped    []string
}

func (f *fakeEngine) start(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(f.containers))
	})
	mux.HandleFunc("/containers/", func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/containers/"), "/stop")
		if !ok || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if f.stopStatus >= 400 {
			w.WriteHeader(f.stopStatus)
			json.NewEncoder(w).Encode(map[string]string{"message": "permission denied by policy"})
			return
		}
		f.stopped = append(f.stopped, id+"?"+r.URL.RawQuery)
		w.WriteHeader(f.stopStatus)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
}

func TestContainers(t *testing.T) {
	f := &fakeEngine{containers: containersJSON}
	f.start(t)
	c, err := Dial()
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Containers()
	if err != nil {
		t.Fatal(err)
	}
	want := []Container{
		{ID: "aaa111", Name: "shop-web-1", Image: "nginx:1.27", Project: "shop", Ports: []int{8080}, UpSeconds: 7200},
		{ID: "bbb222", Name: "pg", Image: "postgres:16", Ports: []int{5432}, UpSeconds: 3 * 86400}, // UDP ignored
		{ID: "ccc333", Name: "worker", Image: "busybox", UpSeconds: -1},                            // not "Up
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestParseUptime(t *testing.T) {
	tests := map[string]int64{
		"Up Less than a second":        0,
		"Up 1 second":                  1,
		"Up 45 seconds":                45,
		"Up About a minute":            60,
		"Up 5 minutes":                 300,
		"Up About an hour":             3600,
		"Up 2 hours":                   7200,
		"Up 2 hours (healthy)":         7200,
		"Up 3 days (Paused)":           3 * 86400,
		"Up 2 weeks":                   14 * 86400,
		"Up 4 months":                  120 * 86400,
		"Exited (0) 3 hours ago":       -1,
		"Restarting (1) 4 seconds ago": -1,
		"Created":                      -1,
		"Up a while":                   -1, // unrecognised wording: unknown, not a guess
		"":                             -1,
	}
	for status, want := range tests {
		if got := parseUptime(status); got != want {
			t.Errorf("parseUptime(%q) = %d, want %d", status, got, want)
		}
	}
}

func TestApply(t *testing.T) {
	containers := []Container{
		{ID: "aaa111", Name: "shop-web-1", Image: "nginx:1.27", Project: "shop", Ports: []int{8080}, UpSeconds: 160},
		{ID: "bbb222", Name: "pg", Image: "postgres:16", Ports: []int{5432}, UpSeconds: -1},
	}
	entries := []inspector.PortEntry{
		{Port: 3000, PID: 10, Process: "node", ProjectName: "my-app", AgeSeconds: 5, Command: "node server.js"},
		{Port: 5432, PID: 0, Process: "(unknown)", AgeSeconds: -1},                                  // Linux: root's docker-proxy, unseen
		{Port: 8080, PID: 700, Process: "com.docker.backend", AgeSeconds: 9000, Command: "backend"}, // Docker Desktop
		{Port: 8080, PID: 701, Process: "wslrelay", AgeSeconds: 9000},                               // ...and its second listener
		{Port: 9999, PID: 700, Process: "com.docker.backend", AgeSeconds: 9000},                     // Docker's own API port
	}

	got := apply(entries, containers)

	if len(got) != 4 {
		t.Fatalf("got %d entries, want 4 (the two :8080 rows merge): %+v", len(got), got)
	}
	if !reflect.DeepEqual(got[0], entries[0]) {
		t.Errorf("a non-Docker process must be untouched: %+v", got[0])
	}
	pg := got[1]
	if pg.ContainerID != "bbb222" || pg.Process != "pg" || pg.Image != "postgres:16" || pg.AgeSeconds != -1 {
		t.Errorf("postgres container (unknown owner on Linux) = %+v", pg)
	}
	web := got[2]
	if web.ContainerID != "aaa111" || web.Process != "shop-web-1" || web.ProjectName != "shop" ||
		web.AgeSeconds != 160 || web.PID != 700 || !strings.Contains(web.Command, "nginx:1.27") {
		t.Errorf("web container = %+v", web)
	}
	if got[3].IsContainer() || got[3].Process != "com.docker.backend" {
		t.Errorf("a Docker port no container publishes must stay as it is: %+v", got[3])
	}
}

func TestApplyNeverRelabelsOtherProcesses(t *testing.T) {
	// A normal process on a port that some container also claims (e.g. bound
	// to a different address) must not be presented as that container:
	// "killing" it would stop the wrong thing.
	containers := []Container{{ID: "x", Name: "web", Image: "nginx", Ports: []int{3000}}}
	entries := []inspector.PortEntry{{Port: 3000, PID: 10, Process: "node"}}
	if got := apply(entries, containers); got[0].IsContainer() {
		t.Fatalf("node was relabelled as a container: %+v", got[0])
	}
}

func TestAnnotateUsesTheEngine(t *testing.T) {
	f := &fakeEngine{containers: containersJSON}
	f.start(t)
	entries, warnings := Annotate([]inspector.PortEntry{{Port: 8080, PID: 700, Process: "com.docker.backend"}})
	if len(warnings) != 0 || entries[0].Process != "shop-web-1" {
		t.Fatalf("entries=%+v warnings=%v", entries, warnings)
	}
}

func TestAnnotateReportsABrokenEngine(t *testing.T) {
	f := &fakeEngine{containers: `{"message":"not json array`}
	f.start(t)
	in := []inspector.PortEntry{{Port: 8080, PID: 700, Process: "com.docker.backend"}}
	entries, warnings := Annotate(in)
	if len(warnings) != 1 || !reflect.DeepEqual(entries, in) {
		t.Fatalf("want the entries unchanged and one warning; got %+v, %v", entries, warnings)
	}
}

func TestStopPublished(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		port       int
		stopStatus int
		wantErr    string
		wantStop   bool
	}{
		{"stops a running container", "aaa111", 8080, http.StatusNoContent, "", true},
		{"already stopped is fine", "aaa111", 8080, http.StatusNotModified, "", true},
		{"container no longer publishes that port", "aaa111", 9090, http.StatusNoContent, "no longer publishes port 9090", false},
		{"container is gone", "zzz999", 8080, http.StatusNoContent, "no longer running", false},
		{"engine refuses", "aaa111", 8080, http.StatusForbidden, "permission denied by policy", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeEngine{containers: containersJSON, stopStatus: tt.stopStatus}
			f.start(t)
			err := StopPublished(tt.id, tt.port)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if tt.wantStop != (len(f.stopped) == 1) {
				t.Fatalf("stop requests = %v, want one: %v", f.stopped, tt.wantStop)
			}
			if tt.wantStop && f.stopped[0] != "aaa111?t=5" {
				t.Errorf("stop request = %q, want the 5s grace period", f.stopped[0])
			}
		})
	}
}

func TestBadDockerHostIsAnError(t *testing.T) {
	t.Setenv("DOCKER_HOST", "ssh://user@host")
	if _, err := Dial(); err == nil || !strings.Contains(err.Error(), "DOCKER_HOST") {
		t.Fatalf("err = %v, want it to name DOCKER_HOST", err)
	}
}
