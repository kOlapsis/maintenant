// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/kolapsis/maintenant/internal/ssrf"
	"github.com/kolapsis/maintenant/internal/trust"
)

const registryResponseHeaderTimeout = 30 * time.Second

// RegistryClient wraps go-containerregistry for read-only registry operations.
type RegistryClient struct {
	transport http.RoundTripper
	outbound  http.RoundTripper
}

const registryLookupTimeout = 2 * time.Second

// destinationGuard refuses a request to an address the probe guard blocks when the dial guard cannot see it: an IP literal, or behind a proxy a name that resolves to one.
type destinationGuard struct {
	next   *http.Transport
	lookup func(ctx context.Context, host string) ([]net.IPAddr, error)
}

func (g *destinationGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	if ip := g.blocked(req); ip != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, fmt.Errorf("probe guard: blocked connection to %s", ip)
	}
	return g.next.RoundTrip(req)
}

func (g *destinationGuard) blocked(req *http.Request) net.IP {
	host := req.URL.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if ssrf.IsBlockedForProbe(ip) {
			return ip
		}
		return nil
	}
	if g.next.Proxy == nil {
		return nil
	}
	if proxy, err := g.next.Proxy(req); err != nil || proxy == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(req.Context(), registryLookupTimeout)
	defer cancel()
	addrs, _ := g.lookup(ctx, host)
	for _, a := range addrs {
		if ssrf.IsBlockedForProbe(a.IP) {
			return a.IP
		}
	}
	return nil
}

// NewRegistryClient creates a new registry client.
func NewRegistryClient() *RegistryClient {
	t := trust.HTTPTransport()
	t.ResponseHeaderTimeout = registryResponseHeaderTimeout
	rc := &RegistryClient{transport: t, outbound: t}
	if !ssrf.ProbeGuardDisabled() {
		t.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: ssrf.ProbeControl}).DialContext
		rc.outbound = &destinationGuard{next: t, lookup: net.DefaultResolver.LookupIPAddr}
	}
	return rc
}

func (rc *RegistryClient) remoteOptions() []remote.Option {
	return []remote.Option{
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithTransport(rc.outbound),
	}
}

// ListTags returns all tags for the given image reference.
func (rc *RegistryClient) ListTags(ctx context.Context, imageRef string) ([]string, error) {
	repo, err := name.NewRepository(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse repository %q: %w", imageRef, err)
	}
	tags, err := remote.List(repo, append(rc.remoteOptions(), remote.WithContext(ctx))...)
	if err != nil {
		return nil, fmt.Errorf("list tags for %q: %w", imageRef, err)
	}
	return tags, nil
}

// RemoteDigests is what a reference points at in its registry: the digest of that manifest and, for a multi-platform index, the digests of the platform manifests it lists.
type RemoteDigests struct {
	Digest    string
	Platforms map[string]string // digest of each manifest an index lists -> "os/arch[/variant]", empty for an attestation
}

// Contains reports whether digest names the manifest itself or one of its platform manifests.
func (d RemoteDigests) Contains(digest string) bool {
	_, listed := d.Platforms[digest]
	return digest != "" && (digest == d.Digest || listed)
}

// ResolveDigests returns the digests the given image reference resolves to, the same for every platform.
func (rc *RegistryClient) ResolveDigests(ctx context.Context, imageRef string) (RemoteDigests, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return RemoteDigests{}, fmt.Errorf("parse reference %q: %w", imageRef, err)
	}
	desc, err := remote.Get(ref, append(rc.remoteOptions(), remote.WithContext(ctx))...)
	if err != nil {
		return RemoteDigests{}, fmt.Errorf("get manifest for %q: %w", imageRef, err)
	}

	out := RemoteDigests{Digest: desc.Digest.String()}
	if !desc.MediaType.IsIndex() {
		return out, nil
	}
	idx, err := desc.ImageIndex()
	if err != nil {
		return RemoteDigests{}, fmt.Errorf("read image index for %q: %w", imageRef, err)
	}
	manifest, err := idx.IndexManifest()
	if err != nil {
		return RemoteDigests{}, fmt.Errorf("read index manifest for %q: %w", imageRef, err)
	}
	out.Platforms = make(map[string]string, len(manifest.Manifests))
	for _, m := range manifest.Manifests {
		platform := ""
		if p := m.Platform; p != nil {
			platform = strings.TrimSuffix(p.OS+"/"+p.Architecture+"/"+p.Variant, "/")
		}
		out.Platforms[m.Digest.String()] = platform
	}
	return out, nil
}

// GetManifest returns the raw manifest descriptor for the given image reference.
func (rc *RegistryClient) GetManifest(ctx context.Context, imageRef string) (*remote.Descriptor, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse reference %q: %w", imageRef, err)
	}
	desc, err := remote.Get(ref, append(rc.remoteOptions(), remote.WithContext(ctx))...)
	if err != nil {
		return nil, fmt.Errorf("get manifest for %q: %w", imageRef, err)
	}
	return desc, nil
}

// GetConfigLabels returns the OCI/Docker config labels for the given image reference.
func (rc *RegistryClient) GetConfigLabels(ctx context.Context, imageRef string) (map[string]string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse reference %q: %w", imageRef, err)
	}
	desc, err := remote.Get(ref, append(rc.remoteOptions(), remote.WithContext(ctx))...)
	if err != nil {
		return nil, fmt.Errorf("get manifest for %q: %w", imageRef, err)
	}

	img, err := rc.resolveImage(desc)
	if err != nil {
		return nil, fmt.Errorf("resolve image for %q: %w", imageRef, err)
	}

	cf, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get config file for %q: %w", imageRef, err)
	}
	if cf == nil {
		return nil, nil
	}
	return cf.Config.Labels, nil
}

// resolveImage resolves a single v1.Image from a descriptor, handling manifest lists.
func (rc *RegistryClient) resolveImage(desc *remote.Descriptor) (v1.Image, error) {
	switch desc.MediaType {
	case types.OCIImageIndex, types.DockerManifestList:
		idx, err := desc.ImageIndex()
		if err != nil {
			return nil, err
		}
		manifest, err := idx.IndexManifest()
		if err != nil {
			return nil, err
		}
		targetOS := runtime.GOOS
		targetArch := runtime.GOARCH
		for _, m := range manifest.Manifests {
			if m.Platform != nil && m.Platform.OS == targetOS && m.Platform.Architecture == targetArch {
				return idx.Image(m.Digest)
			}
		}
		if len(manifest.Manifests) > 0 {
			return idx.Image(manifest.Manifests[0].Digest)
		}
		return nil, fmt.Errorf("no manifests in index")
	default:
		return desc.Image()
	}
}
