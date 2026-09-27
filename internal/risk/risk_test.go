package risk

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		process, parent string
		want            string
	}{
		// Databases are HIGH by substring, whatever their exact binary name.
		{"postgres", "", High},
		{"mysqld", "services", High},
		{"mongod", "", High},
		{"redis-server", "", High},
		{"sqlservr", "", High},
		{"PostgreSQL", "", High}, // case-insensitive
		// Anything started over SSH is HIGH, even a dev server.
		{"node", "sshd", High},
		{"python", "ssh", High},
		// Dev runtimes are LOW.
		{"node", "cmd", Low},
		{"python", "", Low},
		{"ruby", "", Low},
		{"javaw", "", Low},
		{"dlv", "", Low},
		// `go run` builds a temp binary with an arbitrary name; its parent is go.
		{"listener", "go", Low},
		// Everything else, including services, is MEDIUM.
		{"svchost", "services", Medium},
		{"caddy", "", Medium},
		{"", "", Medium},
		// Traps: substrings that must not match.
		{"mongo-express", "", Medium}, // "mongo", not "mongod"
		{"listener", "gopls", Medium}, // parent must be exactly "go"
		{"worker", "ssh-agent", Medium},
	}
	for _, tt := range tests {
		if got := Classify(tt.process, tt.parent); got != tt.want {
			t.Errorf("Classify(%q, parent %q) = %s, want %s", tt.process, tt.parent, got, tt.want)
		}
	}
}
