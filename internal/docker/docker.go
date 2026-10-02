// Package docker talks to the local Docker engine so portfind can tell which
// container publishes a port, and stop that container instead of killing the
// Docker process that holds the port on its behalf.
//
// It speaks the engine's HTTP API directly over the local socket (a named
// pipe on Windows); nothing shells out to the docker CLI.
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrUnavailable means no Docker engine is reachable: Docker isn't installed
// or isn't running. Callers treat this as "no containers", not as a failure.
var ErrUnavailable = errors.New("no Docker engine is running")

const (
	listTimeout = 2 * time.Second  // scans run every 2s; never let Docker stall them
	stopTimeout = 30 * time.Second // the engine waits stopGraceSeconds before SIGKILL
	stopGrace   = 5                // seconds between SIGTERM and SIGKILL, as for processes
)

// Container is a running container and the host TCP ports it publishes.
type Container struct {
	ID      string
	Name    string // without the leading "/"
	Image   string
	Project string // Docker Compose project, if any
	Ports   []int  // published host TCP ports
	// UpSeconds is roughly how long the container has been running since it
	// was last started, or -1 if unknown. It comes from the engine's status
	// text ("Up 2 hours"), so it is only as precise as that.
	UpSeconds int64
}

// Client is a connection to one Docker engine endpoint.
type Client struct {
	endpoint string
	http     *http.Client
}

// Dial connects to the Docker engine named by $DOCKER_HOST, or else the
// first reachable default endpoint for this OS. It returns ErrUnavailable if
// there is none.
func Dial() (*Client, error) {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		c, err := newClient(host)
		if err != nil {
			return nil, fmt.Errorf("DOCKER_HOST=%s: %w", host, err)
		}
		return c, nil
	}
	for _, ep := range defaultEndpoints() {
		if !endpointExists(ep) {
			continue
		}
		if c, err := newClient(ep); err == nil {
			return c, nil
		}
	}
	return nil, ErrUnavailable
}

func newClient(endpoint string) (*Client, error) {
	network, addr, err := parseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		if network == "npipe" {
			return dialPipe(ctx, addr)
		}
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
	return &Client{
		endpoint: endpoint,
		http:     &http.Client{Transport: &http.Transport{DialContext: dial, DisableKeepAlives: true}},
	}, nil
}

// parseEndpoint splits a Docker endpoint into a network and address:
// unix:///var/run/docker.sock, npipe:////./pipe/docker_engine, tcp://host:2375.
func parseEndpoint(endpoint string) (network, addr string, err error) {
	scheme, rest, ok := strings.Cut(endpoint, "://")
	if !ok {
		return "", "", fmt.Errorf("unsupported Docker endpoint %q", endpoint)
	}
	switch scheme {
	case "unix":
		return "unix", rest, nil
	case "npipe":
		// npipe:////./pipe/name -> \\.\pipe\name
		return "npipe", strings.ReplaceAll(rest, "/", `\`), nil
	case "tcp", "http":
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" {
			return "", "", fmt.Errorf("unsupported Docker endpoint %q", endpoint)
		}
		return "tcp", u.Host, nil
	}
	return "", "", fmt.Errorf("unsupported Docker endpoint %q (portfind handles unix, npipe and tcp)", endpoint)
}

func (c *Client) do(ctx context.Context, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker %s %s: %w", method, path, err)
	}
	return resp, nil
}

// apiContainer is the part of the engine's /containers/json entries we use.
type apiContainer struct {
	ID     string            `json:"Id"`
	Status string            `json:"Status"` // e.g. "Up 2 hours (healthy)"
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	Labels map[string]string `json:"Labels"`
	Ports  []struct {
		PublicPort int    `json:"PublicPort"`
		Type       string `json:"Type"`
	} `json:"Ports"`
}

// Containers lists running containers.
func (c *Client) Containers() ([]Container, error) {
	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	defer cancel()
	resp, err := c.do(ctx, http.MethodGet, "/containers/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apiError("list containers", resp)
	}
	var raw []apiContainer
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("docker: decode container list: %w", err)
	}
	return toContainers(raw), nil
}

func toContainers(raw []apiContainer) []Container {
	out := make([]Container, 0, len(raw))
	for _, r := range raw {
		c := Container{ID: r.ID, Image: r.Image, Project: r.Labels["com.docker.compose.project"], UpSeconds: parseUptime(r.Status)}
		if len(r.Names) > 0 {
			c.Name = strings.TrimPrefix(r.Names[0], "/")
		}
		seen := map[int]bool{}
		for _, p := range r.Ports { // one entry per host IP: 0.0.0.0 and ::
			if p.Type == "tcp" && p.PublicPort > 0 && !seen[p.PublicPort] {
				seen[p.PublicPort] = true
				c.Ports = append(c.Ports, p.PublicPort)
			}
		}
		out = append(out, c)
	}
	return out
}

// parseUptime turns the engine's status text into seconds: "Up 2 hours",
// "Up About a minute", "Up 3 days (healthy)". The engine rounds to one unit,
// so the result is approximate. It returns -1 for anything it doesn't
// recognise, including containers that aren't up.
func parseUptime(status string) int64 {
	rest, ok := strings.CutPrefix(status, "Up ")
	if !ok {
		return -1
	}
	if i := strings.Index(rest, " ("); i >= 0 { // "(healthy)", "(Paused)"
		rest = rest[:i]
	}
	rest = strings.TrimSpace(rest)
	switch rest {
	case "Less than a second":
		return 0
	case "About a minute":
		return 60
	case "About an hour":
		return 3600
	}
	var n int64
	var unit string
	if _, err := fmt.Sscanf(rest, "%d %s", &n, &unit); err != nil || n < 0 {
		return -1
	}
	seconds := map[string]int64{
		"second": 1, "minute": 60, "hour": 3600, "day": 86400,
		"week": 7 * 86400, "month": 30 * 86400, "year": 365 * 86400,
	}[strings.TrimSuffix(unit, "s")]
	if seconds == 0 {
		return -1
	}
	return n * seconds
}

// Stop stops a container: SIGTERM, then SIGKILL after stopGrace seconds. It
// returns once the container has stopped.
func (c *Client) Stop(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	resp, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/containers/%s/stop?t=%d", url.PathEscape(id), stopGrace))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusNotModified: // stopped, or already stopped
		return nil
	}
	return apiError("stop container", resp)
}

// apiError turns a non-success response into an error carrying the engine's
// own message.
func apiError(action string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var msg struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &msg) == nil && msg.Message != "" {
		return fmt.Errorf("docker: %s: %s", action, msg.Message)
	}
	return fmt.Errorf("docker: %s: %s", action, resp.Status)
}
