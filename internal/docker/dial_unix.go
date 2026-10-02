//go:build !windows

package docker

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// defaultEndpoints are tried in order when $DOCKER_HOST is unset: the system
// engine, then the per-user sockets of Docker Desktop and rootless Docker.
func defaultEndpoints() []string {
	eps := []string{"unix:///var/run/docker.sock"}
	if home, err := os.UserHomeDir(); err == nil {
		eps = append(eps,
			"unix://"+filepath.Join(home, ".docker", "run", "docker.sock"),
			"unix://"+filepath.Join(home, ".docker", "desktop", "docker.sock"),
		)
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		eps = append(eps, "unix://"+filepath.Join(dir, "docker.sock"))
	}
	return eps
}

// endpointExists reports whether the socket file is there.
func endpointExists(endpoint string) bool {
	path, ok := strings.CutPrefix(endpoint, "unix://")
	if !ok {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func dialPipe(context.Context, string) (net.Conn, error) {
	return nil, errors.New("named pipes are a Windows feature")
}
