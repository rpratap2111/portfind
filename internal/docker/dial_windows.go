//go:build windows

package docker

import (
	"context"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// defaultEndpoints are tried in order when $DOCKER_HOST is unset. Docker
// Desktop serves its Linux engine on the second pipe and, for compatibility,
// usually on the first too.
func defaultEndpoints() []string {
	return []string{
		"npipe:////./pipe/docker_engine",
		"npipe:////./pipe/dockerDesktopLinuxEngine",
	}
}

// endpointExists reports whether the pipe is there, i.e. Docker is running.
func endpointExists(endpoint string) bool {
	_, addr, err := parseEndpoint(endpoint)
	if err != nil {
		return false
	}
	timeout := 500 * time.Millisecond
	conn, err := winio.DialPipe(addr, &timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func dialPipe(ctx context.Context, path string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, path)
}
