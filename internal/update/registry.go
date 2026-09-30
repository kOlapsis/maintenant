// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"slices"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/kolapsis/maintenant/internal/trust"
)

// RegistryClient wraps go-containerregistry for read-only registry operations.
type RegistryClient struct {
	transport http.RoundTripper
}

// NewRegistryClient creates a new registry client.
func NewRegistryClient() *RegistryClient {
	return &RegistryClient{transport: trust.HTTPTransport()}
}

func (rc *RegistryClient) remoteOptions() []remote.Option {
	return []remote.Option{
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithTransport(rc.transport),
	}
}

// ListTags returns all tags for the given image reference.
func (rc *RegistryClient) ListTags(ctx context.Context, imageRef string) ([]string, error) {
	repo, err := name.NewRepository(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse repository %q: %w", imageRef, err)
	}
	tags, err := remote.List(repo, rc.remoteOptions()...)
	if err != nil {
		return nil, fmt.Errorf("list tags for %q: %w", imageRef, err)
	}
	return tags, nil
}

// RemoteDigests is what a reference points at in its registry: the digest of that manifest and, for a multi-platform index, the digests of the platform manifests it lists.
type RemoteDigests struct {
	Digest    string
	Platforms []string
}

// Contains reports whether digest names the manifest itself or one of its platform manifests.
func (d RemoteDigests) Contains(digest string) bool {
	return digest != "" && (digest == d.Digest || slices.Contains(d.Platforms, digest))
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
	for _, m := range manifest.Manifests {
		out.Platforms = append(out.Platforms, m.Digest.String())
	}
	return out, nil
}

// GetManifest returns the raw manifest descriptor for the given image reference.
func (rc *RegistryClient) GetManifest(ctx context.Context, imageRef string) (*remote.Descriptor, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse reference %q: %w", imageRef, err)
	}
	desc, err := remote.Get(ref, rc.remoteOptions()...)
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
	desc, err := remote.Get(ref, rc.remoteOptions()...)
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
