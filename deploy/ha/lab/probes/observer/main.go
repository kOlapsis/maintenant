// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See deploy/ha/LICENSE.

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
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/kolapsis/maintenant/deploy/ha/lab/probes/journal"
)

func main() {
	mode := flag.String("mode", "collect", "collect or summarise")
	base := flag.String("base", "", "base URL of the service address")
	interval := flag.Duration("interval", time.Second, "delay between two observations")
	timeout := flag.Duration("timeout", 3*time.Second, "per-request timeout")
	journalPath := flag.String("journal", "observer.ndjson", "ndjson journal of observations")
	from := flag.String("from", "", "start of the window, RFC3339")
	to := flag.String("to", "", "end of the window, RFC3339")
	agentsTo := flag.String("agents-to", "", "end of the window the agents have to reconnect in, RFC3339 (default: -to)")
	containerName := flag.String("container-name", "", "name of the container, run by a remote agent, whose history says how many samples were lost")
	containerHost := flag.String("container-host", "", "label or hostname of the agent running that container, when the name alone is ambiguous")
	faultedHosts := flag.String("faulted-hosts", "", "comma-separated labels or hostnames of the nodes that rebooted or died during the window")
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
		if *containerName == "" || *base == "" {
			fail("-container-name and -base are required: the samples lost are counted in the stored history")
		}
		err = summarise(summariseArgs{
			journalPath:   *journalPath,
			base:          *base,
			containerName: *containerName,
			containerHost: *containerHost,
			faulted:       splitList(*faultedHosts),
			from:          *from,
			to:            *to,
			agentsTo:      *agentsTo,
			timeout:       *timeout,
		})
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

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
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
		Hosts []hostSeen `json:"hosts"`
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
	o.Hosts = hosts.Hosts
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

type summary struct {
	From                 string            `json:"from"`
	To                   string            `json:"to"`
	AgentsTo             string            `json:"agents_to"`
	FaultedHosts         []string          `json:"faulted_hosts"`
	FaultedAgentIDs      []string          `json:"faulted_agent_ids"`
	Container            string            `json:"container,omitempty"`
	TelemetryGapS        *float64          `json:"telemetry_gap_s"`
	TelemetrySamplesLost *int              `json:"telemetry_samples_lost"`
	AlertsInduced        *induced          `json:"alerts_induced"`
	AlertsInducedIDs     []string          `json:"alerts_induced_ids"`
	Agents               *agentsMeasure    `json:"agents"`
	Errors               map[string]string `json:"errors"`
	Observations         int               `json:"observations"`
}

type summariseArgs struct {
	journalPath   string
	base          string
	containerName string
	containerHost string
	faulted       []string
	from          string
	to            string
	agentsTo      string
	timeout       time.Duration
}

func summarise(a summariseArgs) error {
	from, err := time.Parse(time.RFC3339, a.from)
	if err != nil {
		return fmt.Errorf("-from: %w", err)
	}
	to, err := time.Parse(time.RFC3339, a.to)
	if err != nil {
		return fmt.Errorf("-to: %w", err)
	}
	agentsTo := to
	if a.agentsTo != "" {
		if agentsTo, err = time.Parse(time.RFC3339, a.agentsTo); err != nil {
			return fmt.Errorf("-agents-to: %w", err)
		}
	}

	observations, err := readObservations(a.journalPath)
	if err != nil {
		return err
	}

	faultedIDs := faultedAgents(observations, a.faulted)
	s := summary{
		From:             from.UTC().Format(time.RFC3339Nano),
		To:               to.UTC().Format(time.RFC3339Nano),
		AgentsTo:         agentsTo.UTC().Format(time.RFC3339Nano),
		FaultedHosts:     append([]string{}, a.faulted...),
		FaultedAgentIDs:  sortedKeys(faultedIDs),
		Observations:     len(observations),
		AlertsInducedIDs: []string{},
		Errors:           map[string]string{},
	}

	if gap, err := telemetryGap(observations, from, to, faultedIDs); err != nil {
		s.Errors["telemetry_gap_s"] = err.Error()
	} else {
		s.TelemetryGapS = &gap
	}

	if counts, ids, err := alertsInduced(observations, from, to, faultedIDs); err != nil {
		s.Errors["alerts_induced"] = err.Error()
	} else {
		s.AlertsInduced = &counts
		s.AlertsInducedIDs = ids
	}

	if m, err := agentsReconnect(observations, from, agentsTo, faultedIDs); err != nil {
		s.Errors["agents"] = err.Error()
	} else {
		s.Agents = &m
	}

	c := client(a.timeout)
	if id, err := findContainer(c, a.base, a.containerName, a.containerHost); err != nil {
		s.Errors["telemetry_samples_lost"] = err.Error()
	} else {
		s.Container = id
		if lost, err := fetchSamplesLost(c, a.base, id, from, to); err != nil {
			s.Errors["telemetry_samples_lost"] = err.Error()
		} else {
			s.TelemetrySamplesLost = &lost
		}
	}

	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))

	if len(s.Errors) > 0 {
		var parts []string
		for _, k := range sortedKeys(s.Errors) {
			parts = append(parts, k+": "+s.Errors[k])
		}
		return errors.New(strings.Join(parts, "; "))
	}
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
	sort.SliceStable(out, func(i, j int) bool {
		a, _ := wallOf(out[i])
		b, _ := wallOf(out[j])
		return a.Before(b)
	})
	return out, nil
}

func findContainer(c *http.Client, base, name, host string) (string, error) {
	var body struct {
		Groups []struct {
			Containers []containerSeen `json:"containers"`
		} `json:"groups"`
	}
	if err := getJSON(c, base+"/api/v1/containers", &body); err != nil {
		return "", err
	}
	var all []containerSeen
	for _, g := range body.Groups {
		all = append(all, g.Containers...)
	}
	return resolveContainer(all, name, host)
}

func fetchSamplesLost(c *http.Client, base, containerID string, from, to time.Time) (int, error) {
	var body struct {
		Points []struct {
			Timestamp time.Time `json:"timestamp"`
		} `json:"points"`
	}
	u := fmt.Sprintf("%s/api/v1/containers/%s/resources/history?range=1h", base, url.PathEscape(containerID))
	if err := getJSON(c, u, &body); err != nil {
		return 0, err
	}
	stamps := make([]time.Time, 0, len(body.Points))
	for _, p := range body.Points {
		stamps = append(stamps, p.Timestamp)
	}
	return samplesLost(stamps, from, to)
}
