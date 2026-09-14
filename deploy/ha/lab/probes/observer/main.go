// Command observer collects the two figures that are reported beside the write
// loss and never mixed into it: the telemetry hole a failover leaves, and the
// alerts the failover alone caused (FR-020).
//
// A telemetry hole is not data loss. Samples that were never collected because
// the collector was moving are missing from the history, and saying so is the
// point; counting them as lost writes would be a lie about the RPO.
//
// An alert only counts as induced when its subject was healthy just before the
// injection. A monitor that was already failing does not become the failover's
// fault.
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
	"syscall"
	"time"

	"github.com/kolapsis/maintenant/deploy/ha/lab/probes/journal"
)

type alertSeen struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	EntityID string `json:"entity_id"`
	Severity string `json:"severity"`
}

type hostSeen struct {
	AgentID   string `json:"agent_id"`
	Available bool   `json:"available"`
}

type observation struct {
	Seq       uint64      `json:"seq"`
	Wall      string      `json:"wall"`
	Epoch     string      `json:"epoch"`
	MonoNS    int64       `json:"mono_ns"`
	Reachable bool        `json:"reachable"`
	Hosts     []hostSeen  `json:"hosts"`
	Alerts    []alertSeen `json:"alerts"`
	Error     string      `json:"error,omitempty"`
}

func main() {
	mode := flag.String("mode", "collect", "collect or summarise")
	base := flag.String("base", "", "base URL of the service address")
	interval := flag.Duration("interval", time.Second, "delay between two observations")
	timeout := flag.Duration("timeout", 3*time.Second, "per-request timeout")
	journalPath := flag.String("journal", "observer.ndjson", "ndjson journal of observations")
	from := flag.String("from", "", "start of the window, RFC3339")
	to := flag.String("to", "", "end of the window, RFC3339")
	container := flag.String("container", "", "container whose history says how many samples were lost")
	flag.Parse()

	var err error
	switch *mode {
	case "collect":
		if *base == "" {
			fail("-base is required to collect")
		}
		err = collect(*base, *interval, *timeout, *journalPath)
	case "summarise":
		if *from == "" || *to == "" {
			fail("-from and -to are required to summarise")
		}
		if *container == "" || *base == "" {
			fail("-container and -base are required: the samples lost are counted in the stored history")
		}
		err = summarise(*journalPath, *base, *container, *from, *to, *timeout)
	default:
		fail("-mode must be collect or summarise")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "observer: %v\n", err)
		os.Exit(1)
	}
}

func fail(msg string) {
	fmt.Fprintf(os.Stderr, "observer: %s\n", msg)
	os.Exit(2)
}

func client(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext:       (&net.Dialer{Timeout: timeout}).DialContext,
		},
	}
}

func collect(base string, interval, timeout time.Duration, journalPath string) error {
	resume, err := journal.LastSeq(journalPath)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(journalPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	encoder := json.NewEncoder(f)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := client(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	start := time.Now()
	epoch := start.UTC().Format(time.RFC3339Nano)
	seq := resume

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			seq++
			o := observe(c, base, seq, start, epoch)
			if err := encoder.Encode(o); err != nil {
				return err
			}
		}
	}
}

func observe(c *http.Client, base string, seq uint64, start time.Time, epoch string) observation {
	at := time.Now()
	o := observation{
		Seq:    seq,
		Wall:   at.UTC().Format(time.RFC3339Nano),
		Epoch:  epoch,
		MonoNS: at.Sub(start).Nanoseconds(),
	}

	var hosts struct {
		Hosts []struct {
			AgentID   string `json:"agent_id"`
			Available bool   `json:"available"`
		} `json:"hosts"`
	}
	if err := getJSON(c, base+"/api/v1/resources/hosts", &hosts); err != nil {
		o.Error = err.Error()
		return o
	}

	var alerts struct {
		Alerts []alertSeen `json:"alerts"`
	}
	if err := getJSON(c, base+"/api/v1/alerts/active", &alerts); err != nil {
		o.Error = err.Error()
		return o
	}

	o.Reachable = true
	for _, h := range hosts.Hosts {
		o.Hosts = append(o.Hosts, hostSeen{AgentID: h.AgentID, Available: h.Available})
	}
	o.Alerts = alerts.Alerts
	return o
}

func getJSON(c *http.Client, url string, into interface{}) error {
	resp, err := c.Get(url) //nolint:noctx // the client carries the timeout
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

type induced struct {
	AgentOffline     int `json:"agent_offline"`
	HeartbeatOverdue int `json:"heartbeat_overdue"`
	EndpointDown     int `json:"endpoint_down"`
	Other            int `json:"other"`
}

type summary struct {
	From                 string   `json:"from"`
	To                   string   `json:"to"`
	TelemetryGapS        float64  `json:"telemetry_gap_s"`
	TelemetrySamplesLost int      `json:"telemetry_samples_lost"`
	AlertsInduced        induced  `json:"alerts_induced"`
	AlertsInducedIDs     []string `json:"alerts_induced_ids"`
	Observations         int      `json:"observations"`
}

func summarise(journalPath, base, container, fromRaw, toRaw string, timeout time.Duration) error {
	from, err := time.Parse(time.RFC3339, fromRaw)
	if err != nil {
		return fmt.Errorf("-from: %w", err)
	}
	to, err := time.Parse(time.RFC3339, toRaw)
	if err != nil {
		return fmt.Errorf("-to: %w", err)
	}

	observations, err := readObservations(journalPath)
	if err != nil {
		return err
	}

	s := summary{
		From:             from.UTC().Format(time.RFC3339Nano),
		To:               to.UTC().Format(time.RFC3339Nano),
		Observations:     len(observations),
		AlertsInducedIDs: []string{},
	}

	s.TelemetryGapS = telemetryGap(observations, from, to)

	s.AlertsInduced, s.AlertsInducedIDs, err = alertsInduced(observations, from, to)
	if err != nil {
		return err
	}

	lost, err := samplesLost(base, container, from, to, timeout)
	if err != nil {
		return err
	}
	s.TelemetrySamplesLost = lost

	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func readObservations(path string) ([]observation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []observation
	decoder := json.NewDecoder(f)
	for {
		var o observation
		if err := decoder.Decode(&o); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("journal %s: %w", path, err)
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Wall < out[j].Wall })
	return out, nil
}

// Telemetry flows when the service answers and at least one host is reporting
// fresh samples. The hole is the span between the last observation where it
// flowed and the first where it flowed again.
func flowing(o observation) bool {
	if !o.Reachable {
		return false
	}
	for _, h := range o.Hosts {
		if h.Available {
			return true
		}
	}
	return false
}

func telemetryGap(observations []observation, from, to time.Time) float64 {
	var lastFlowing time.Time
	var widest float64
	haveFlowing := false

	for _, o := range observations {
		at, err := time.Parse(time.RFC3339Nano, o.Wall)
		if err != nil || at.Before(from) || at.After(to) {
			continue
		}
		if flowing(o) {
			if haveFlowing {
				if gap := at.Sub(lastFlowing).Seconds(); gap > widest {
					widest = gap
				}
			}
			lastFlowing = at
			haveFlowing = true
		}
	}
	return widest
}

func alertsInduced(observations []observation, from, to time.Time) (induced, []string, error) {
	var counts induced
	ids := []string{}

	// Whatever was already firing when the injection happened is the baseline,
	// keyed by subject: a monitor already down does not become the failover's
	// fault when its alert is replaced by another one.
	baseline := map[string]bool{}
	haveBaseline := false
	for _, o := range observations {
		at, err := time.Parse(time.RFC3339Nano, o.Wall)
		if err != nil || at.After(from) {
			continue
		}
		if !o.Reachable {
			continue
		}
		baseline = map[string]bool{}
		for _, a := range o.Alerts {
			baseline[a.Source+"/"+a.EntityID] = true
		}
		haveBaseline = true
	}

	// Without a reading from before the injection, an alert that was already
	// firing cannot be told from one the failover caused, and every one of
	// them would be blamed on the failover.
	if !haveBaseline {
		return counts, ids, fmt.Errorf("no reachable observation at or before %s: nothing to compare the alerts against",
			from.UTC().Format(time.RFC3339Nano))
	}

	counted := map[string]bool{}
	for _, o := range observations {
		at, err := time.Parse(time.RFC3339Nano, o.Wall)
		if err != nil || !at.After(from) || at.After(to) {
			continue
		}
		for _, a := range o.Alerts {
			if counted[a.ID] || baseline[a.Source+"/"+a.EntityID] {
				continue
			}
			counted[a.ID] = true
			ids = append(ids, a.ID)
			switch a.Source {
			case "agent":
				counts.AgentOffline++
			case "heartbeat":
				counts.HeartbeatOverdue++
			case "endpoint":
				counts.EndpointDown++
			default:
				counts.Other++
			}
		}
	}
	sort.Strings(ids)
	return counts, ids, nil
}

// The cadence is read from the history itself rather than assumed, so the
// figure holds whatever the agent's sample period is.
func samplesLost(base, container string, from, to time.Time, timeout time.Duration) (int, error) {
	var body struct {
		Points []struct {
			Timestamp time.Time `json:"timestamp"`
		} `json:"points"`
	}
	url := fmt.Sprintf("%s/api/v1/containers/%s/resources/history?range=1h", base, container)
	if err := getJSON(client(timeout), url, &body); err != nil {
		return 0, err
	}

	var stamps []time.Time
	for _, p := range body.Points {
		if !p.Timestamp.Before(from) && !p.Timestamp.After(to) {
			stamps = append(stamps, p.Timestamp)
		}
	}
	if len(stamps) < 3 {
		return 0, fmt.Errorf("history holds %d points in the window, too few to read a cadence", len(stamps))
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i].Before(stamps[j]) })

	gaps := make([]time.Duration, 0, len(stamps)-1)
	for i := 1; i < len(stamps); i++ {
		gaps = append(gaps, stamps[i].Sub(stamps[i-1]))
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
	cadence := gaps[len(gaps)/2]
	if cadence <= 0 {
		return 0, errors.New("history cadence reads as zero")
	}

	lost := 0
	for _, g := range gaps {
		if missing := int(g/cadence) - 1; missing > 0 {
			lost += missing
		}
	}
	return lost, nil
}
