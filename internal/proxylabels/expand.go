// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

// Package proxylabels derives maintenant endpoint labels from reverse proxy labels.
package proxylabels

import (
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	labelIgnore         = "maintenant.ignore"
	labelOptOut         = "maintenant.proxy-labels"
	labelEndpointHTTP   = "maintenant.endpoint.http"
	labelEndpointTCP    = "maintenant.endpoint.tcp"
	labelExpectedStatus = "maintenant.endpoint.http.expected-status"
	labelCertificates   = "maintenant.tls.certificates"
	labelTraefikEnable  = "traefik.enable"

	defaultExpectedStatus = "2xx,3xx"
)

var (
	indexedTargetRe = regexp.MustCompile(`^maintenant\.endpoint\.\d+\.(http|tcp)$`)
	traefikRuleRe   = regexp.MustCompile(`^traefik\.http\.routers\.([^.]+)\.rule$`)
	hostMatcherRe   = regexp.MustCompile(`(?:^|[^A-Za-z])Host\(([^)]*)\)`)
	pathMatcherRe   = regexp.MustCompile(`(?:^|[^A-Za-z])(?:PathPrefix|Path)\(([^)]*)\)`)
	caddySiteKeyRe  = regexp.MustCompile(`^caddy(_\d+)?$`)
)

type target struct {
	url         string
	host        string
	https       bool
	internalTLS bool
}

// Expand returns a copy of labels with maintenant endpoint labels synthesized from Traefik and Caddy labels.
func Expand(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		out[k] = v
	}
	if !eligible(labels) {
		return out
	}

	byURL := make(map[string]*target)
	for _, t := range append(traefikTargets(labels), caddyTargets(labels)...) {
		if existing, ok := byURL[t.url]; ok {
			existing.internalTLS = existing.internalTLS || t.internalTLS
			continue
		}
		byURL[t.url] = &t
	}
	if len(byURL) == 0 {
		return out
	}

	urls := make([]string, 0, len(byURL))
	for u := range byURL {
		urls = append(urls, u)
	}
	sort.Strings(urls)

	_, globalStatus := labels[labelExpectedStatus]
	var certHosts []string
	for i, u := range urls {
		t := byURL[u]
		prefix := "maintenant.endpoint." + strconv.Itoa(i) + ".http"
		out[prefix] = u
		if !globalStatus {
			out[prefix+".expected-status"] = defaultExpectedStatus
		}
		if t.internalTLS {
			out[prefix+".tls-verify"] = "false"
		}
		if t.https && !t.internalTLS {
			certHosts = append(certHosts, t.host)
		}
	}
	if merged := mergeCertificates(labels[labelCertificates], certHosts); merged != "" {
		out[labelCertificates] = merged
	}
	return out
}

func eligible(labels map[string]string) bool {
	if v := strings.ToLower(strings.TrimSpace(labels[labelIgnore])); v == "true" || v == "1" {
		return false
	}
	if isFalse(labels[labelOptOut]) {
		return false
	}
	for k := range labels {
		if k == labelEndpointHTTP || k == labelEndpointTCP || indexedTargetRe.MatchString(k) {
			return false
		}
	}
	return true
}

func isFalse(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no":
		return true
	}
	return false
}

func traefikTargets(labels map[string]string) []target {
	if isFalse(labels[labelTraefikEnable]) {
		return nil
	}
	var targets []target
	for k, rule := range labels {
		m := traefikRuleRe.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		router := "traefik.http.routers." + m[1]
		scheme := "http"
		if traefikRouterTLS(labels, router) {
			scheme = "https"
		}
		path := traefikPath(rule)
		for _, hm := range hostMatcherRe.FindAllStringSubmatch(rule, -1) {
			for _, host := range matcherArgs(hm[1]) {
				if !validHost(host) {
					continue
				}
				targets = append(targets, target{
					url:   scheme + "://" + host + path,
					host:  host,
					https: scheme == "https",
				})
			}
		}
	}
	return targets
}

func traefikRouterTLS(labels map[string]string, router string) bool {
	tlsKey := router + ".tls"
	for k, v := range labels {
		if (k == tlsKey || strings.HasPrefix(k, tlsKey+".")) && !isFalse(v) {
			return true
		}
	}
	for _, ep := range strings.Split(labels[router+".entrypoints"], ",") {
		switch strings.ToLower(strings.TrimSpace(ep)) {
		case "websecure", "https", "443":
			return true
		}
	}
	return false
}

func traefikPath(rule string) string {
	matches := pathMatcherRe.FindAllStringSubmatch(rule, -1)
	if len(matches) != 1 {
		return ""
	}
	args := matcherArgs(matches[0][1])
	if len(args) != 1 || !strings.HasPrefix(args[0], "/") || args[0] == "/" {
		return ""
	}
	return args[0]
}

func matcherArgs(raw string) []string {
	var args []string
	for _, a := range strings.Split(raw, ",") {
		a = strings.Trim(strings.TrimSpace(a), "`\"'")
		if a != "" {
			args = append(args, a)
		}
	}
	return args
}

func caddyTargets(labels map[string]string) []target {
	var targets []target
	for k, v := range labels {
		if !caddySiteKeyRe.MatchString(k) {
			continue
		}
		internal := strings.EqualFold(strings.TrimSpace(labels[k+".tls"]), "internal")
		fields := strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' })
		for _, addr := range fields {
			t, ok := caddyTarget(addr)
			if !ok {
				continue
			}
			t.internalTLS = internal && t.https
			targets = append(targets, t)
		}
	}
	return targets
}

func caddyTarget(addr string) (target, bool) {
	if addr == "" || strings.HasPrefix(addr, ":") || strings.ContainsAny(addr, "*{") {
		return target{}, false
	}
	explicitHTTP := strings.HasPrefix(strings.ToLower(addr), "http://")
	if !strings.Contains(addr, "://") {
		addr = "https://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return target{}, false
	}
	host := u.Hostname()
	if !validHost(host) {
		return target{}, false
	}
	port := u.Port()

	https := !explicitHTTP && port != "80"
	hostPort := host
	if strings.Contains(host, ":") {
		hostPort = "[" + host + "]"
	}
	switch {
	case https && port != "" && port != "443":
		hostPort = net.JoinHostPort(host, port)
	case !https && port != "" && port != "80":
		hostPort = net.JoinHostPort(host, port)
	}

	scheme := "http"
	if https {
		scheme = "https"
	}
	return target{url: scheme + "://" + hostPort, host: hostPort, https: https}, true
}

func validHost(h string) bool {
	return h != "" && !strings.ContainsAny(h, " \t/{}*`\"'()")
}

func mergeCertificates(existing string, hosts []string) string {
	if len(hosts) == 0 {
		return existing
	}
	entries := make([]string, 0, len(hosts))
	seen := make(map[string]bool)
	for _, e := range strings.Split(existing, ",") {
		e = strings.TrimSpace(e)
		if e != "" && !seen[e] {
			seen[e] = true
			entries = append(entries, e)
		}
	}
	for _, h := range hosts {
		if !seen[h] {
			seen[h] = true
			entries = append(entries, h)
		}
	}
	return strings.Join(entries, ",")
}
