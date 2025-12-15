package endpoint

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// warnedLinkLocal tracks endpoints for which a link-local warning has been emitted.
var warnedLinkLocal sync.Map

// CheckHTTP performs an HTTP(S) health check against the given endpoint.
func CheckHTTP(ctx context.Context, ep *Endpoint, logger interface{ Warn(string, ...any) }) CheckResult {
	start := time.Now()
	result := CheckResult{
		EndpointID: ep.ID,
		Timestamp:  start,
	}

	cfg := ep.Config
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	// Check for link-local addresses (T009)
	warnLinkLocal(ep, logger)

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !cfg.TLSVerify,
		},
		DialContext: (&net.Dialer{
			Timeout: timeout,
		}).DialContext,
		ResponseHeaderTimeout: timeout,
	}

	maxRedirects := cfg.MaxRedirects
	if maxRedirects < 0 {
		maxRedirects = 0
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	defer client.CloseIdleConnections()

	method := cfg.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, ep.Target, nil)
	if err != nil {
		result.ResponseTimeMs = time.Since(start).Milliseconds()
		result.ErrorMessage = fmt.Sprintf("create request: %v", err)
		return result
	}

	req.Header.Set("User-Agent", "PulseBoard/1.0")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	result.ResponseTimeMs = time.Since(start).Milliseconds()

	if err != nil {
		result.ErrorMessage = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	// Extract TLS peer certificates for certificate auto-detection
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		result.TLSPeerCertificates = resp.TLS.PeerCertificates
	}

	statusCode := resp.StatusCode
	result.HTTPStatus = &statusCode

	matcher := NewStatusMatcher(cfg.ExpectedStatus)
	result.Success = matcher.Matches(statusCode)
	if !result.Success {
		result.ErrorMessage = fmt.Sprintf("unexpected status %d", statusCode)
	}

	return result
}

// warnLinkLocal logs a warning (once per endpoint) if the target resolves to a link-local or loopback address.
func warnLinkLocal(ep *Endpoint, logger interface{ Warn(string, ...any) }) {
	if logger == nil {
		return
	}

	key := ep.ID
	if _, loaded := warnedLinkLocal.LoadOrStore(key, true); loaded {
		return
	}

	u, err := url.Parse(ep.Target)
	if err != nil {
		return
	}

	host := u.Hostname()
	if host == "" {
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

// isMetadataAddress checks if an IP is a cloud metadata address (169.254.169.254).
func isMetadataAddress(ip net.IP) bool {
	return ip.Equal(net.ParseIP("169.254.169.254"))
}

// ClearLinkLocalWarning removes the link-local warning state for an endpoint (e.g., on reconfigure).
func ClearLinkLocalWarning(endpointID int64) {
	warnedLinkLocal.Delete(endpointID)
}
