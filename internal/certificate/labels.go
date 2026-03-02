package certificate

import (
	"net"
	"strconv"
	"strings"
)

const tlsLabel = "maintenant.tls.certificates"

// ParsedCertLabel represents a single hostname:port parsed from the TLS certificates label.
type ParsedCertLabel struct {
	Hostname string
	Port     int
}

// ParseCertificateLabels extracts certificate monitoring targets from container labels.
// The label format is: maintenant.tls.certificates=host1,host2:8443,host3
// Hostnames without a port default to 443.
func ParseCertificateLabels(labels map[string]string) []ParsedCertLabel {
	raw, ok := labels[tlsLabel]
	if !ok {
		return nil
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var results []ParsedCertLabel
	seen := make(map[string]bool)

	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		hostname, port, err := parseCertEntry(entry)
		if err != nil {
			continue
		}

		key := net.JoinHostPort(hostname, strconv.Itoa(port))
		if seen[key] {
			continue
		}
		seen[key] = true

		results = append(results, ParsedCertLabel{
			Hostname: hostname,
			Port:     port,
		})
	}

	return results
}

func parseCertEntry(entry string) (string, int, error) {
	// Strip any scheme prefix (common mistake)
	for _, prefix := range []string{"https://", "http://"} {
		entry = strings.TrimPrefix(entry, prefix)
	}

	// Strip trailing path
	if idx := strings.Index(entry, "/"); idx != -1 {
		entry = entry[:idx]
	}

	host, portStr, err := net.SplitHostPort(entry)
	if err != nil {
		// No port specified — use default 443
		if validateHostname(entry) {
			return entry, 443, nil
		}
		return "", 0, err
	}

	if !validateHostname(host) {
		return "", 0, net.InvalidAddrError("invalid hostname")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, net.InvalidAddrError("invalid port")
	}

	return host, port, nil
}

func validateHostname(h string) bool {
	return h != "" && !strings.ContainsAny(h, " \t\n\r/")
}
