package docker

import (
	"context"
	"io"
	"log/slog"
	"os"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	cmodel "github.com/kolapsis/pulseboard/internal/container"
	pbruntime "github.com/kolapsis/pulseboard/internal/runtime"
)

func init() {
	pbruntime.Register("docker", func(ctx context.Context, logger *slog.Logger) (pbruntime.Runtime, error) {
		return NewRuntime(os.Getenv("DOCKER_HOST"), logger)
	})
}

// Runtime implements runtime.Runtime for Docker.
type Runtime struct {
	client *Client
	logger *slog.Logger

	mu   sync.Mutex
	prev map[string]*cpuPrev // CPU delta state keyed by externalID
}

type cpuPrev struct {
	cpuTotal    uint64
	systemUsage uint64
}

// NewRuntime creates a Docker runtime wrapping the Docker SDK client.
func NewRuntime(host string, logger *slog.Logger) (*Runtime, error) {
	client, err := NewClient(host, logger)
	if err != nil {
		return nil, err
	}
	return &Runtime{
		client: client,
		logger: logger,
		prev:   make(map[string]*cpuPrev),
	}, nil
}

func (r *Runtime) Connect(ctx context.Context) error {
	return r.client.ConnectWithRetry(ctx)
}

func (r *Runtime) IsConnected() bool {
	return r.client.IsConnected()
}

func (r *Runtime) SetDisconnected() {
	r.client.SetDisconnected()
}

func (r *Runtime) Close() error {
	return r.client.Close()
}

func (r *Runtime) Name() string {
	return "docker"
}

func (r *Runtime) DiscoverAll(ctx context.Context) ([]*cmodel.Container, error) {
	return r.client.DiscoverAll(ctx)
}

// DiscoverAllWithLabels delegates to the underlying Docker client for endpoint label discovery.
func (r *Runtime) DiscoverAllWithLabels(ctx context.Context) ([]*DiscoveryResult, error) {
	return r.client.DiscoverAllWithLabels(ctx)
}

func (r *Runtime) StreamEvents(ctx context.Context) <-chan pbruntime.RuntimeEvent {
	dockerCh := r.client.StreamEvents(ctx)
	out := make(chan pbruntime.RuntimeEvent, 64)
	go func() {
		defer close(out)
		for evt := range dockerCh {
			out <- pbruntime.RuntimeEvent{
				Action:       evt.Action,
				ExternalID:   evt.ExternalID,
				Name:         evt.Name,
				ExitCode:     evt.ExitCode,
				HealthStatus: evt.HealthStatus,
				Timestamp:    evt.Timestamp,
				Labels:       evt.Labels,
			}
		}
	}()
	return out
}

func (r *Runtime) StatsSnapshot(ctx context.Context, externalID string) (*pbruntime.RawStats, error) {
	stats, err := r.client.StatsOneShot(ctx, externalID)
	if err != nil {
		return nil, err
	}

	// Sum network bytes across all interfaces.
	var netRx, netTx int64
	for _, ns := range stats.Networks {
		netRx += int64(ns.RxBytes)
		netTx += int64(ns.TxBytes)
	}

	// Sum block I/O.
	var blockRead, blockWrite int64
	for _, entry := range stats.BlkioStats.IoServiceBytesRecursive {
		switch entry.Op {
		case "read", "Read":
			blockRead += int64(entry.Value)
		case "write", "Write":
			blockWrite += int64(entry.Value)
		}
	}

	// Memory: working-set = Usage - inactive_file.
	memUsed := int64(stats.MemoryStats.Usage)
	if inactive, ok := stats.MemoryStats.Stats["inactive_file"]; ok {
		memUsed -= int64(inactive)
		if memUsed < 0 {
			memUsed = 0
		}
	}

	// CPU delta computation.
	r.mu.Lock()
	prev, hasPrev := r.prev[externalID]
	r.prev[externalID] = &cpuPrev{
		cpuTotal:    stats.CPUStats.CPUUsage.TotalUsage,
		systemUsage: stats.CPUStats.SystemUsage,
	}
	r.mu.Unlock()

	if !hasPrev {
		return nil, nil // first sample, no delta yet
	}

	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - prev.cpuTotal)
	systemDelta := float64(stats.CPUStats.SystemUsage - prev.systemUsage)

	var cpuPercent float64
	if systemDelta > 0 && cpuDelta >= 0 {
		numCPUs := float64(stats.CPUStats.OnlineCPUs)
		if numCPUs == 0 {
			numCPUs = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
		}
		if numCPUs == 0 {
			numCPUs = float64(goruntime.NumCPU())
		}
		cpuPercent = (cpuDelta / systemDelta) * numCPUs * 100.0
	}

	return &pbruntime.RawStats{
		CPUPercent:      cpuPercent,
		MemUsed:         memUsed,
		MemLimit:        int64(stats.MemoryStats.Limit),
		NetRxBytes:      netRx,
		NetTxBytes:      netTx,
		BlockReadBytes:  blockRead,
		BlockWriteBytes: blockWrite,
		Timestamp:       time.Now(),
	}, nil
}

func (r *Runtime) FetchLogs(ctx context.Context, externalID string, lines int, timestamps bool) ([]string, error) {
	return r.client.FetchLogs(ctx, externalID, lines, timestamps)
}

// FetchLogSnippet retrieves the last 50 lines of logs for die event snippets.
// Satisfies container.LogFetcher interface.
func (r *Runtime) FetchLogSnippet(ctx context.Context, externalID string) (string, error) {
	return r.client.FetchLogSnippet(ctx, externalID)
}

func (r *Runtime) StreamLogs(ctx context.Context, externalID string, lines int, timestamps bool) (io.ReadCloser, error) {
	reader, isTTY, err := r.client.StreamLogs(ctx, externalID, lines, timestamps)
	if err != nil {
		return nil, err
	}
	if isTTY {
		return reader, nil
	}
	return newDemuxReader(reader), nil
}

func (r *Runtime) GetHealthInfo(ctx context.Context, externalID string) (*pbruntime.HealthInfo, error) {
	hi, err := r.client.GetHealthInfo(ctx, externalID)
	if err != nil {
		return nil, err
	}
	result := &pbruntime.HealthInfo{
		HasHealthCheck: hi.HasHealthCheck,
		Status:         "none",
	}
	if hi.Status != nil {
		result.Status = string(*hi.Status)
	}
	return result, nil
}

// Client returns the underlying Docker client for Docker-specific operations.
func (r *Runtime) Client() *Client {
	return r.client
}

// Logger returns the runtime logger.
func (r *Runtime) Logger() *slog.Logger {
	return r.logger
}

// demuxReader wraps a Docker multiplexed log stream with stdcopy demuxing.
type demuxReader struct {
	pr       *io.PipeReader
	original io.ReadCloser
}

func newDemuxReader(r io.ReadCloser) *demuxReader {
	pr, pw := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, r)
		if err != nil {
			pw.CloseWithError(err)
		} else {
			pw.Close()
		}
		r.Close()
	}()
	return &demuxReader{pr: pr, original: r}
}

func (d *demuxReader) Read(p []byte) (int, error) {
	return d.pr.Read(p)
}

func (d *demuxReader) Close() error {
	err := d.pr.Close()
	d.original.Close()
	return err
}
