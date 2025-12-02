package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	defaultLogTail = "100"
	maxLogTail     = "500"
	snippetTail    = "50"
	maxSnippetLen  = 10240 // 10 KB max for stored snippets
)

// FetchLogs retrieves the last N lines of logs from a container.
func (c *Client) FetchLogs(ctx context.Context, containerID string, lines int, timestamps bool) ([]string, error) {
	tail := fmt.Sprintf("%d", lines)
	if lines <= 0 {
		tail = defaultLogTail
	}
	if lines > 500 {
		tail = maxLogTail
	}

	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Timestamps: timestamps,
	}

	reader, err := c.cli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return nil, fmt.Errorf("container logs: %w", err)
	}
	defer reader.Close()

	return demuxLogs(reader)
}

// FetchLogSnippet retrieves the last 50 lines of logs for storing as a snippet on exit events.
func (c *Client) FetchLogSnippet(ctx context.Context, containerID string) (string, error) {
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       snippetTail,
	}

	reader, err := c.cli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return "", fmt.Errorf("container logs snippet: %w", err)
	}
	defer reader.Close()

	lines, err := demuxLogs(reader)
	if err != nil {
		return "", err
	}

	snippet := strings.Join(lines, "\n")
	if len(snippet) > maxSnippetLen {
		snippet = snippet[len(snippet)-maxSnippetLen:]
	}

	return snippet, nil
}

// StreamLogs returns an io.ReadCloser that follows container logs in real-time.
// isTTY indicates whether the container uses a TTY (determines demux needs).
func (c *Client) StreamLogs(ctx context.Context, dockerID string, lines int, timestamps bool) (io.ReadCloser, bool, error) {
	tail := fmt.Sprintf("%d", lines)
	if lines <= 0 {
		tail = defaultLogTail
	}
	if lines > 500 {
		tail = maxLogTail
	}

	// Determine TTY status from container inspect.
	info, err := c.cli.ContainerInspect(ctx, dockerID)
	if err != nil {
		return nil, false, fmt.Errorf("container inspect for logs: %w", err)
	}
	isTTY := info.Config != nil && info.Config.Tty

	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Timestamps: timestamps,
		Follow:     true,
	}

	reader, err := c.cli.ContainerLogs(ctx, dockerID, opts)
	if err != nil {
		return nil, false, fmt.Errorf("container stream logs: %w", err)
	}

	return reader, isTTY, nil
}

func demuxLogs(reader interface{ Read([]byte) (int, error) }) ([]string, error) {
	var stdout, stderr bytes.Buffer
	_, err := stdcopy.StdCopy(&stdout, &stderr, reader)
	if err != nil {
		// Might be a TTY container — try raw read
		var raw bytes.Buffer
		raw.Write(stdout.Bytes())
		raw.Write(stderr.Bytes())
		// Read remaining
		buf := make([]byte, 32*1024)
		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				raw.Write(buf[:n])
			}
			if readErr != nil {
				break
			}
		}
		return splitLines(raw.String()), nil
	}

	combined := stdout.String() + stderr.String()
	return splitLines(combined), nil
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}
