package provenance

import (
	"errors"
	"fmt"
	"path/filepath"

	"portfind/internal/inspector"
)

// Where the directory used for the project walk came from.
const (
	SourceCwd    = "cwd"     // the process's current working directory
	SourceExeDir = "exe-dir" // fallback: the directory of the executable
)

// Result is everything provenance learned about one process.
type Result struct {
	Project     Project // zero if no project marker was found
	Dir         string  // directory the walk started from
	DirSource   string  // SourceCwd or SourceExeDir
	CommandLine string  // empty if it could not be read
}

// processParams is what the OS-specific reader extracts from a live process.
type processParams struct {
	Cwd         string
	CommandLine string
}

// Resolve finds the project for a running process. exePath is used as a
// fallback starting point when the working directory cannot be read (see
// readProcessParams for when that happens). The returned error is non-fatal:
// Result holds whatever could be resolved.
func Resolve(pid int, exePath string) (Result, error) {
	var res Result
	var errs []error

	params, err := readProcessParams(pid)
	if err != nil {
		errs = append(errs, fmt.Errorf("read process parameters: %w", err))
	}
	res.CommandLine = params.CommandLine

	switch {
	case params.Cwd != "":
		res.Dir, res.DirSource = filepath.Clean(params.Cwd), SourceCwd
	case exePath != "":
		res.Dir, res.DirSource = filepath.Dir(exePath), SourceExeDir
		errs = append(errs, fmt.Errorf("working directory unknown, guessing project from executable directory %s", res.Dir))
	default:
		errs = append(errs, errors.New("neither working directory nor executable path is known"))
		return res, errors.Join(errs...)
	}

	res.Project, err = DefaultFinder().Find(res.Dir)
	if err != nil {
		errs = append(errs, fmt.Errorf("find project from %s: %w", res.Dir, err))
	}
	return res, errors.Join(errs...)
}

// Annotate fills ProjectName, and Command when the full command line can be
// read, on each entry in place. It returns non-fatal per-process warnings.
func Annotate(entries []inspector.PortEntry) []error {
	type cached struct {
		res Result
		err error
	}
	byPID := make(map[int]cached) // a process often listens on several ports

	var warnings []error
	for i := range entries {
		e := &entries[i]
		if e.PID == 0 || e.PID == 4 {
			continue // kernel pseudo-processes: no user-mode parameters exist
		}
		if e.Command == "" {
			// The inspector could not open this process at all and has
			// already reported it; reading its memory will fail the same way.
			continue
		}
		c, ok := byPID[e.PID]
		if !ok {
			c.res, c.err = Resolve(e.PID, e.Command)
			byPID[e.PID] = c
			if c.err != nil {
				warnings = append(warnings, fmt.Errorf("pid %d (%s): %w", e.PID, e.Process, c.err))
			}
		}
		e.ProjectName = c.res.Project.Name
		if c.res.CommandLine != "" {
			e.Command = c.res.CommandLine
		}
	}
	return warnings
}
