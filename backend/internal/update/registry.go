package update

import (
	"context"
	"fmt"
	"runtime"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// RegistryClient wraps go-containerregistry for read-only registry operations.
type RegistryClient struct{}

// NewRegistryClient creates a new registry client.
func NewRegistryClient() *RegistryClient {
	return &RegistryClient{}
}

func remoteOptions() []remote.Option {
	return []remote.Option{
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	}
}

// ListTags returns all tags for the given image reference.
func (rc *RegistryClient) ListTags(ctx context.Context, imageRef string) ([]string, error) {
	repo, err := name.NewRepository(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse repository %q: %w", imageRef, err)
	}
	tags, err := remote.List(repo, remoteOptions()...)
	if err != nil {
		return nil, fmt.Errorf("list tags for %q: %w", imageRef, err)
	}
	return tags, nil
}

// GetDigest returns the platform-specific digest for the given image reference.
// For multi-arch manifests, it resolves the platform matching the host OS/arch.
func (rc *RegistryClient) GetDigest(ctx context.Context, imageRef string) (string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("parse reference %q: %w", imageRef, err)
	}
	desc, err := remote.Get(ref, remoteOptions()...)
	if err != nil {
		return "", fmt.Errorf("get manifest for %q: %w", imageRef, err)
	}

	// Check if this is a manifest list / OCI index
	switch desc.MediaType {
	case types.OCIImageIndex, types.DockerManifestList:
		digest, err := rc.resolvePlatformDigest(desc)
		if err != nil {
			return "", fmt.Errorf("resolve platform digest for %q: %w", imageRef, err)
		}
		return digest, nil
	default:
		return desc.Digest.String(), nil
	}
}

// GetManifest returns the raw manifest descriptor for the given image reference.
func (rc *RegistryClient) GetManifest(ctx context.Context, imageRef string) (*remote.Descriptor, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse reference %q: %w", imageRef, err)
	}
	desc, err := remote.Get(ref, remoteOptions()...)
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
	desc, err := remote.Get(ref, remoteOptions()...)
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

// resolvePlatformDigest resolves the platform-specific digest from a manifest list.
func (rc *RegistryClient) resolvePlatformDigest(desc *remote.Descriptor) (string, error) {
	idx, err := desc.ImageIndex()
	if err != nil {
		return "", fmt.Errorf("read image index: %w", err)
	}
	manifest, err := idx.IndexManifest()
	if err != nil {
		return "", fmt.Errorf("read index manifest: %w", err)
	}

	targetOS := runtime.GOOS
	targetArch := runtime.GOARCH

	for _, m := range manifest.Manifests {
		if m.Platform != nil && m.Platform.OS == targetOS && m.Platform.Architecture == targetArch {
			return m.Digest.String(), nil
		}
	}

	// Fallback: return the first manifest if no platform match
	if len(manifest.Manifests) > 0 {
		return manifest.Manifests[0].Digest.String(), nil
	}
	return "", fmt.Errorf("no manifests found in index")
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
