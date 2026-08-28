package endpoint

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/trust"
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
	timeout := time.Duration(cfg.Timeout)
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	// Check for link-local addresses (T009)
	warnLinkLocal(ep, logger)

	method := cfg.Method
	if method == "" {
		method = "GET"
	}

	newRequest := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, method, ep.Target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "maintenant/1.0")
		for k, v := range cfg.Headers {
			req.Header.Set(k, v)
		}
		return req, nil
	}

	client := newHTTPClient(cfg, timeout, !cfg.TLSVerify)
	defer client.CloseIdleConnections()

	req, err := newRequest()
	if err != nil {
		result.ResponseTimeMs = time.Since(start).Milliseconds()
		result.ErrorMessage = fmt.Sprintf("create request: %v", err)
		return result
	}

	resp, err := client.Do(req)

	if err != nil {
		reason, isTrustFailure := trustFailureReason(err)
		if !isTrustFailure {
			result.ResponseTimeMs = time.Since(start).Milliseconds()
			result.ErrorMessage = fmt.Sprintf("request failed: %v", err)
			return result
		}

		// The certificate was rejected, but that says nothing about whether the
		// host is serving. Retry once without verification purely to find out:
		// a host behind an internal PKI that answers is degraded, not down. The
		// retry never yields "up" — it only tells the two apart, and it hands
		// back the chain so expiry monitoring keeps working on these hosts.
		insecure := newHTTPClient(cfg, timeout, true)
		defer insecure.CloseIdleConnections()

		retryReq, reqErr := newRequest()
		if reqErr != nil {
			result.ResponseTimeMs = time.Since(start).Milliseconds()
			result.ErrorMessage = fmt.Sprintf("create request: %v", reqErr)
			return result
		}

		resp, err = insecure.Do(retryReq)
		if err != nil {
			result.ResponseTimeMs = time.Since(start).Milliseconds()
			result.ErrorMessage = fmt.Sprintf("request failed: %v", err)
			return result
		}

		result.Degraded = true
		result.DegradedReason = reason
	}

	result.ResponseTimeMs = time.Since(start).Milliseconds()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// Extract TLS peer certificates for certificate auto-detection
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		result.TLSPeerCertificates = resp.TLS.PeerCertificates
		result.TLSOCSPResponse = resp.TLS.OCSPResponse
	}

	statusCode := resp.StatusCode
	result.HTTPStatus = &statusCode

	matcher := NewStatusMatcher(cfg.ExpectedStatus)
	result.Success = matcher.Matches(statusCode)
	if !result.Success {
		result.ErrorMessage = fmt.Sprintf("unexpected status %d", statusCode)
	}

	// Surface the trust problem either way: on its own when the host is
	// otherwise healthy, appended when it is also returning a bad status.
	if result.Degraded {
		if result.ErrorMessage == "" {
			result.ErrorMessage = result.DegradedReason
		} else {
			result.ErrorMessage += "; " + result.DegradedReason
		}
	}

	return result
}

// newHTTPClient builds the probe client. skipVerify is used both for the
// per-endpoint opt-out and for the diagnostic retry after a rejected certificate.
func newHTTPClient(cfg EndpointConfig, timeout time.Duration, skipVerify bool) *http.Client {
	maxRedirects := cfg.MaxRedirects
	if maxRedirects < 0 {
		maxRedirects = 0
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: skipVerify, // #nosec G402 -- per-endpoint opt-out, or the diagnostic retry that classifies a rejected certificate.
				MinVersion:         tls.VersionTLS12,
				RootCAs:            trust.Pool(), // nil unless an extra CA was configured: system store
			},
			DialContext:           (&net.Dialer{Timeout: timeout}).DialContext,
			ResponseHeaderTimeout: timeout,
		},
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// trustFailureReason reports whether err is the peer's certificate being
// rejected — as opposed to the host being unreachable — and describes it in
// terms an operator can act on.
func trustFailureReason(err error) (string, bool) {
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return "certificate signed by an unknown authority", true
	}

	var hostname x509.HostnameError
	if errors.As(err, &hostname) {
		return "certificate is not valid for this hostname", true
	}

	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) {
		if invalid.Reason == x509.Expired {
			return "certificate has expired", true
		}
		return fmt.Sprintf("certificate is not valid: %v", invalid), true
	}

	// Go wraps verification failures in this type; the cases above unwrap
	// through it, so reaching here means a kind we have not named yet.
	var verification *tls.CertificateVerificationError
	if errors.As(err, &verification) {
		return "certificate verification failed", true
	}

	return "", false
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
func ClearLinkLocalWarning(endpointID string) {
	warnedLinkLocal.Delete(endpointID)
}
