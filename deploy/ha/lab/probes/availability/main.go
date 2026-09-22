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
	"sort"
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
	mode := flag.String("mode", "collect", "collect or summarise")
	url := flag.String("url", "", "URL to probe on the service address")
	interval := flag.Duration("interval", 100*time.Millisecond, "delay between two requests")
	timeout := flag.Duration("timeout", 2*time.Second, "per-request timeout")
	out := flag.String("out", "-", "ndjson output file, or - for stdout; also the journal read back to summarise")
	from := flag.String("from", "", "start of the window, RFC3339")
	to := flag.String("to", "", "end of the window, RFC3339")
	flag.Parse()

	var err error
	switch *mode {
	case "collect":
		err = runCollect(*url, *interval, *timeout, *out)
	case "summarise":
		if *from == "" || *to == "" {
			fmt.Fprintln(os.Stderr, "availability: -from and -to are required to summarise")
			os.Exit(2)
		}
		err = runSummarise(*out, *from, *to)
	default:
		fmt.Fprintln(os.Stderr, "availability: -mode must be collect or summarise")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "availability: %v\n", err)
		os.Exit(1)
	}
}

func runCollect(url string, interval, timeout time.Duration, out string) error {
	if url == "" {
		fmt.Fprintln(os.Stderr, "availability: -url is required")
		os.Exit(2)
	}

	sink, closeSink, err := openSink(out)
	if err != nil {
		return err
	}
	defer closeSink()

	resume, err := journal.LastSeq(out)
	if err != nil {
		return err
	}

	return run(url, interval, timeout, sink, resume)
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

type summary struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Samples     int    `json:"samples"`
	Failed      int    `json:"failed"`
	RTOMs       int64  `json:"rto_ms"`
	OutageStart string `json:"outage_start"`
	OutageEnd   string `json:"outage_end"`
	Recovered   bool   `json:"recovered"`
}

type outage struct {
	startWall, endWall   string
	startEpoch, endEpoch string
	startMono, endMono   int64
	startTime, endTime   time.Time
}

func (o outage) durationNS() int64 {
	if o.startEpoch == o.endEpoch {
		return o.endMono - o.startMono
	}
	return o.endTime.Sub(o.startTime).Nanoseconds()
}

func runSummarise(journalPath, fromRaw, toRaw string) error {
	from, err := parseWindowTime(fromRaw)
	if err != nil {
		return fmt.Errorf("-from: %w", err)
	}
	to, err := parseWindowTime(toRaw)
	if err != nil {
		return fmt.Errorf("-to: %w", err)
	}

	all, err := readSamples(journalPath)
	if err != nil {
		return err
	}

	var kept []sample
	for _, sm := range all {
		at, err := time.Parse(time.RFC3339Nano, sm.Wall)
		if err != nil || at.Before(from) || at.After(to) {
			continue
		}
		kept = append(kept, sm)
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Seq < kept[j].Seq })

	s := summarise(kept, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))

	enc, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(enc))
	return nil
}

func parseWindowTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func readSamples(path string) ([]sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []sample
	decoder := json.NewDecoder(f)
	for {
		var sm sample
		if err := decoder.Decode(&sm); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("journal %s: %w", path, err)
		}
		out = append(out, sm)
	}
	return out, nil
}

// summarise finds the longest run of consecutive non-up samples. A run still
// open at the last sample counts too, and marks the summary unrecovered.
func summarise(samples []sample, from, to string) summary {
	s := summary{From: from, To: to, Samples: len(samples), Recovered: true}

	var spans []outage
	var current *outage
	for _, sm := range samples {
		if sm.Outcome != outcomeUp {
			s.Failed++
			if current == nil {
				t, _ := time.Parse(time.RFC3339Nano, sm.Wall)
				current = &outage{startWall: sm.Wall, startEpoch: sm.Epoch, startMono: sm.MonoNS, startTime: t}
			}
			continue
		}
		if current != nil {
			t, _ := time.Parse(time.RFC3339Nano, sm.Wall)
			current.endWall, current.endEpoch, current.endMono, current.endTime = sm.Wall, sm.Epoch, sm.MonoNS, t
			spans = append(spans, *current)
			current = nil
		}
	}
	if current != nil {
		last := samples[len(samples)-1]
		t, _ := time.Parse(time.RFC3339Nano, last.Wall)
		current.endWall, current.endEpoch, current.endMono, current.endTime = last.Wall, last.Epoch, last.MonoNS, t
		spans = append(spans, *current)
		s.Recovered = false
	}

	longest := int64(-1)
	for _, o := range spans {
		if d := o.durationNS(); d > longest {
			longest = d
			s.OutageStart = o.startWall
			s.OutageEnd = o.endWall
		}
	}
	if longest >= 0 {
		s.RTOMs = longest / int64(time.Millisecond)
	}
	return s
}
