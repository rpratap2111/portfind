package provenance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dirMarker as a file's content means "create a directory here".
const dirMarker = "<dir>"

// makeTree creates files (and directories) under root. Paths use '/'.
func makeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if content == dirMarker {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFinderFind(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		start      string // walk starts here (relative to the temp root)
		stop       string // extra stop dir, relative; the temp root is always one
		wantName   string // "" = no project
		wantMarker string
		wantRoot   string // relative
		wantErr    string
	}{
		{
			name:  "package.json name, found from a nested folder",
			files: map[string]string{"web/package.json": `{"name": "demo-web-app"}`, "web/src/lib": dirMarker},
			start: "web/src/lib", wantName: "demo-web-app", wantMarker: "package.json", wantRoot: "web",
		},
		{
			name:  "scoped package name kept as is",
			files: map[string]string{"ui/package.json": `{"name": "@acme/ui", "version": "1.0.0"}`},
			start: "ui", wantName: "@acme/ui", wantMarker: "package.json", wantRoot: "ui",
		},
		{
			name:  "package.json without a name falls back to the folder name",
			files: map[string]string{"my-site/package.json": `{"private": true}`},
			start: "my-site", wantName: "my-site", wantMarker: "package.json", wantRoot: "my-site",
		},
		{
			name:  "malformed package.json: folder name plus an error",
			files: map[string]string{"broken/package.json": `{"name": `},
			start: "broken", wantName: "broken", wantMarker: "package.json", wantRoot: "broken", wantErr: "parse",
		},
		{
			name:  "Cargo.toml [package] name, trailing comment ignored",
			files: map[string]string{"api/Cargo.toml": "[package]\nname = \"rusty-api\" # the server\nversion = \"0.1.0\"\n"},
			start: "api", wantName: "rusty-api", wantMarker: "Cargo.toml", wantRoot: "api",
		},
		{
			name:  "Cargo name outside [package] is ignored",
			files: map[string]string{"ws/Cargo.toml": "[workspace]\nmembers = [\"a\"]\n[dependencies]\nname = \"not-me\"\n"},
			start: "ws", wantName: "ws", wantMarker: "Cargo.toml", wantRoot: "ws",
		},
		{
			name:  "go.mod: last path element, major version suffix skipped",
			files: map[string]string{"svc/go.mod": "module github.com/acme/widget/v2\n\ngo 1.22\n", "svc/cmd/server": dirMarker},
			start: "svc/cmd/server", wantName: "widget", wantMarker: "go.mod", wantRoot: "svc",
		},
		{
			name:  "go.mod with comment and quotes",
			files: map[string]string{"tool/go.mod": "// tooling\nmodule \"example.com/tools/lint\" // pinned\n"},
			start: "tool", wantName: "lint", wantMarker: "go.mod", wantRoot: "tool",
		},
		{
			name:  "git repo without a manifest: folder name",
			files: map[string]string{"git-only-repo/.git": dirMarker, "git-only-repo/scripts": dirMarker},
			start: "git-only-repo/scripts", wantName: "git-only-repo", wantMarker: ".git", wantRoot: "git-only-repo",
		},
		{
			name:  ".git as a file (worktree or submodule) also counts",
			files: map[string]string{"wt/.git": "gitdir: ../main/.git/worktrees/wt\n"},
			start: "wt", wantName: "wt", wantMarker: ".git", wantRoot: "wt",
		},
		{
			name: "nearest marker wins: a package inside a monorepo",
			files: map[string]string{
				"mono/.git": dirMarker, "mono/package.json": `{"name": "monorepo"}`,
				"mono/packages/web/package.json": `{"name": "@mono/web"}`, "mono/packages/web/src": dirMarker,
			},
			start: "mono/packages/web/src", wantName: "@mono/web", wantMarker: "package.json", wantRoot: "mono/packages/web",
		},
		{
			name:  "same folder: package.json beats .git",
			files: map[string]string{"app/.git": dirMarker, "app/package.json": `{"name": "from-package-json"}`},
			start: "app", wantName: "from-package-json", wantMarker: "package.json", wantRoot: "app",
		},
		{
			name:  "same folder: Cargo.toml beats go.mod",
			files: map[string]string{"mixed/go.mod": "module example.com/gomod\n", "mixed/Cargo.toml": "[package]\nname = \"cargo-wins\"\n"},
			start: "mixed", wantName: "cargo-wins", wantMarker: "Cargo.toml", wantRoot: "mixed",
		},
		{
			name:  "a directory named like a manifest is skipped",
			files: map[string]string{"odd/package.json": dirMarker, "odd/.git": dirMarker},
			start: "odd", wantName: "odd", wantMarker: ".git", wantRoot: "odd",
		},
		{
			name:  "stops at a stop dir (e.g. home) without checking it",
			files: map[string]string{"home/package.json": `{"name": "stray-home-package"}`, "home/Downloads/tool": dirMarker},
			start: "home/Downloads/tool", stop: "home", wantName: "",
		},
		{
			name:  "stop dirs match case-insensitively",
			files: map[string]string{"Home/package.json": `{"name": "stray"}`, "Home/x": dirMarker},
			start: "Home/x", stop: "HOME", wantName: "",
		},
		{
			name:  "no marker anywhere",
			files: map[string]string{"plain/folder": dirMarker},
			start: "plain/folder", wantName: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			makeTree(t, root, tt.files)
			// The temp root is a stop dir so the walk never escapes into the
			// real filesystem above it.
			f := Finder{StopDirs: []string{root}}
			if tt.stop != "" {
				f.StopDirs = append(f.StopDirs, filepath.Join(root, tt.stop))
			}

			got, err := f.Find(filepath.Join(root, filepath.FromSlash(tt.start)))

			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.wantErr)
			}
			if got.Name != tt.wantName {
				t.Fatalf("Name = %q, want %q (project %+v)", got.Name, tt.wantName, got)
			}
			if tt.wantName == "" {
				return
			}
			if got.Marker != tt.wantMarker {
				t.Errorf("Marker = %q, want %q", got.Marker, tt.wantMarker)
			}
			if want := filepath.Join(root, filepath.FromSlash(tt.wantRoot)); got.Root != want {
				t.Errorf("Root = %q, want %q", got.Root, want)
			}
		})
	}
}

func TestGoModuleName(t *testing.T) {
	tests := map[string]string{
		"module example.com/app\n":           "app",
		"module github.com/a/b/v3\n":         "b",
		"module example.com/v2\n":            "example.com", // v2 is the whole second element
		"module single\n":                    "single",
		"module example.com/vfoo\n":          "vfoo", // not a major version suffix
		"modules are fun\nmodule real/one\n": "one",  // "modules" is not the directive
		"go 1.22\n":                          "",     // no module line
		"module\n":                           "",     // directive without a path
	}
	for in, want := range tests {
		got, err := goModuleName([]byte(in))
		if err != nil || got != want {
			t.Errorf("goModuleName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestTomlString(t *testing.T) {
	tests := map[string]string{
		`"name"`:               "name",
		` 'single' `:           "single",
		`"a" # comment`:        "a",
		`"unterminated`:        "",
		`bare`:                 "",
		`""`:                   "",
		`"has 'inner' ok"`:     "has 'inner' ok",
		`{ workspace = true }`: "",
	}
	for in, want := range tests {
		if got := tomlString(in); got != want {
			t.Errorf("tomlString(%q) = %q, want %q", in, got, want)
		}
	}
}
