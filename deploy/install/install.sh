#!/usr/bin/env sh
# Maintenant install script
# Usage: curl -fsSL https://install.maintenant.dev | sudo bash -s -- [FLAGS]
# Flags: see --help
set -eu

# ── Constants ────────────────────────────────────────────────────────────────

INSTALL_DIR="${INSTALL_DIR:-${MAINTENANT_INSTALL_DIR:-/usr/local/bin}}"
DATA_DIR="${DATA_DIR:-${MAINTENANT_DATA_DIR:-/var/lib/maintenant}}"
CONFIG_DIR="${CONFIG_DIR:-${MAINTENANT_CONFIG_DIR:-/etc/maintenant}}"
SERVICE_USER="${SERVICE_USER:-maintenant}"
SERVICE_FILE="${SERVICE_FILE:-/etc/systemd/system/maintenant.service}"
GITHUB_REPO="kolapsis/maintenant"
GITHUB_API="https://api.github.com"
SCRIPT_VERSION="__GIT_SHA__"

# ── Color / output ────────────────────────────────────────────────────────────

_color() {
    if [ -n "${NO_COLOR:-}" ] || [ ! -t 1 ]; then
        printf '%s\n' "$2"
    else
        printf '\033[%sm%s\033[0m\n' "$1" "$2"
    fi
}
log_info()  { _color "32" "  ✓ $*"; }
log_warn()  { _color "33" "  ⚠ $*" >&2; }
log_error() { _color "31" "  ✗ $*" >&2; }
log_step()  { _color "36" "==> $*"; }

abort() {
    log_error "$1"
    exit "${2:-1}"
}

# ── Cleanup trap ──────────────────────────────────────────────────────────────

TMPDIR_INSTALL="${TMPDIR_INSTALL:-}"
cleanup() {
    if [ -n "${TMPDIR_INSTALL:-}" ]; then rm -rf "$TMPDIR_INSTALL"; fi
}
trap cleanup EXIT

# ── Usage ─────────────────────────────────────────────────────────────────────

usage() {
    cat <<'EOF'
Usage: install.sh [SCRIPT FLAGS] [BINARY FLAGS]

Script flags:
  --no-service          Do not install or enable the systemd service
  --uninstall           Remove Maintenant (keeps data and user by default)
  --purge               With --uninstall: also remove data dir, config, user
  --skip-cosign         Skip cosign signature check (SHA256 still required)
  --help, -h            Show this help

Binary configuration flags (written to /etc/maintenant/maintenant.env):
  --addr <host:port>
  --baseUrl <url>
  --db <path>
  --organisationName <name>
  --corsOrigins <list>
  --runtime <docker|kubernetes>
  --logLevel <level>
  --maxBodySize <bytes>
  --updateInterval <duration>
  --securityScoreThreshold <int>
  --disableTelemetry
  --allowPrivateWebhooks
  --licenseKey <key>
  --smtpHost <host>
  --smtpPort <port>
  --smtpUsername <user>
  --smtpPassword <pass>
  --smtpFrom <addr>
  --mcp
  --mcpClientId <id>
  --mcpClientSecret <secret>
  --k8sNamespaces <list>
  --k8sExcludeNamespaces <list>
  --statusUrl <url>
  --retentionSnapshots <duration>
  --retentionInterval <duration>
  --retentionBatchSize <int>
  --mode <embedded|server|agent>
  --server <url>
  --enrollment-token <token>
  --label <name>
  --grpc-listen <host:port>
  --grpc-url <url>
  --grpc-tls-cert <path>
  --grpc-tls-key <path>
  --grpc-insecure-skip-tls-verify
  --embedded-agent
  --ca-cert <path>
  --database-url <postgres-url>

Examples:
  # Standalone server on this host
  install.sh --addr 0.0.0.0:8080 --baseUrl https://maintenant.example.com

  # Native agent reporting to an existing server
  install.sh --mode agent --server grpcs://maintenant.example.com:8443 \
             --enrollment-token TOKEN --label web-01

Environment variables:
  MAINTENANT_VERSION       Version to install (default: latest)
  MAINTENANT_INSTALL_DIR   Binary install path (default: /usr/local/bin)
  MAINTENANT_DATA_DIR      Data directory (default: /var/lib/maintenant)
  MAINTENANT_CONFIG_DIR    Config directory (default: /etc/maintenant)
  NO_COLOR                 Disable ANSI colors

EOF
}

# ── detect_platform ───────────────────────────────────────────────────────────

detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64) ARCH=amd64 ;;
        aarch64|arm64) ARCH=arm64 ;;
        *) abort "Unsupported architecture: $ARCH (only amd64 and arm64 are supported)" 10 ;;
    esac
    [ "$OS" = "linux" ] || abort "Unsupported OS: $OS (only Linux is supported)" 10
}

# ── check_prereqs ─────────────────────────────────────────────────────────────

check_prereqs() {
    [ "$(id -u)" -eq 0 ] || abort "This script must be run as root (EUID 0)" 11

    # curl or wget
    if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
        abort "curl or wget is required" 12
    fi

    for cmd in tar install useradd; do
        command -v "$cmd" >/dev/null 2>&1 || abort "Required command not found: $cmd" 12
    done

    if [ -z "${NO_SERVICE:-}" ]; then
        command -v systemctl >/dev/null 2>&1 || abort "systemctl is required (use --no-service to skip)" 12
    fi
}

# ── resolve_version ───────────────────────────────────────────────────────────

resolve_version() {
    VERSION="${MAINTENANT_VERSION:-latest}"

    if [ "$VERSION" = "latest" ]; then
        log_step "Resolving latest version..."
        VERSION=$(fetch_url "$GITHUB_API/repos/$GITHUB_REPO/releases/latest" \
            | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
        [ -n "$VERSION" ] || abort "Failed to resolve latest version from GitHub API" 20
        log_info "Latest version: $VERSION"
        return
    fi

    # Pinned version — verify release exists and contains standalone binaries
    log_step "Verifying version $VERSION..."
    RELEASE_JSON=$(fetch_url "$GITHUB_API/repos/$GITHUB_REPO/releases/tags/$VERSION" 2>/dev/null) || {
        _suggest_versions
        abort "Version $VERSION not found on GitHub Releases" 20
    }

    # Check that standalone binaries exist in this release
    ASSET_PATTERN="maintenant-${VERSION}-linux-"
    if ! printf '%s' "$RELEASE_JSON" | grep -q "\"name\": *\"${ASSET_PATTERN}"; then
        log_error "Version $VERSION exists but does not include standalone binaries."
        log_error "Standalone binaries were introduced starting from a later release."
        _suggest_versions
        abort "Version $VERSION predates standalone binary support" 20
    fi

    log_info "Version $VERSION verified"
}

_suggest_versions() {
    log_warn "Recent versions with standalone binaries:"
    RECENT=$(fetch_url "$GITHUB_API/repos/$GITHUB_REPO/releases?per_page=5" 2>/dev/null \
        | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/') || true
    if [ -n "$RECENT" ]; then
        printf '%s\n' "$RECENT" | while IFS= read -r v; do
            log_warn "  $v"
        done
    fi
}

# ── download_and_verify ───────────────────────────────────────────────────────

download_and_verify() {
    TMPDIR_INSTALL=$(mktemp -d)
    ASSET_NAME="maintenant-${VERSION}-linux-${ARCH}"
    BASE_URL="https://github.com/$GITHUB_REPO/releases/download/${VERSION}"

    log_step "Downloading $ASSET_NAME..."
    fetch_url_to "$BASE_URL/$ASSET_NAME"       "$TMPDIR_INSTALL/$ASSET_NAME"
    fetch_url_to "$BASE_URL/SHA256SUMS"        "$TMPDIR_INSTALL/SHA256SUMS"

    log_step "Verifying SHA256 checksum..."
    (cd "$TMPDIR_INSTALL" && sha256sum -c SHA256SUMS --ignore-missing) \
        || abort "SHA256 checksum mismatch — download may be corrupted" 21

    if [ -z "${SKIP_COSIGN:-}" ] && command -v cosign >/dev/null 2>&1; then
        fetch_url_to "$BASE_URL/SHA256SUMS.sig" "$TMPDIR_INSTALL/SHA256SUMS.sig"
        fetch_url_to "$BASE_URL/SHA256SUMS.pem" "$TMPDIR_INSTALL/SHA256SUMS.pem"
        log_step "Verifying cosign signature..."
        if ! cosign verify-blob \
            --certificate "$TMPDIR_INSTALL/SHA256SUMS.pem" \
            --signature "$TMPDIR_INSTALL/SHA256SUMS.sig" \
            --certificate-identity-regexp "^https://github\\.com/$GITHUB_REPO/\\.github/workflows/release\\.yml@refs/tags/v[0-9]+\\.[0-9]+\\.[0-9]+\$" \
            --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
            "$TMPDIR_INSTALL/SHA256SUMS"; then
            abort "cosign signature verification failed" 22
        fi
        log_info "cosign signature verified"
    elif [ -z "${SKIP_COSIGN:-}" ]; then
        log_warn "cosign not found — skipping signature verification (install cosign for supply-chain protection)"
    else
        log_warn "cosign verification skipped (--skip-cosign)"
    fi
}

# ── ensure_user ───────────────────────────────────────────────────────────────

ensure_user() {
    if id "$SERVICE_USER" >/dev/null 2>&1; then
        log_info "User $SERVICE_USER already exists"
    else
        log_step "Creating system user $SERVICE_USER..."
        useradd -r -s /usr/sbin/nologin -d "$DATA_DIR" \
            -c "Maintenant service user" "$SERVICE_USER"
        log_info "User $SERVICE_USER created"
    fi

    if getent group docker >/dev/null 2>&1; then
        if id -nG "$SERVICE_USER" | grep -qw docker; then
            log_info "User $SERVICE_USER already in docker group"
        else
            usermod -aG docker "$SERVICE_USER" 2>/dev/null \
                || log_warn "Could not add $SERVICE_USER to docker group"
        fi
    fi
}

# ── install_binary ────────────────────────────────────────────────────────────

install_binary() {
    log_step "Installing binary..."
    mkdir -p "$DATA_DIR"
    chown "$SERVICE_USER:$SERVICE_USER" "$DATA_DIR"
    chmod 0750 "$DATA_DIR"

    mkdir -p "$CONFIG_DIR"
    chown "root:$SERVICE_USER" "$CONFIG_DIR"
    chmod 0750 "$CONFIG_DIR"

    install -m 0755 -o root -g root \
        "$TMPDIR_INSTALL/maintenant-${VERSION}-linux-${ARCH}" \
        "$INSTALL_DIR/maintenant"
    log_info "Binary installed to $INSTALL_DIR/maintenant"
}

# ── install_service ───────────────────────────────────────────────────────────

install_service() {
    [ -z "${NO_SERVICE:-}" ] || { log_info "Skipping service installation (--no-service)"; return; }

    log_step "Installing systemd service..."
    cat > "$SERVICE_FILE" <<'UNIT'
[Unit]
Description=Maintenant infrastructure monitoring
Documentation=https://docs.maintenant.dev
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=maintenant
Group=maintenant
EnvironmentFile=-/etc/maintenant/maintenant.env
ExecStart=/usr/local/bin/maintenant
WorkingDirectory=/var/lib/maintenant
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/maintenant
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictNamespaces=true
RestrictRealtime=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    systemctl enable --now maintenant

    log_step "Waiting for service to become active..."
    i=0
    while [ $i -lt 10 ]; do
        if systemctl is-active --quiet maintenant; then
            log_info "Service is active"
            return
        fi
        sleep 1
        i=$((i + 1))
    done

    log_error "Service failed to start. Last 50 log lines:"
    journalctl -u maintenant -n 50 --no-pager >&2 || true
    abort "Maintenant service did not become active within 10 seconds" 31
}

# ── print_summary ─────────────────────────────────────────────────────────────

print_summary() {
    LISTEN_ADDR="${MAINTENANT_ADDR:-127.0.0.1:8080}"
    cat <<EOF

  ╔══════════════════════════════════════════════════════╗
  ║          Maintenant installed successfully           ║
  ╠══════════════════════════════════════════════════════╣
  ║  Version : $VERSION
  ║  Binary  : $INSTALL_DIR/maintenant
  ║  Listens : http://$LISTEN_ADDR
  ╠══════════════════════════════════════════════════════╣
  ║  Commands:
  ║    systemctl status maintenant
  ║    journalctl -fu maintenant
  ╚══════════════════════════════════════════════════════╝

EOF
}

# ── parse_maintenant_flags ────────────────────────────────────────────────────
# Separates script-own flags from binary configuration flags.
# Sets: NO_SERVICE, DO_UNINSTALL, DO_PURGE, SKIP_COSIGN
# Populates: BINARY_FLAGS associative-style via BINARY_FLAG_KEYS / BINARY_FLAG_VALS

BINARY_FLAG_KEYS=""
BINARY_FLAG_VALS=""

_store_binary_flag() {
    key="$1"
    val="$2"
    BINARY_FLAG_KEYS="${BINARY_FLAG_KEYS}${key}
"
    BINARY_FLAG_VALS="${BINARY_FLAG_VALS}${val}
"
}

_get_binary_flag_val() {
    # Returns value for key $1 (newline-separated parallel lists)
    key="$1"
    line=0
    printf '%s\n' "$BINARY_FLAG_KEYS" | while IFS= read -r k; do
        line=$((line + 1))
        if [ "$k" = "$key" ]; then
            printf '%s\n' "$BINARY_FLAG_VALS" | sed -n "${line}p"
            return
        fi
    done
}

parse_maintenant_flags() {
    NO_SERVICE=""
    DO_UNINSTALL=""
    DO_PURGE=""
    # Kept from the environment when already set: MAINTENANT_SKIP_COSIGN and the
    # test harness both drive it that way.
    SKIP_COSIGN="${SKIP_COSIGN:-${MAINTENANT_SKIP_COSIGN:-}}"
    BINARY_FLAG_KEYS=""
    BINARY_FLAG_VALS=""

    while [ $# -gt 0 ]; do
        case "$1" in
            --no-service)    NO_SERVICE=1; shift ;;
            --uninstall)     DO_UNINSTALL=1; shift ;;
            --purge)         DO_PURGE=1; shift ;;
            --skip-cosign)   SKIP_COSIGN=1; shift ;;
            --help|-h)       usage; exit 0 ;;
            --*)
                FLAG="${1#--}"
                # Boolean binary flags (no value)
                case "$FLAG" in
                    disableTelemetry|allowPrivateWebhooks|mcp|embedded-agent|grpc-insecure-skip-tls-verify)
                        _store_binary_flag "$FLAG" "true"
                        shift
                        ;;
                    *)
                        # Value-taking flag: require next argument
                        [ $# -gt 1 ] || abort "Flag --$FLAG requires a value" 2
                        _store_binary_flag "$FLAG" "$2"
                        shift 2
                        ;;
                esac
                ;;
            *)
                abort "Unknown argument: $1" 2
                ;;
        esac
    done
}

# ── flag_to_env ───────────────────────────────────────────────────────────────
# Converts camelCase flag name to MAINTENANT_SCREAMING_SNAKE_CASE env name.
# Algorithm: inverse of R5. Kebab-case multi-host flags (--grpc-listen) map the
# same way, their separator is already the one the env name uses.

flag_to_env() {
    flagname="$1"
    # Insert underscore before each uppercase letter, then upper-case all
    result=$(printf '%s' "$flagname" \
        | sed 's/\([A-Z]\)/_\1/g; s/-/_/g' \
        | tr '[:lower:]' '[:upper:]')
    printf 'MAINTENANT_%s' "$result"
}

# ── merge_env_file ────────────────────────────────────────────────────────────
# Implements R7: idempotent key-by-key merge into /etc/maintenant/maintenant.env

merge_env_file() {
    ENV_FILE="$CONFIG_DIR/maintenant.env"
    NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    HEADER="# Generated by install.sh
# Last updated: $NOW
# Script version: $SCRIPT_VERSION
# Edit this file directly then: systemctl restart maintenant"

    # Build new key=value pairs from provided binary flags
    printf '%s\n' "$BINARY_FLAG_KEYS" | while IFS= read -r flagname; do
        [ -n "$flagname" ] || continue
        envname=$(flag_to_env "$flagname")
        # Use parallel list trick — find index
        idx=0
        printf '%s\n' "$BINARY_FLAG_KEYS" | while IFS= read -r k; do
            idx=$((idx + 1))
            if [ "$k" = "$flagname" ]; then
                val=$(printf '%s\n' "$BINARY_FLAG_VALS" | sed -n "${idx}p")
                printf '%s=%s\n' "$envname" "$val"
                break
            fi
        done
    done > "$TMPDIR_INSTALL/new_flags.env"

    if [ ! -f "$ENV_FILE" ]; then
        # First creation
        {
            printf '%s\n\n' "$HEADER"
            cat "$TMPDIR_INSTALL/new_flags.env"
        } > "$ENV_FILE"
        chown "root:$SERVICE_USER" "$ENV_FILE"
        chmod 0640 "$ENV_FILE"
        log_info "Created $ENV_FILE"
        return
    fi

    # Merge: read existing, override with new
    KEYS_COUNT=0
    UPDATES=0

    # Read existing non-comment lines
    grep -E '^MAINTENANT_[A-Z_]+=.*$' "$ENV_FILE" > "$TMPDIR_INSTALL/existing.env" 2>/dev/null || true

    # Build merged file
    {
        printf '%s\n\n' "$HEADER"

        # Start with existing entries, override if in new_flags
        while IFS='=' read -r key rest; do
            [ -n "$key" ] || continue
            val="$rest"
            # Check if this key appears in new flags
            new_val=$(grep "^${key}=" "$TMPDIR_INSTALL/new_flags.env" | cut -d= -f2-)
            if [ -n "$new_val" ]; then
                printf '%s=%s\n' "$key" "$new_val"
                UPDATES=$((UPDATES + 1))
            else
                printf '%s=%s\n' "$key" "$val"
            fi
            KEYS_COUNT=$((KEYS_COUNT + 1))
        done < "$TMPDIR_INSTALL/existing.env"

        # Add new keys not already in existing
        while IFS='=' read -r key rest; do
            [ -n "$key" ] || continue
            if ! grep -q "^${key}=" "$TMPDIR_INSTALL/existing.env" 2>/dev/null; then
                printf '%s=%s\n' "$key" "$rest"
                KEYS_COUNT=$((KEYS_COUNT + 1))
            fi
        done < "$TMPDIR_INSTALL/new_flags.env"
    } > "$TMPDIR_INSTALL/merged.env"

    mv "$TMPDIR_INSTALL/merged.env" "$ENV_FILE"
    chown "root:$SERVICE_USER" "$ENV_FILE"
    chmod 0640 "$ENV_FILE"
    log_info "$ENV_FILE updated (${KEYS_COUNT} keys preserved, ${UPDATES} keys updated)"
}

# ── uninstall ─────────────────────────────────────────────────────────────────

uninstall() {
    log_step "Uninstalling Maintenant..."

    # Stop and disable service
    systemctl stop maintenant 2>/dev/null || true
    systemctl disable maintenant 2>/dev/null || true

    # Remove service file
    if [ -f "$SERVICE_FILE" ]; then
        rm -f "$SERVICE_FILE"
        systemctl daemon-reload 2>/dev/null || true
        log_info "Removed $SERVICE_FILE"
    else
        log_warn "Service file not found: $SERVICE_FILE"
    fi

    # Remove binary
    if [ -f "$INSTALL_DIR/maintenant" ]; then
        rm -f "$INSTALL_DIR/maintenant"
        log_info "Removed $INSTALL_DIR/maintenant"
    else
        log_warn "Binary not found: $INSTALL_DIR/maintenant"
    fi

    log_info "Data directory preserved: $DATA_DIR"
    log_info "Config directory preserved: $CONFIG_DIR"
    log_info "User preserved: $SERVICE_USER"

    if [ -n "${DO_PURGE:-}" ]; then
        _purge
    fi

    log_info "Uninstall complete"
}

_purge() {
    # Interactive confirmation only when stdin is a terminal
    if [ -t 0 ]; then
        printf 'This will permanently delete %s, %s, and user %s. Continue? [y/N] ' \
            "$DATA_DIR" "$CONFIG_DIR" "$SERVICE_USER"
        read -r answer
        case "$answer" in
            [yY]|[yY][eE][sS]) ;;
            *) log_info "Purge cancelled"; return ;;
        esac
    fi

    # Remove from docker group
    if getent group docker >/dev/null 2>&1; then
        gpasswd -d "$SERVICE_USER" docker 2>/dev/null || true
    fi

    if [ -d "$DATA_DIR" ]; then
        rm -rf "$DATA_DIR"
        log_info "Removed $DATA_DIR"
    fi
    if [ -d "$CONFIG_DIR" ]; then
        rm -rf "$CONFIG_DIR"
        log_info "Removed $CONFIG_DIR"
    fi

    if userdel "$SERVICE_USER" 2>/dev/null; then
        log_info "Removed user $SERVICE_USER"
    else
        log_warn "Could not remove user $SERVICE_USER"
    fi
}

# ── Utility: HTTP fetch ───────────────────────────────────────────────────────

fetch_url() {
    url="$1"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL --retry 3 --retry-delay 2 --max-time 60 "$url"
    else
        wget -qO- "$url"
    fi
}

fetch_url_to() {
    url="$1"
    dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL --retry 3 --retry-delay 2 --max-time 60 -o "$dest" "$url"
    else
        wget -q -O "$dest" "$url"
    fi
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
    parse_maintenant_flags "$@"

    if [ -n "${DO_UNINSTALL:-}" ]; then
        check_prereqs
        uninstall
        return
    fi

    log_step "Starting Maintenant installation"
    detect_platform
    check_prereqs
    resolve_version
    download_and_verify
    ensure_user
    install_binary

    # Apply binary flags to env file if any were provided
    if [ -n "$BINARY_FLAG_KEYS" ]; then
        log_step "Persisting configuration flags..."
        merge_env_file
    fi

    install_service
    print_summary
}

# Allow sourcing without execution (used by bats tests via _INSTALL_SH_TESTING=1)
[ -n "${_INSTALL_SH_TESTING:-}" ] || main "$@"
