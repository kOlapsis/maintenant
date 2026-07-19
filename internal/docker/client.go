package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	dtypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"github.com/kolapsis/maintenant/internal/retry"
)

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
)

// Client wraps the Docker SDK client with reconnection logic.
type Client struct {
	cli       client.APIClient
	mu        sync.RWMutex
	host      string
	logger    *slog.Logger
	connected bool
}

// NewClient creates a new Docker client wrapper.
// If host is empty, it uses the default Docker socket.
func NewClient(host string, logger *slog.Logger) (*Client, error) {
	opts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}
	if host != "" {
		opts = append(opts, client.WithHost(host))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	c := &Client{
		cli:    cli,
		host:   host,
		logger: logger,
	}

	return c, nil
}

// Connect pings the Docker daemon to verify connectivity.
func (c *Client) Connect(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	if err != nil {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return fmt.Errorf("docker ping failed: %w", err)
	}
	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()
	c.logger.Info("connected to Docker daemon")
	return nil
}

// ConnectWithRetry attempts to connect with exponential backoff.
func (c *Client) TryConnect(ctx context.Context) error {
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := c.cli.Ping(tctx)
	if err != nil {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return fmt.Errorf("docker ping failed: %w", err)
	}
	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()
	c.logger.Info("connected to Docker daemon")
	return nil
}

func (c *Client) ConnectWithRetry(ctx context.Context) error {
	b := retry.New(initialBackoff, maxBackoff, 0)
	for {
		err := c.Connect(ctx)
		if err == nil {
			return nil
		}
		delay := b.Next()
		c.logger.Warn("Docker connection failed, retrying",
			"error", err,
			"retry_in", delay,
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// IsConnected returns the current connection status.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// SetDisconnected marks the client as disconnected.
func (c *Client) SetDisconnected() {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
}

// API returns the underlying Docker API client for direct SDK calls.
func (c *Client) API() client.APIClient {
	return c.cli
}

// Close closes the Docker client connection.
func (c *Client) Close() error {
	return c.cli.Close()
}

// ContainerStats holds the decoded stats response from Docker.
type ContainerStats = dtypes.StatsResponse

// StatsOneShot retrieves a single stats snapshot for a container.
// It uses the one-shot API which returns immediately without a priming read.
func (c *Client) StatsOneShot(ctx context.Context, containerID string) (*ContainerStats, error) {
	resp, err := c.cli.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("stats one-shot %s: %w", containerID, err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	var stats ContainerStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decode stats %s: %w", containerID, err)
	}
	return &stats, nil
}

// SocketUnavailableError is returned when the Docker socket is not accessible.
type SocketUnavailableError struct {
	Err error
}

func (e *SocketUnavailableError) Error() string {
	return fmt.Sprintf("Docker API unavailable: %v. Mount the socket (-v /var/run/docker.sock:/var/run/docker.sock:ro) or set DOCKER_HOST to a reachable endpoint (e.g. a socket proxy: tcp://socketproxy:2375)", e.Err)
}

func (e *SocketUnavailableError) Unwrap() error {
	return e.Err
}
