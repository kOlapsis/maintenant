// Command availability measures what a user sees: one request every 100 ms on
// the service address, one ndjson line per request.
//
// It runs on the measurement machine, outside both sites, so the recovery time
// it reports is observed rather than deduced from what the cluster believes
// (FR-014, FR-049). Every line carries both a wall clock stamp, to line up with
// the other logs of the run, and a monotonic offset, which is the one that can
// be trusted to measure a duration across a clock adjustment.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/kolapsis/maintenant/deploy/ha/lab/probes/journal"
)

type sample struct {
	Seq       uint64 `json:"seq"`
	Wall      string `json:"wall"`
	Epoch     string `json:"epoch"`
	MonoNS    int64  `json:"mono_ns"`
	LatencyNS int64  `json:"latency_ns"`
	Outcome   string `json:"outcome"`
	Status    int    `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
}

const (
	outcomeUp      = "up"
	outcomeStatus  = "status"
	outcomeTimeout = "timeout"
	outcomeRefused = "refused"
	outcomeError   = "error"
)

func main() {
	url := flag.String("url", "", "URL to probe on the service address")
	interval := flag.Duration("interval", 100*time.Millisecond, "delay between two requests")
	timeout := flag.Duration("timeout", 2*time.Second, "per-request timeout")
	out := flag.String("out", "-", "ndjson output file, or - for stdout")
	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "availability: -url is required")
		os.Exit(2)
	}

	sink, closeSink, err := openSink(*out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "availability: %v\n", err)
		os.Exit(1)
	}
	defer closeSink()

	resume, err := journal.LastSeq(*out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "availability: %v\n", err)
		os.Exit(1)
	}

	if err := run(*url, *interval, *timeout, sink, resume); err != nil {
		fmt.Fprintf(os.Stderr, "availability: %v\n", err)
		os.Exit(1)
	}
}

func openSink(path string) (io.Writer, func(), error) {
	if path == "-" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func run(url string, interval, timeout time.Duration, sink io.Writer, resume uint64) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := &http.Client{
		Transport: &http.Transport{
			// A fresh connection every time: a kept-alive socket would measure
			// the connection rather than the path a user takes to the service.
			DisableKeepAlives: true,
			DialContext:       (&net.Dialer{Timeout: timeout}).DialContext,
		},
	}

	lines := make(chan sample, 1024)
	var writing sync.WaitGroup
	writing.Add(1)
	go func() {
		defer writing.Done()
		encoder := json.NewEncoder(sink)
		for s := range lines {
			_ = encoder.Encode(s)
		}
	}()

	start := time.Now()
	// mono_ns counts from this process, so it only measures a duration within
	// one epoch; across a restart the wall clock is the only common ground.
	epoch := start.UTC().Format(time.RFC3339Nano)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var probing sync.WaitGroup
	seq := resume

	for {
		select {
		case <-ctx.Done():
			probing.Wait()
			close(lines)
			writing.Wait()
			return nil
		case <-ticker.C:
			seq++
			// The cadence is kept even while requests hang: an outage is
			// exactly when they do, and that is what the run needs measured.
			probing.Add(1)
			go func(seq uint64) {
				defer probing.Done()
				lines <- probe(client, url, timeout, seq, start, epoch)
			}(seq)
		}
	}
}

func probe(client *http.Client, url string, timeout time.Duration, seq uint64, start time.Time, epoch string) sample {
	at := time.Now()
	s := sample{
		Seq:    seq,
		Wall:   at.UTC().Format(time.RFC3339Nano),
		Epoch:  epoch,
		MonoNS: at.Sub(start).Nanoseconds(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		s.Outcome = outcomeError
		s.Error = err.Error()
		s.LatencyNS = time.Since(at).Nanoseconds()
		return s
	}

	resp, err := client.Do(req)
	s.LatencyNS = time.Since(at).Nanoseconds()
	if err != nil {
		s.Outcome = classify(err)
		s.Error = err.Error()
		return s
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	s.Status = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.Outcome = outcomeUp
	} else {
		s.Outcome = outcomeStatus
	}
	return s
}

func classify(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), os.IsTimeout(err):
		return outcomeTimeout
	case errors.Is(err, syscall.ECONNREFUSED):
		return outcomeRefused
	default:
		return outcomeError
	}
}
