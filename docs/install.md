# Native Linux Install

maintenant can run as a standalone systemd service on any amd64 or arm64 Linux host — no Docker, no container runtime required.

The install script handles everything: binary download, SHA256 verification, cosign signature check, system user creation, and service activation.

```bash
curl -fsSL https://install.maintenant.dev | sudo bash
```

---

## Prerequisites

- Linux (amd64 or arm64)
- `curl` or `wget`
- `tar`, `useradd`, `systemctl`
- Root access (`sudo`)

---

## Installation paths

### Minimal — one-liner, all defaults

```bash
curl -fsSL https://install.maintenant.dev | sudo bash
```

Listens on `127.0.0.1:8080` by default. Use a reverse proxy (nginx, Caddy) to expose it externally.

### With custom configuration

Pass any `--flagName value` after `--`. These are written to `/etc/maintenant/maintenant.env` and loaded by the systemd service.

```bash
curl -fsSL https://install.maintenant.dev | sudo bash -s -- \
  --addr 0.0.0.0:8080 \
  --baseUrl https://monitoring.example.com \
  --organisationName "Acme Corp" \
  --logLevel info
```

### Without systemd (containers, CI, minimal hosts)

```bash
curl -fsSL https://install.maintenant.dev | sudo bash -s -- --no-service
```

Installs the binary only. Run it manually: `maintenant --addr 0.0.0.0:8080`.

### Pinning a specific version

```bash
MAINTENANT_VERSION=v1.6.0 curl -fsSL https://install.maintenant.dev | sudo -E bash
```

The `-E` flag preserves the environment variable through `sudo`. If the version doesn't include standalone binaries (pre-v1.6.0), the script exits with code 20 and lists the 5 most recent valid versions.

### Air-gapped (no outbound internet)

```bash
# On a machine with internet access:
curl -LO https://github.com/kolapsis/maintenant/releases/download/v1.6.0/maintenant-v1.6.0-linux-amd64
curl -LO https://github.com/kolapsis/maintenant/releases/download/v1.6.0/SHA256SUMS

# Transfer both files to the target host, then:
sudo bash -s -- --no-service --skip-cosign
# or install the binary manually:
sudo install -m 0755 maintenant-v1.6.0-linux-amd64 /usr/local/bin/maintenant
```

---

## Configuration reference

Every `MAINTENANT_*` environment variable has a `--flagName` CLI equivalent. Precedence: **CLI flag > environment variable > built-in default**.

| CLI flag | Environment variable | Type | Default |
|---|---|---|---|
| `--addr` | `MAINTENANT_ADDR` | string | `127.0.0.1:8080` |
| `--baseUrl` | `MAINTENANT_BASE_URL` | string | `http://<addr>` |
| `--corsOrigins` | `MAINTENANT_CORS_ORIGINS` | string | _(empty = same-origin)_ |
| `--db` | `MAINTENANT_DB` | string | `./maintenant.db` |
| `--organisationName` | `MAINTENANT_ORGANISATION_NAME` | string | `Maintenant` |
| `--runtime` | `MAINTENANT_RUNTIME` | string | _(autodetect)_ |
| `--logLevel` | `MAINTENANT_LOG_LEVEL` | string | `info` |
| `--maxBodySize` | `MAINTENANT_MAX_BODY_SIZE` | int | `1048576` |
| `--updateInterval` | `MAINTENANT_UPDATE_INTERVAL` | duration | `24h` |
| `--securityScoreThreshold` | `MAINTENANT_SECURITY_SCORE_THRESHOLD` | int | _(unset)_ |
| `--disableTelemetry` | `MAINTENANT_DISABLE_TELEMETRY` | bool | `false` |
| `--allowPrivateWebhooks` | `MAINTENANT_ALLOW_PRIVATE_WEBHOOKS` | bool | `false` |
| `--licenseKey` | `MAINTENANT_LICENSE_KEY` | string | _(unset)_ |
| `--smtpHost` | `MAINTENANT_SMTP_HOST` | string | _(unset)_ |
| `--smtpPort` | `MAINTENANT_SMTP_PORT` | string | `587` |
| `--smtpUsername` | `MAINTENANT_SMTP_USERNAME` | string | _(unset)_ |
| `--smtpPassword` | `MAINTENANT_SMTP_PASSWORD` | string | _(unset)_ |
| `--smtpFrom` | `MAINTENANT_SMTP_FROM` | string | `maintenant@localhost` |
| `--mcp` | `MAINTENANT_MCP` | bool | `false` |
| `--mcpClientId` | `MAINTENANT_MCP_CLIENT_ID` | string | _(unset)_ |
| `--mcpClientSecret` | `MAINTENANT_MCP_CLIENT_SECRET` | string | _(unset)_ |
| `--k8sNamespaces` | `MAINTENANT_K8S_NAMESPACES` | string | _(empty = all)_ |
| `--k8sExcludeNamespaces` | `MAINTENANT_K8S_EXCLUDE_NAMESPACES` | string | _(unset)_ |
| `--statusUrl` | `MAINTENANT_STATUS_URL` | string | _(unset)_ |
| `--retentionSnapshots` | `MAINTENANT_RETENTION_SNAPSHOTS` | duration | `48h` |
| `--retentionInterval` | `MAINTENANT_RETENTION_INTERVAL` | duration | `1h` |
| `--retentionBatchSize` | `MAINTENANT_RETENTION_BATCH_SIZE` | int | `1000` |
| `--mode` | `MAINTENANT_MODE` | string | `embedded` |
| `--server` | `MAINTENANT_SERVER` | string | _(unset)_ |
| `--enrollment-token` | `MAINTENANT_ENROLLMENT_TOKEN` | string | _(unset)_ |
| `--label` | `MAINTENANT_LABEL` | string | _(unset)_ |
| `--grpc-listen` | `MAINTENANT_GRPC_LISTEN` | string | `127.0.0.1:8443` |
| `--grpc-url` | `MAINTENANT_GRPC_URL` | string | _(unset)_ |
| `--grpc-tls-cert` | `MAINTENANT_GRPC_TLS_CERT` | string | _(unset)_ |
| `--grpc-tls-key` | `MAINTENANT_GRPC_TLS_KEY` | string | _(unset)_ |
| `--grpc-insecure-skip-tls-verify` | `MAINTENANT_GRPC_INSECURE_SKIP_TLS_VERIFY` | bool | `false` |
| `--embedded-agent` | `MAINTENANT_EMBEDDED_AGENT` | bool | `false` |
| `--ca-cert` | `MAINTENANT_CA_CERT` | string | _(unset)_ |
| `--database-url` | `MAINTENANT_DATABASE_URL` | string | _(empty = SQLite)_ |

The multi-host flags keep their kebab-case spelling: they shipped before the
flag registry existed and are baked into running agents and systemd units.

### Installing a native agent

A host that only reports to an existing server installs the same binary in agent
mode:

```sh
curl -fsSL https://install.maintenant.dev | sudo sh -s -- \
  --mode agent \
  --server grpcs://maintenant.example.com:8443 \
  --enrollment-token TOKEN \
  --label web-01
```

Run `maintenant --help` to see this list with descriptions at any time.

### The `/etc/maintenant/maintenant.env` file

When you pass configuration flags to the install script, they are written to `/etc/maintenant/maintenant.env` in `KEY=value` format. The systemd service loads this file via `EnvironmentFile=`. You can edit it directly:

```bash
sudo nano /etc/maintenant/maintenant.env
sudo systemctl restart maintenant
```

Re-running the script with new flags **merges** them into the existing file — keys you don't mention are preserved.

---

## Upgrade

Re-run the install script. It overwrites the binary atomically, updates the service file if changed, and restarts the service.

```bash
curl -fsSL https://install.maintenant.dev | sudo bash
```

To upgrade to a specific version:

```bash
MAINTENANT_VERSION=v1.7.0 curl -fsSL https://install.maintenant.dev | sudo -E bash
```

---

## Uninstall

```bash
# Remove binary and service, keep data and config
curl -fsSL https://install.maintenant.dev | sudo bash -s -- --uninstall

# Remove everything including /var/lib/maintenant and /etc/maintenant
curl -fsSL https://install.maintenant.dev | sudo bash -s -- --uninstall --purge
```

`--purge` prompts for confirmation if run interactively. In a non-interactive pipe (`curl | bash`), it executes directly — only pass `--purge` when you mean it.

---

## Supply-chain verification

Every release includes:

| Asset | Purpose |
|---|---|
| `maintenant-vX.Y.Z-linux-amd64` / `-arm64` | Binary for each architecture |
| `SHA256SUMS` | Checksums for all binaries |
| `SHA256SUMS.sig` | cosign signature of `SHA256SUMS` |
| `SHA256SUMS.pem` | Sigstore ephemeral certificate |
| `provenance.intoto.jsonl` | SLSA Build L3 attestation |

### What the script does automatically

1. Downloads the binary and `SHA256SUMS` into a temp directory.
2. Verifies the binary against its SHA256 checksum — **mandatory**, exits code 21 on mismatch.
3. If `cosign` is in `$PATH` and `--skip-cosign` is not set, verifies the `SHA256SUMS` signature against the Sigstore transparency log, asserting that the signature was produced by the official `release.yml` workflow on the correct tag.

The cosign check is **best-effort**: if `cosign` is absent, the script warns and continues. Install it for full supply-chain protection:

```bash
# Install cosign (see https://docs.sigstore.dev/cosign/system_config/installation/)
curl -LO https://github.com/sigstore/cosign/releases/latest/download/cosign-linux-amd64
sudo install -m 0755 cosign-linux-amd64 /usr/local/bin/cosign
```

### Manual verification

```bash
VERSION=v1.6.0
ARCH=amd64
BASE=https://github.com/kolapsis/maintenant/releases/download/${VERSION}

curl -LO ${BASE}/maintenant-${VERSION}-linux-${ARCH}
curl -LO ${BASE}/SHA256SUMS
curl -LO ${BASE}/SHA256SUMS.sig
curl -LO ${BASE}/SHA256SUMS.pem

# SHA256
sha256sum -c SHA256SUMS --ignore-missing

# cosign
cosign verify-blob \
  --certificate SHA256SUMS.pem \
  --signature SHA256SUMS.sig \
  --certificate-identity-regexp \
    "^https://github\.com/kolapsis/maintenant/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS

# SLSA provenance
gh attestation verify --owner kolapsis maintenant-${VERSION}-linux-${ARCH}
```

---

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `2` | Invalid argument |
| `10` | Unsupported OS or architecture |
| `11` | Not running as root |
| `12` | Missing required tool (`curl`/`wget`, `tar`, `useradd`, `systemctl`) |
| `20` | Version not found or predates standalone binary support |
| `21` | SHA256 checksum mismatch |
| `22` | cosign signature invalid |
| `30` | Filesystem write error |
| `31` | systemd service failed to start |
