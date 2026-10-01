# Security Policy

## Supported versions

Security fixes land on the latest minor release line only. Older lines receive
no backports: upgrade to the current release before reporting an issue.

| Version | Supported |
| ------- | --------- |
| 1.8.x   | Yes       |
| < 1.8   | No        |

The container image `ghcr.io/kolapsis/maintenant` is rebuilt for every release,
so the `latest` tag always carries the patched base image.

## Reporting a vulnerability

**Do not open a public issue for a security problem.** A public issue exposes
the flaw to every user of the project before a fix exists.

Report privately through GitHub Security Advisories:

<https://github.com/kOlapsis/maintenant/security/advisories/new>

The advisory stays private between you and the maintainers until a fix ships.
Please include, as far as you can establish them:

- the affected version or image digest,
- the deployment shape (standalone container, Docker socket mounted, agent mode),
- reproduction steps or a proof of concept,
- the impact you believe the flaw has.

## Disclosure timeline

We commit to the following, counted from the moment your advisory is received:

- **48 hours**: acknowledgement of receipt.
- **5 days**: first assessment: reproduction, severity, affected versions.
- **30 days**: a fix released, or a written remediation plan with a date.
- **90 days**: coordinated public disclosure, whether or not a fix has shipped.

If you need a different timeline (an upcoming conference talk, an embargo agreed
with another vendor), say so in the advisory and we will coordinate. We credit
reporters in the release notes and in the published advisory unless you ask us
not to.

## Supply chain guarantees

Every release is verifiable by a third party, with no privileged access to this
repository. A release publishes two kinds of artifact: the container image and
the static Linux binaries used by the native install script.

### Container image

The images tagged for a release (`X.Y.Z`, `X.Y`, `X` and `latest`) are signed
and attested. The `main` and commit-SHA tags, published on every merge to
`main`, and the `demo` image are neither signed nor attested: do not rely on
them where verification matters.

Build provenance (SLSA), signed keyless through GitHub OIDC:

```bash
gh attestation verify oci://ghcr.io/kolapsis/maintenant:1.8.0 --owner kOlapsis
```

Cosign signature. Both flags are required: without them, `cosign verify` would
accept any identity.

```bash
cosign verify ghcr.io/kolapsis/maintenant:1.8.0 \
  --certificate-identity-regexp "https://github.com/kOlapsis/maintenant/.github/workflows/release.yml@.*" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

Note that `sigstore/cosign-installer@v4` signs with Cosign 3.x. A local Cosign
2.x will report `no signatures found` on a perfectly signed image; align your
local version before concluding anything.

A CycloneDX SBOM is attached to each release as `sbom.cdx.json` and attested in
the registry alongside the image. `provenance.intoto.jsonl`, also attached to
the release, is the provenance attestation of the image.

### Linux binaries

Each release carries `maintenant-vX.Y.Z-linux-amd64`, `maintenant-vX.Y.Z-linux-arm64`
and `install.sh`, plus:

- `SHA256SUMS`: the checksums of the two binaries and of `install.sh`.
- `SHA256SUMS.bundle`: the keyless Cosign signature of `SHA256SUMS`, with its
  certificate. `install.sh` is covered by this signature through the checksum
  file, and has no provenance attestation of its own.
- A SLSA build provenance attestation for each binary, stored by GitHub and
  checked with `gh attestation verify`.

```bash
VERSION=v1.8.0
ARCH=amd64
BASE=https://github.com/kOlapsis/maintenant/releases/download/${VERSION}

curl -LO ${BASE}/maintenant-${VERSION}-linux-${ARCH}
curl -LO ${BASE}/SHA256SUMS
curl -LO ${BASE}/SHA256SUMS.bundle

# Checksum
sha256sum -c SHA256SUMS --ignore-missing

# Signature of SHA256SUMS (Cosign 3.x)
cosign verify-blob \
  --bundle SHA256SUMS.bundle \
  --certificate-identity-regexp \
    "^https://github\.com/kOlapsis/maintenant/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS

# SLSA provenance
gh attestation verify maintenant-${VERSION}-linux-${ARCH} --owner kOlapsis
```

The native install script runs the checksum check on every install and the
Cosign check when Cosign 3 or later is installed; see
[Supply-chain verification](https://docs.maintenant.dev/install/#supply-chain-verification).

## Hardening the deployment

The threat model of a monitoring agent is dominated by what you grant it. Two
points matter more than the rest:

- **Do not mount `/var/run/docker.sock` directly.** Use a read-only
  docker-socket-proxy; see [Recommended: Docker Socket Proxy](https://docs.maintenant.dev/security/#recommended-docker-socket-proxy)
  for the configuration. A mounted socket is equivalent to root on the host.
- **Do not expose the listener to an untrusted network.** maintenant has no
  login of its own and listens on `127.0.0.1:8080` by default. Inside a
  container it has to listen on `0.0.0.0:8080`, but publish that port only to a
  private interface, or put an authenticating reverse proxy in front; see
  [Reverse Proxy Setup](https://docs.maintenant.dev/security/#reverse-proxy-setup).
