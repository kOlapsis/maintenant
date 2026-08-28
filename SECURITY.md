# Security Policy

## Supported versions

Security fixes land on the latest minor release line only. Older lines receive
no backports: upgrade to the current release before reporting an issue.

| Version | Supported |
| ------- | --------- |
| 1.3.x   | Yes       |
| < 1.3   | No        |

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

- **48 hours** — acknowledgement of receipt.
- **5 days** — first assessment: reproduction, severity, affected versions.
- **30 days** — a fix released, or a written remediation plan with a date.
- **90 days** — coordinated public disclosure, whether or not a fix has shipped.

If you need a different timeline (an upcoming conference talk, an embargo agreed
with another vendor), say so in the advisory and we will coordinate. We credit
reporters in the release notes and in the published advisory unless you ask us
not to.

## Supply chain guarantees

Every released image is verifiable by a third party, with no privileged access
to this repository.

Build provenance (SLSA), signed keyless through GitHub OIDC:

```bash
gh attestation verify oci://ghcr.io/kolapsis/maintenant:1.3.7 --owner kOlapsis
```

Cosign signature. Both flags are required: without them, `cosign verify` would
accept any identity.

```bash
cosign verify ghcr.io/kolapsis/maintenant:1.3.7 \
  --certificate-identity-regexp "https://github.com/kOlapsis/maintenant/.github/workflows/release.yml@.*" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

Note that `sigstore/cosign-installer@v4` signs with Cosign 3.x. A local Cosign
2.x will report `no signatures found` on a perfectly signed image; align your
local version before concluding anything.

A CycloneDX SBOM is attached to each release as `sbom.cdx.json` and attested in
the registry alongside the image.

## Hardening the deployment

The threat model of a monitoring agent is dominated by what you grant it. Two
points matter more than the rest:

- **Do not mount `/var/run/docker.sock` directly.** Use a read-only
  docker-socket-proxy; see `docs/` for the recommended configuration. A mounted
  socket is equivalent to root on the host.
- **Do not bind the listener to `0.0.0.0`** on a machine reachable from an
  untrusted network. Bind to a private interface and put a TLS terminator in
  front.
