package endpoint

import (
	"context"
	"fmt"
	"net"
	"time"
)

// CheckTCP performs a TCP connectivity probe against the given endpoint.
func CheckTCP(ctx context.Context, ep *Endpoint, logger interface{ Warn(string, ...any) }) CheckResult {
	start := time.Now()
	result := CheckResult{
		EndpointID: ep.ID,
		Timestamp:  start,
	}

	timeout := ep.Config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	// Link-local warning
	warnLinkLocalTCP(ep, logger)

	conn, err := net.DialTimeout("tcp", ep.Target, timeout)
	result.ResponseTimeMs = time.Since(start).Milliseconds()

	if err != nil {
		result.ErrorMessage = fmt.Sprintf("tcp dial failed: %v", err)
		return result
	}
	conn.Close()

	result.Success = true
	return result
}

// warnedLinkLocalTCP tracks TCP endpoints for which a link-local warning has been emitted.
// Reuses the same sync.Map from checker_http.go via a different key space (negative IDs won't collide
// since we use the same endpoint ID — the HTTP checker already stored it).
func warnLinkLocalTCP(ep *Endpoint, logger interface{ Warn(string, ...any) }) {
	if logger == nil {
		return
	}

	key := ep.ID
	if _, loaded := warnedLinkLocal.LoadOrStore(key, true); loaded {
		return
	}

	host, _, err := net.SplitHostPort(ep.Target)
	if err != nil {
		return
	}

	ips, err := net.LookupHost(host)
	if err != nil {
		return
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			isMetadataAddress(ip) {
			logger.Warn("endpoint target resolves to link-local/loopback address",
				"endpoint_id", ep.ID,
				"target", ep.Target,
				"resolved_ip", ipStr,
			)
			return
		}
	}
}
