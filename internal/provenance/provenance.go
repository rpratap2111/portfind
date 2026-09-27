// Package provenance works out which project a process belongs to: it finds
// the directory the process runs from (see Resolve) and walks up from there
// looking for a project marker such as package.json or .git (see Finder).
package provenance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Project identifies the project a directory belongs to.
type Project struct {
	Name   string // display name, e.g. "my-app"
	Root   string // directory containing the marker
	Marker string // marker that matched, e.g. "package.json"
}

// marker is a file or directory whose presence marks a project root. parse
// extracts a project name from the file's contents; a nil parse, or an empty
// name, means the root directory's name is used instead.
type marker struct {
	file  string
	parse func(data []byte) (string, error)
}

// markers are checked in this order within each directory. Manifests come
// before .git because they carry an explicit name, and the nearest directory
// always wins, so a package inside a monorepo resolves to the package.
var markers = []marker{
	{"package.json", packageJSONName},
	{"Cargo.toml", cargoPackageName},
	{"go.mod", goModuleName},
	{".git", nil},
}

// Finder walks up a directory tree looking for project markers.
type Finder struct {
	// StopDirs are never treated as project roots, and the walk ends when it
	// reaches one. This keeps a stray package.json in the user's home
	// directory from claiming every app that runs from somewhere under it.
	StopDirs []string
}

// DefaultFinder stops at the user's home directory when it can be determined.
func DefaultFinder() Finder {
	var f Finder
	if home, err := os.UserHomeDir(); err == nil {
		f.StopDirs = append(f.StopDirs, home)
	}
	return f
}

// Find walks up from dir and returns the first project it finds. A zero
// Project with a nil error means no marker was found. When a marker is found
// but its manifest cannot be read or parsed, Find returns the project (named
// after its directory) together with an error describing the problem.
func (f Finder) Find(dir string) (Project, error) {
	dir = filepath.Clean(dir)
	for {
		if f.isStop(dir) {
			return Project{}, nil
		}
		p, found, err := projectAt(dir)
		if found || err != nil {
			return p, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Project{}, nil
		}
		dir = parent
	}
}

func (f Finder) isStop(dir string) bool {
	for _, s := range f.StopDirs {
		// Case-insensitive because Windows paths are; a false match on a
		// case-sensitive filesystem only shortens the walk.
		if strings.EqualFold(filepath.Clean(s), dir) {
			return true
		}
	}
	return false
}

// projectAt checks a single directory for markers.
func projectAt(dir string) (Project, bool, error) {
	for _, m := range markers {
		path := filepath.Join(dir, m.file)
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Project{}, false, fmt.Errorf("stat %s: %w", path, err)
		}
		p := Project{Name: dirName(dir), Root: dir, Marker: m.file}
		if m.parse == nil {
			return p, true, nil
		}
		if info.IsDir() {
			continue // e.g. a directory that happens to be named go.mod
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return p, true, fmt.Errorf("read %s: %w", path, err)
		}
		name, err := m.parse(data)
		if err != nil {
			return p, true, fmt.Errorf("parse %s: %w", path, err)
		}
		if name != "" {
			p.Name = name
		}
		return p, true, nil
	}
	return Project{}, false, nil
}

// dirName is filepath.Base, except a volume root like `C:\` keeps its full form.
func dirName(dir string) string {
	base := filepath.Base(dir)
	if base == string(filepath.Separator) || base == "." {
		return dir
	}
	return base
}

func packageJSONName(data []byte) (string, error) {
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", err
	}
	return strings.TrimSpace(pkg.Name), nil
}

// cargoPackageName reads `name` from the [package] table. It is a line scanner,
// not a TOML parser: it handles the overwhelmingly common single-line form and
// returns "" (falling back to the directory name) for anything else, such as a
// workspace-only Cargo.toml.
func cargoPackageName(data []byte) (string, error) {
	inPackage := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inPackage = line == "[package]"
			continue
		}
		if !inPackage {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "name" {
			return tomlString(val), nil
		}
	}
	return "", nil
}

// tomlString returns the contents of a quoted TOML string value, ignoring any
// trailing comment, or "" if val does not start with a quote.
func tomlString(val string) string {
	val = strings.TrimSpace(val)
	if len(val) < 2 || (val[0] != '"' && val[0] != '\'') {
		return ""
	}
	if end := strings.IndexByte(val[1:], val[0]); end >= 0 {
		return val[1 : 1+end]
	}
	return ""
}

// goModuleName returns the last element of the module path, skipping a major
// version suffix: "github.com/acme/widget/v2" -> "widget".
func goModuleName(data []byte) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module")
		if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		if i := strings.Index(rest, "//"); i >= 0 {
			rest = rest[:i]
		}
		path := strings.Trim(strings.TrimSpace(rest), "\"`")
		if path == "" {
			return "", nil
		}
		parts := strings.Split(path, "/")
		last := parts[len(parts)-1]
		if len(parts) > 1 && isMajorVersion(last) {
			last = parts[len(parts)-2]
		}
		return last, nil
	}
	return "", nil
}

func isMajorVersion(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for _, r := range s[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
