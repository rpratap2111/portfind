package risk

import (
	"testing"

	"github.com/rpratap2111/portfind/internal/inspector"
)

func TestClassifyContainer(t *testing.T) {
	tests := []struct {
		name, image, want string
	}{
		{"pg", "postgres:16", High},
		{"shop-db-1", "mysql:8", High},
		{"cache", "redis:7-alpine", High}, // image is "redis", not "redis-server"
		{"docs", "mongo:7", High},         // "mongo", not "mongod"
		{"db", "mcr.microsoft.com/mssql/server:2022-latest", High},
		{"my-postgres", "ghcr.io/acme/custom-db:1", High}, // the name counts too
		{"email_scheduler_es", "docker.elastic.co/elasticsearch/elasticsearch:8.12.0", High},
		{"shop-web-1", "nginx:1.27", Medium},
		{"api", "node:22", Medium}, // never LOW: stopping a container is not a quick y/N
		{"", "", Medium},
	}
	for _, tt := range tests {
		if got := ClassifyContainer(tt.name, tt.image); got != tt.want {
			t.Errorf("ClassifyContainer(%q, %q) = %s, want %s", tt.name, tt.image, got, tt.want)
		}
	}
}

func TestAnnotateTiersContainersByImage(t *testing.T) {
	entries := []inspector.PortEntry{
		{Port: 3000, Process: "node"},                                          // a real node process
		{Port: 8080, Process: "node", ContainerID: "abc", Image: "node:22"},    // a container that happens to be called node
		{Port: 5432, Process: "pg", ContainerID: "def", Image: "postgres:16"},  //
		{Port: 9000, Process: "com.docker.backend", ParentProcess: "services"}, // Docker itself
	}
	Annotate(entries)
	want := []string{Low, Medium, High, Medium}
	for i, e := range entries {
		if e.RiskTier != want[i] {
			t.Errorf("entry %d (%s): tier %s, want %s", i, e.Process, e.RiskTier, want[i])
		}
	}
}
