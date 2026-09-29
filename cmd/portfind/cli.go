package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// mode is what the command line asked portfind to do.
type mode int

const (
	modeTUI mode = iota
	modeJSON
	modeUpdate
	modeVersion
	modeHelp
)

// parseArgs turns the command-line arguments (without the program name) into
// a mode. Only one of --json, --update and --version may be given.
func parseArgs(args []string) (mode, error) {
	fs := flag.NewFlagSet("portfind", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // we print our own help and errors
	asJSON := fs.Bool("json", false, "")
	update := fs.Bool("update", false, "")
	version := fs.Bool("version", false, "")
	fs.BoolVar(version, "v", false, "")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return modeHelp, nil
		}
		return 0, err
	}
	if fs.NArg() > 0 {
		return 0, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	var chosen []mode
	if *asJSON {
		chosen = append(chosen, modeJSON)
	}
	if *update {
		chosen = append(chosen, modeUpdate)
	}
	if *version {
		chosen = append(chosen, modeVersion)
	}
	switch len(chosen) {
	case 0:
		return modeTUI, nil
	case 1:
		return chosen[0], nil
	default:
		return 0, errors.New("use only one of --json, --update and --version")
	}
}

// printHelp writes the --help text.
func printHelp(w io.Writer, version string) {
	fmt.Fprintf(w, `portfind %s
Find what's listening on your ports, see which project it belongs to, and kill it safely.

Usage:
  portfind             open the interactive terminal UI
  portfind --json      print listening ports as JSON and exit (for scripts)
  portfind --update    update portfind to the latest release
  portfind --version   print the version (also -v)
  portfind --help      show this help (also -h)

In the terminal UI:
  type        search by port, process or project
  ↑ / ↓       move the selection
  Enter       kill the selected process (asks first)
  Tab         history of processes that left their ports
  Ctrl+R      refresh now
  Ctrl+W      show warnings
  Esc         back out of a view; quit from the port list

Docs and issues: https://github.com/rpratap2111/portfind
`, version)
}
