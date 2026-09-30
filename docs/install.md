# Native Linux Install

maintenant can run as a standalone systemd service on any amd64 or arm64 Linux host: no Docker, no container runtime required.

The published binaries are statically linked against musl, so a single file per
architecture runs on Debian, Ubuntu, RHEL, Rocky, Alpine or Arch. There is no
libc version to match and nothing to install alongside it.

The install script handles everything: binary download, SHA256 verification, cosign signature check, system user creation, and service activation. Running it again upgrades an existing install.

```bash
curl -fsSL https://install.maintenant.dev | sudo bash
```

!!! warning "No built-in authentication"
    maintenant has no login of its own. The default `127.0.0.1:8080` keeps it local. Before binding another interface, put a reverse proxy with authentication in front, see [Security](security.md).

---

## Prerequisites

- Linux (amd64 or arm64), any distribution: the binary carries its own libc
- `curl` or `wget` (not needed with `--binary`)
- `install` and `sha256sum` (coreutils) and `useradd`
- `systemctl`, unless installing with `--no-service`
- Root access (`sudo`)
- To monitor containers: Docker Engine 19.03 or later (API 1.40)

---

## What the script does

1. Checks the platform and the prerequisites, then works out the install, configuration and data paths.
2. Resolves the version and downloads the binary with `SHA256SUMS` (or takes the local file given with `--binary`), and verifies them.
3. Creates the `maintenant` system user (no login shell, home in the data directory) and adds it to the `docker` group when that group exists, see [The docker group](#the-docker-group).
4. Creates the data directory (`0750`, owned by the service user) and the configuration directory (`0750`, `root:maintenant`).
5. Merges the configuration flags you passed into `/etc/maintenant/maintenant.env`.
6. Replaces the binary atomically (written next to `/usr/local/bin/maintenant`, then renamed).
7. Writes the systemd unit, enables it, then restarts the service if it runs or starts it if not, and waits up to 10 seconds for it to become active.
8. Prints a summary: version, binary path and the address it listens on.

---

## Installation paths

### Minimal: one-liner, all defaults

```bash
curl -fsSL https://install.maintenant.dev | sudo bash
```

Listens on `127.0.0.1:8080` by default. Use a reverse proxy (nginx, Caddy) to expose it externally.

### With custom configuration

Pass any [configuration flag](#configuration-reference) after `--`, as `--flagName value` or `--flagName=value`. They are written to `/etc/maintenant/maintenant.env` and loaded by the systemd service.

```bash
curl -fsSL https://install.maintenant.dev | sudo bash -s -- \
  --addr 0.0.0.0:8080 \
  --baseUrl https://monitoring.example.com \
  --organisationName "Acme Corp" \
  --logLevel info
```

Boolean flags take no value (`--proxyLabels`) or an explicit one (`--proxyLabels=false`); `true`, `false`, `1`, `0`, `yes`, `no`, `on`, `off`, `t`, `f`, `y` and `n` are accepted. `--proxyLabels true`, with a space, is refused: `true` would be read as a separate, unknown argument. Any other flag needs a non-empty, single-line value. For a value that starts with `--`, use the `--flagName=value` form. The script exits with code 2 on an unknown flag, a missing value or a bad boolean.

### Without systemd (containers, CI, minimal hosts)

```bash
curl -fsSL https://install.maintenant.dev | sudo bash -s -- --no-service
```

Installs the binary, the user, the directories and the env file, and no service. Run it manually: `maintenant --addr 0.0.0.0:8080`. If a `maintenant` service already runs, the script warns that it still uses the previous binary, and you restart it yourself.

### Pinning a specific version

```bash
curl -fsSL https://install.maintenant.dev | sudo MAINTENANT_VERSION=v1.8.0 bash -s -- --addr 0.0.0.0:8080
```

The variable is set on the `bash` side of the pipe. `MAINTENANT_VERSION=v1.8.0 curl … | sudo bash` does not pin anything: the assignment applies to `curl`, and `sudo` does not pass it on. Without the variable the script installs the latest release.

Standalone binaries exist from v1.6.0 on. If the version does not exist or predates them, the script exits with code 20 and lists the recent releases (among the 10 latest) that ship binaries. The script is also a release asset: to run the exact script of a release, download `install.sh` from that release's assets.

### Air-gapped (no outbound internet)

`--binary` installs a binary you already have, with no network access at all. Download the files on a machine with internet access:

```bash
VERSION=v1.8.0
BASE=https://github.com/kOlapsis/maintenant/releases/download/${VERSION}

curl -LO ${BASE}/maintenant-${VERSION}-linux-amd64
curl -LO ${BASE}/SHA256SUMS
curl -LO ${BASE}/install.sh
```

Transfer the three files to the target host, then:

```bash
sudo bash install.sh \
  --binary ./maintenant-v1.8.0-linux-amd64 \
  --sha256sums ./SHA256SUMS
```

- `--sha256sums` checks the binary against a `SHA256SUMS` file. The binary can have any file name: it is matched by checksum against a `linux-<arch>` entry of the file, and the version shown in the summary comes from that entry. A checksum that matches nothing for this architecture exits with code 21.
- Without `--sha256sums`, the script warns that the integrity of the binary is not checked and installs it, with the version `local`.
- The cosign signature is never verified offline, and the script says so. To check it, verify `SHA256SUMS` with `SHA256SUMS.bundle` on the connected machine first, see [Manual verification](#manual-verification).
- Neither `curl` nor `wget` is required. Every other flag works as usual (`--no-service`, configuration flags).
- A missing `--binary` file exits with code 2.

---

## Configuration reference

Every `MAINTENANT_*` environment variable in the table has a CLI equivalent. Precedence: **CLI flag > environment variable > built-in default**. The [Configuration](getting-started/configuration.md) page describes what each one does.

| CLI flag | Environment variable | Type | Default |
|---|---|---|---|
| `--addr` | `MAINTENANT_ADDR` | string | `127.0.0.1:8080` |
| `--baseUrl` | `MAINTENANT_BASE_URL` | string | `http://<addr>` |
| `--corsOrigins` | `MAINTENANT_CORS_ORIGINS` | string | _(empty = same-origin)_ |
| `--trustedProxies` | `MAINTENANT_TRUSTED_PROXIES` | string | _(empty = none)_ |
| `--db` | `MAINTENANT_DB` | string | `./maintenant.db` (the service runs in its data directory, so `/var/lib/maintenant/maintenant.db`) |
| `--organisationName` | `MAINTENANT_ORGANISATION_NAME` | string | `Maintenant` |
| `--runtime` | `MAINTENANT_RUNTIME` | string | _(autodetect)_ |
| `--proxyLabels` | `MAINTENANT_PROXY_LABELS` | bool | `false` |
| `--logLevel` | `MAINTENANT_LOG_LEVEL` | string | `info` |
| `--maxBodySize` | `MAINTENANT_MAX_BODY_SIZE` | int | `1048576` |
| `--updateInterval` | `MAINTENANT_UPDATE_INTERVAL` | duration | `24h` |
| `--disableOsEolRefresh` | `MAINTENANT_DISABLE_OS_EOL_REFRESH` | bool | `false` |
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
| `--mcpAllowedRedirectUris` | `MAINTENANT_MCP_ALLOWED_REDIRECT_URIS` | string | _(unset)_ |
| `--mcpAllowUnauthenticated` | `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED` | bool | `false` |
| `--k8sNamespaces` | `MAINTENANT_K8S_NAMESPACES` | string | _(empty = all)_ |
| `--k8sExcludeNamespaces` | `MAINTENANT_K8S_EXCLUDE_NAMESPACES` | string | _(unset)_ |
| `--statusUrl` | `MAINTENANT_STATUS_URL` | string | _(unset)_ |
| `--containerDownAfter` | `MAINTENANT_CONTAINER_DOWN_AFTER` | duration | `0` (off) |
| `--retentionSnapshots` | `MAINTENANT_RETENTION_SNAPSHOTS` | duration | `48h` |
| `--retentionInterval` | `MAINTENANT_RETENTION_INTERVAL` | duration | `1h` |
| `--retentionBatchSize` | `MAINTENANT_RETENTION_BATCH_SIZE` | int | `1000` |
| `--mode` | `MAINTENANT_MODE` | string | `embedded` |
| `--server` | `MAINTENANT_SERVER` | string | _(unset)_ |
| `--enrollment-token` | `MAINTENANT_ENROLLMENT_TOKEN` | string | _(unset)_ |
| `--label` | `MAINTENANT_LABEL` | string | _(unset)_ |
| `--nodeName` | `MAINTENANT_NODE_NAME` | string | _(unset)_ |
| `--grpc-listen` | `MAINTENANT_GRPC_LISTEN` | string | `127.0.0.1:8443` |
| `--grpc-url` | `MAINTENANT_GRPC_URL` | string | _(unset)_ |
| `--grpc-tls-cert` | `MAINTENANT_GRPC_TLS_CERT` | string | _(unset)_ |
| `--grpc-tls-key` | `MAINTENANT_GRPC_TLS_KEY` | string | _(unset)_ |
| `--grpc-insecure-skip-tls-verify` | `MAINTENANT_GRPC_INSECURE_SKIP_TLS_VERIFY` | bool | `false` |
| `--embedded-agent` | `MAINTENANT_EMBEDDED_AGENT` | bool | `false` |
| `--grpc-tls-insecure` | `MAINTENANT_GRPC_TLS_INSECURE` | bool | `false` |
| `--agentRateLimitPerSecond` | `MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND` | int | `1000` |
| `--agentStaleThresholdSeconds` | `MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS` | int | `60` |
| `--agentSpoolMaxMemoryBytes` | `MAINTENANT_AGENT_SPOOL_MAX_MEMORY_BYTES` | int | `16777216` |
| `--agentSpoolMaxDiskBytes` | `MAINTENANT_AGENT_SPOOL_MAX_DISK_BYTES` | int | `134217728` |
| `--agentSpoolMaxAgeSeconds` | `MAINTENANT_AGENT_SPOOL_MAX_AGE_SECONDS` | int | `86400` |
| `--data-dir` | `MAINTENANT_DATA_DIR` | string | `/var/lib/maintenant` |
| `--ca-cert` | `MAINTENANT_CA_CERT` | string | _(unset)_ |
| `--database-url` | `MAINTENANT_DATABASE_URL` | string | _(empty = SQLite)_ |

The spelling is not uniform: most flags are camelCase, while `enrollment-token`, `grpc-*`, `embedded-agent`, `data-dir`, `ca-cert` and `database-url` are kebab-case. Use the names exactly as written; the script rejects any other spelling.

Two options exist only on the binary, and the script refuses them with exit code 2: `--copy-store-to` and `--yes` (copy an install into PostgreSQL, see [PostgreSQL storage](guides/postgresql.md)). `MAINTENANT_DEMO_TOKEN` has no flag at all.

Run `maintenant --help` to see this list with descriptions at any time.

### Script options and environment

| Option | Effect |
|---|---|
| `--no-service` | Do not install or enable the systemd service. |
| `--uninstall` | Remove the binary and the service. Keeps the data, the configuration and the user. |
| `--purge` | With `--uninstall`: also remove the data, the configuration and the user. |
| `--skip-cosign` | Skip the cosign signature check. The SHA256 check stays mandatory. |
| `--binary <path>` | Install this local binary instead of downloading one. |
| `--sha256sums <path>` | With `--binary`: check it against this `SHA256SUMS` file. |
| `--help`, `-h` | Print the usage and exit. |

| Variable | Default | Effect |
|---|---|---|
| `MAINTENANT_VERSION` | `latest` | Release to install. |
| `MAINTENANT_INSTALL_DIR` | `/usr/local/bin` | Where the binary goes. |
| `MAINTENANT_DATA_DIR` | `/var/lib/maintenant` | Data directory. |
| `MAINTENANT_CONFIG_DIR` | `/etc/maintenant` | Configuration directory. |
| `NO_COLOR` | unset | Disable ANSI colors. |

Because `sudo` does not pass the environment through, give these on the `sudo` command line, as in `sudo MAINTENANT_VERSION=v1.8.0 bash`.

### Installing a native agent

A host that only reports to an existing server installs the same binary in agent mode:

```bash
curl -fsSL https://install.maintenant.dev | sudo bash -s -- \
  --mode agent \
  --server grpcs://maintenant.example.com:8443 \
  --enrollment-token TOKEN \
  --label web-01
```

This is the command the *Standalone* tab of the enrollment dialog shows. The agent keeps its identity and its spool in `/var/lib/maintenant`. See [Agent Setup](guides/agent-setup.md) for the server side and the enrollment token.

### The `/etc/maintenant/maintenant.env` file

When you pass configuration flags to the install script, they are written to `/etc/maintenant/maintenant.env` in `KEY=value` format, with `0640` permissions, owner `root` and group `maintenant`. The systemd service loads this file via `EnvironmentFile=`. You can edit it directly:

```bash
sudo nano /etc/maintenant/maintenant.env
sudo systemctl restart maintenant
```

Re-running the script with flags **merges** them into the existing file: only the keys you pass are replaced, or added when missing, and every other line is kept as it is, comments and extra variables such as `DOCKER_HOST` included. The script reports `N keys replaced, M keys added, other lines kept`. The file is untouched when you pass no configuration flag.

The unit applies `ProtectSystem=strict`, `ProtectHome=true` and `PrivateTmp=true`: a file you point maintenant to (a CA bundle, a TLS key) must live outside `/home` and be readable by the `maintenant` user.

### Custom data and database paths

By default the data directory is `/var/lib/maintenant` and the database is `maintenant.db` inside it. `--data-dir` and `--db` move them:

```bash
curl -fsSL https://install.maintenant.dev | sudo bash -s -- \
  --data-dir /srv/maintenant \
  --db /mnt/volume/maintenant/maintenant.db
```

- The data directory is the first of: `--data-dir`, `MAINTENANT_DATA_DIR` in the env file, the `MAINTENANT_DATA_DIR` environment variable of the script, `/var/lib/maintenant`. It must be an absolute path other than `/`.
- The database is the first of: `--db`, `MAINTENANT_DB` in the env file, `./maintenant.db`. A relative path lands in the data directory; an absolute path can point anywhere.
- The script creates both directories, owned by the service user with `0750`, and writes a unit with the matching `WorkingDirectory`, `ReadWritePaths`, `Environment=MAINTENANT_DATA_DIR`, `User`, `ExecStart` and `EnvironmentFile`.
- An existing directory that is not empty and belongs to another owner is refused (exit code 30): use a dedicated directory, or `chown` it to `maintenant` first.
- The install, configuration, data and database directories accept only the characters `A-Z a-z 0-9 . _ / @ + -`; any other character exits with code 2.
- `--uninstall --purge` removes the real data directory. A database directory outside it is kept, with a warning.

### The docker group

If a `docker` group exists when the script runs, the `maintenant` user is added to it, so the service can read the Docker socket. **Membership of the `docker` group is equivalent to root access on the host.** If Docker is installed after maintenant, the user is not added: run the script again, or add it yourself with `usermod -aG docker maintenant` and restart the service. `--uninstall --purge` removes the user from the group before deleting it.

To avoid that access, remove the user from the group (`gpasswd -d maintenant docker`) and point maintenant at a socket proxy with `DOCKER_HOST=tcp://…` in the env file, see [Security](security.md#recommended-docker-socket-proxy). The merge keeps such a line. The next run of the script adds the user to the group again, so repeat the `gpasswd` command after an upgrade.

---

## Upgrade

Re-run the install script. It replaces the binary atomically, rewrites the unit file, reloads systemd, enables the service and restarts it if it is running (it starts the service otherwise). Your data and your env file stay in place.

```bash
curl -fsSL https://install.maintenant.dev | sudo bash
```

To upgrade to a specific version:

```bash
curl -fsSL https://install.maintenant.dev | sudo MAINTENANT_VERSION=v1.8.0 bash
```

With `--no-service` the script replaces the binary and warns that a running service still uses the previous one: restart it with `systemctl restart maintenant`.

---

## Uninstall

```bash
# Remove binary and service, keep data and config
curl -fsSL https://install.maintenant.dev | sudo bash -s -- --uninstall

# Remove everything including /var/lib/maintenant and /etc/maintenant
curl -fsSL https://install.maintenant.dev | sudo bash -s -- --uninstall --purge
```

`--purge` deletes the data directory, the configuration directory and the `maintenant` user. It prompts for confirmation when the script reads from a terminal. In a non-interactive pipe (`curl | bash`), it executes directly: only pass `--purge` when you mean it. If you installed with `--data-dir` or `--db`, pass the same flags (or keep them in the env file) so the script finds what to remove.

---

## Supply-chain verification

Every release includes:

| Asset | Purpose |
|---|---|
| `maintenant-vX.Y.Z-linux-amd64` / `-arm64` | Binary for each architecture |
| `install.sh` | The install script itself, stamped with the release tag |
| `SHA256SUMS` | Checksums for the binaries and the script |
| `SHA256SUMS.bundle` | Sigstore bundle: the cosign signature and its certificate |
| `provenance.intoto.jsonl` | SLSA build provenance of the container image |
| `sbom.cdx.json` | CycloneDX SBOM of the container image |

The binaries have their own SLSA build provenance attestation, stored on GitHub: it is checked with `gh attestation verify` below, and it is not a release asset.

The script published with a release carries that release's tag in its header, and
writes it into `/etc/maintenant/maintenant.env` (`# Script version:`), so a file on
disk always names the script that produced it.

### What the script does automatically

1. Downloads the binary and `SHA256SUMS` into a temp directory.
2. Verifies the binary against its SHA256 checksum: **mandatory**, exits code 21 on mismatch.
3. If `cosign` 3 or later is in `$PATH` and `--skip-cosign` is not set, verifies the `SHA256SUMS` signature from the bundle against the Sigstore transparency log, asserting that the signature was produced by the official `release.yml` workflow on the correct tag.

**cosign 3 is required.** `sign-blob` dropped `--output-signature` and `--output-certificate`, so a release carries a bundle and nothing else, and reading that bundle needs a 3.x binary. An older cosign is treated like an absent one: the script says so and continues, rather than reporting a signature failure that would say nothing about the signature.

The cosign check is **best-effort**: if `cosign` is absent, the script warns and continues. Install it for full supply-chain protection:

```bash
# Install cosign (see https://docs.sigstore.dev/cosign/system_config/installation/)
curl -LO https://github.com/sigstore/cosign/releases/latest/download/cosign-linux-amd64
sudo install -m 0755 cosign-linux-amd64 /usr/local/bin/cosign
```

### Manual verification

```bash
VERSION=v1.8.0
ARCH=amd64
BASE=https://github.com/kOlapsis/maintenant/releases/download/${VERSION}

curl -LO ${BASE}/maintenant-${VERSION}-linux-${ARCH}
curl -LO ${BASE}/SHA256SUMS
curl -LO ${BASE}/SHA256SUMS.bundle

# SHA256
sha256sum -c SHA256SUMS --ignore-missing

# cosign (3.x)
cosign verify-blob \
  --bundle SHA256SUMS.bundle \
  --certificate-identity-regexp \
    "^https://github\.com/kOlapsis/maintenant/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS

# SLSA provenance
gh attestation verify --owner kOlapsis maintenant-${VERSION}-linux-${ARCH}
```

---

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success, or `--help` |
| `1` | Unexpected failure, for example `useradd` failing |
| `2` | Invalid argument: unknown flag, missing or empty value, bad boolean, unsupported character in a path, `--sha256sums` without `--binary`, or a `--binary` or `--sha256sums` file that does not exist |
| `10` | Unsupported OS or architecture |
| `11` | Not running as root |
| `12` | Missing required tool: `curl` or `wget` (unless `--binary`), `install`, `useradd`, or `systemctl` (unless `--no-service`) |
| `20` | Release not found, release without standalone binaries, or download failure |
| `21` | SHA256 checksum mismatch, or (offline) no matching `linux-<arch>` entry in `SHA256SUMS` |
| `22` | cosign signature invalid |
| `30` | Filesystem error, including a non-empty directory owned by someone else |
| `31` | systemd failure: a `systemctl` command failed, or the service was not active after 10 seconds |
