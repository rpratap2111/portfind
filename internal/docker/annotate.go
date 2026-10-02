package docker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rpratap2111/portfind/internal/inspector"
)

// hostProcesses are the programs that hold a published port on a container's
// behalf: Docker Desktop's backend and WSL relay on Windows and macOS,
// docker-proxy on Linux. Matched as substrings of the lower-cased name.
var hostProcesses = []string{"com.docker", "docker-proxy", "dockerd", "wslrelay", "vpnkit", "rootlesskit", "docker desktop"}

// Annotate rewrites entries whose port is published by a running container:
// the entry takes the container's name, image and Compose project, so the
// user sees (and kills) the container rather than Docker's own process. Rows
// that turn out to be the same container port (Docker Desktop listens with
// two helper processes) are merged.
//
// It returns the entries and non-fatal warnings. No reachable Docker engine
// is not a warning: most machines simply aren't running Docker.
func Annotate(entries []inspector.PortEntry) ([]inspector.PortEntry, []error) {
	c, err := Dial()
	if errors.Is(err, ErrUnavailable) {
		return entries, nil
	}
	if err != nil {
		return entries, []error{err}
	}
	containers, err := c.Containers()
	if err != nil {
		return entries, []error{fmt.Errorf("%w (container names unavailable)", err)}
	}
	return apply(entries, containers), nil
}

// apply is Annotate's pure core.
func apply(entries []inspector.PortEntry, containers []Container) []inspector.PortEntry {
	byPort := map[int]Container{}
	for _, c := range containers {
		for _, p := range c.Ports {
			byPort[p] = c
		}
	}
	if len(byPort) == 0 {
		return entries
	}

	type key struct {
		port int
		id   string
	}
	seen := map[key]bool{}
	out := make([]inspector.PortEntry, 0, len(entries))
	for _, e := range entries {
		c, ok := byPort[e.Port]
		if !ok || !heldByDocker(e) {
			out = append(out, e)
			continue
		}
		k := key{e.Port, c.ID}
		if seen[k] {
			continue // second Docker helper process on the same container port
		}
		seen[k] = true

		e.ContainerID = c.ID
		e.Image = c.Image
		e.Process = c.Name
		e.Command = "docker container · image " + c.Image
		// The container's uptime, not the age of Docker's own process.
		e.AgeSeconds = c.UpSeconds
		if c.Project != "" {
			e.ProjectName = c.Project
		}
		out = append(out, e)
	}
	return out
}

// heldByDocker reports whether an entry's process could be Docker holding a
// published port. On Linux, docker-proxy runs as root, so an unprivileged
// scan sees the port with an unknown owner (PID 0); that counts too.
func heldByDocker(e inspector.PortEntry) bool {
	if e.PID == 0 {
		return true
	}
	name := strings.ToLower(e.Process)
	for _, h := range hostProcesses {
		if strings.Contains(name, h) {
			return true
		}
	}
	return false
}

// StopPublished stops container id after checking that it is still running
// and still publishes port, so a stale row can't stop a container that has
// since been replaced.
func StopPublished(id string, port int) error {
	c, err := Dial()
	if err != nil {
		return err
	}
	containers, err := c.Containers()
	if err != nil {
		return err
	}
	for _, ct := range containers {
		if ct.ID != id {
			continue
		}
		for _, p := range ct.Ports {
			if p == port {
				return c.Stop(id)
			}
		}
		return fmt.Errorf("container %s no longer publishes port %d; not stopped", ct.Name, port)
	}
	return errors.New("the container is no longer running")
}
