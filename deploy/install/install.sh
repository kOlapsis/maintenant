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
# Exact repository spelling, capital O included: it goes into the certificate
# identity regexp below, and Sigstore matches that case-sensitively. GitHub URLs
# are forgiving about case, which is why a wrong spelling here downloads fine and
# only fails at verification.
GITHUB_REPO="kOlapsis/maintenant"
GITHUB_API="https://api.github.com"
SCRIPT_VERSION="__GIT_SHA__"
NL='
'

# Kept in step with internal/app/flags.go by internal/app/install_script_test.go.
BOOL_FLAGS="proxyLabels disableOsEolRefresh disableTelemetry allowPrivateWebhooks \
mcp mcpAllowUnauthenticated grpc-tls-insecure grpc-insecure-skip-tls-verify embedded-agent \
require-state-dir require-existing-data"
VALUE_FLAGS="addr baseUrl corsOrigins trustedProxies db state-dir sqlite-synchronous organisationName runtime logLevel \
maxBodySize updateInterval securityScoreThreshold licenseKey \
smtpHost smtpPort smtpUsername smtpPassword smtpFrom \
mcpClientId mcpClientSecret mcpAllowedRedirectUris k8sNamespaces k8sExcludeNamespaces \
statusUrl containerDownAfter retentionSnapshots retentionInterval retentionBatchSize \
mode server enrollment-token label nodeName grpc-listen grpc-url grpc-tls-cert grpc-tls-key \
agentRateLimitPerSecond agentStaleThresholdSeconds \
agentSpoolMaxMemoryBytes agentSpoolMaxDiskBytes agentSpoolMaxAgeSeconds \
data-dir ca-cert database-url"

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

_fs() {
    "$@" || abort "Filesystem operation failed: $*" 30
}

# ── Cleanup trap ──────────────────────────────────────────────────────────────

TMPDIR_INSTALL="${TMPDIR_INSTALL:-}"
BINARY_TMP=""
cleanup() {
    if [ -n "${TMPDIR_INSTALL:-}" ]; then rm -rf "$TMPDIR_INSTALL"; fi
    if [ -n "${BINARY_TMP:-}" ]; then rm -f "$BINARY_TMP"; fi
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
  --binary <path>       Install this local binary instead of downloading one
                        (offline install: no network access at all)
  --sha256sums <path>   With --binary: check it against this SHA256SUMS file
  --help, -h            Show this help

Binary configuration flags (written to /etc/maintenant/maintenant.env).
Value flags take "--flag value" or "--flag=value". Boolean flags take "--flag"
or "--flag=true|false". Run "maintenant --help" for what each flag does.
  --addr <host:port>
  --baseUrl <url>
  --corsOrigins <list>
  --trustedProxies <list>
  --db <path>
  --state-dir <path>
  --sqlite-synchronous <NORMAL|FULL>
  --require-state-dir
  --require-existing-data
  --containerDownAfter <duration>
  --retentionSnapshots <duration>
  --retentionInterval <duration>
  --retentionBatchSize <int>
  --organisationName <name>
  --statusUrl <url>
  --runtime <docker|kubernetes|swarm>
  --proxyLabels
  --logLevel <level>
  --maxBodySize <bytes>
  --updateInterval <duration>
  --disableOsEolRefresh
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
  --mcpAllowedRedirectUris <list>
  --mcpAllowUnauthenticated
  --k8sNamespaces <list>
  --k8sExcludeNamespaces <list>
  --mode <embedded|server|agent>
  --server <url>
  --enrollment-token <token>
  --label <name>
  --nodeName <name>
  --grpc-listen <host:port>
  --grpc-url <url>
  --grpc-tls-cert <path>
  --grpc-tls-key <path>
  --grpc-tls-insecure
  --grpc-insecure-skip-tls-verify
  --agentRateLimitPerSecond <int>
  --agentStaleThresholdSeconds <int>
  --agentSpoolMaxMemoryBytes <bytes>
  --agentSpoolMaxDiskBytes <bytes>
  --agentSpoolMaxAgeSeconds <int>
  --embedded-agent
  --ca-cert <path>
  --data-dir <path>
  --database-url <postgres-url>

Examples:
  # Standalone server on this host
  install.sh --addr 0.0.0.0:8080 --baseUrl https://maintenant.example.com

  # Native agent reporting to an existing server
  install.sh --mode agent --server grpcs://maintenant.example.com:8443 \
             --enrollment-token TOKEN --label web-01

  # Offline, from a binary and its SHA256SUMS copied onto this host
  install.sh --binary ./maintenant-v1.2.3-linux-amd64 --sha256sums ./SHA256SUMS

Environment variables:
  MAINTENANT_VERSION       Version to download (default: latest)
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

    if [ -z "${LOCAL_BINARY:-}" ] && ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
        abort "curl or wget is required (or install a local binary with --binary)" 12
    fi

    # No tar: the release assets are bare binaries, not archives.
    for cmd in install useradd; do
        command -v "$cmd" >/dev/null 2>&1 || abort "Required command not found: $cmd" 12
    done

    if [ -z "${NO_SERVICE:-}" ]; then
        command -v systemctl >/dev/null 2>&1 || abort "systemctl is required (use --no-service to skip)" 12
    fi
}

# ── resolve_version ───────────────────────────────────────────────────────────

# ── _release_has_binaries ─────────────────────────────────────────────────────
# True when the release payload carries a linux binary for that tag. Both the
# pinned and the latest paths need it: a release published without its assets —
# a version older than the installer, or a build job that failed after the
# release was created — must fail here with a message, not with a 404 halfway
# through the download.

_release_has_binaries() {
    printf '%s' "$1" | grep -q "\"name\": *\"maintenant-${2}-linux-"
}

resolve_version() {
    VERSION="${MAINTENANT_VERSION:-latest}"

    if [ "$VERSION" = "latest" ]; then
        log_step "Resolving latest version..."
        RELEASE_JSON=$(fetch_url "$GITHUB_API/repos/$GITHUB_REPO/releases/latest" 2>/dev/null) \
            || abort "Failed to reach GitHub Releases" 20
        VERSION=$(printf '%s' "$RELEASE_JSON" \
            | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
        [ -n "$VERSION" ] || abort "Failed to resolve latest version from GitHub API" 20

        if ! _release_has_binaries "$RELEASE_JSON" "$VERSION"; then
            log_error "The latest release ($VERSION) does not include standalone binaries."
            _suggest_versions
            abort "No standalone binaries in the latest release" 20
        fi

        log_info "Latest version: $VERSION"
        return
    fi

    # Pinned version — verify release exists and contains standalone binaries
    log_step "Verifying version $VERSION..."
    RELEASE_JSON=$(fetch_url "$GITHUB_API/repos/$GITHUB_REPO/releases/tags/$VERSION" 2>/dev/null) || {
        _suggest_versions
        abort "Version $VERSION not found on GitHub Releases" 20
    }

    if ! _release_has_binaries "$RELEASE_JSON" "$VERSION"; then
        log_error "Version $VERSION exists but does not include standalone binaries."
        log_error "Standalone binaries were introduced starting from a later release."
        _suggest_versions
        abort "Version $VERSION predates standalone binary support" 20
    fi

    log_info "Version $VERSION verified"
}

_suggest_versions() {
    RECENT_JSON=$(fetch_url "$GITHUB_API/repos/$GITHUB_REPO/releases?per_page=10" 2>/dev/null) || return 0
    RECENT=$(printf '%s' "$RECENT_JSON" \
        | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    [ -n "$RECENT" ] || return 0

    # The heading promises releases you can actually install, so the ones with no
    # binary are filtered out instead of being offered and failing on the retry.
    SUGGESTED=""
    for v in $RECENT; do
        if _release_has_binaries "$RECENT_JSON" "$v"; then
            SUGGESTED="${SUGGESTED} $v"
        fi
    done
    [ -n "$SUGGESTED" ] || return 0

    log_warn "Recent versions with standalone binaries:"
    for v in $SUGGESTED; do
        log_warn "  $v"
    done
}

# ── download_and_verify ───────────────────────────────────────────────────────

# ── _cosign_major ─────────────────────────────────────────────────────────────
# Major version of the cosign on PATH, empty when it cannot be read. cosign 3
# dropped --output-signature and --output-certificate from sign-blob, so a
# release carries a bundle and nothing else; reading that bundle needs a 3.x
# binary too.

_cosign_major() {
    cosign version 2>/dev/null \
        | sed -n 's/^[[:space:]]*GitVersion:[[:space:]]*v\{0,1\}\([0-9]\{1,\}\).*/\1/p' \
        | head -n 1
}

download_and_verify() {
    TMPDIR_INSTALL=$(mktemp -d)
    ASSET_NAME="maintenant-${VERSION}-linux-${ARCH}"
    BASE_URL="https://github.com/$GITHUB_REPO/releases/download/${VERSION}"

    log_step "Downloading $ASSET_NAME..."
    fetch_url_to "$BASE_URL/$ASSET_NAME" "$TMPDIR_INSTALL/$ASSET_NAME" \
        || abort "Failed to download $ASSET_NAME" 20
    fetch_url_to "$BASE_URL/SHA256SUMS" "$TMPDIR_INSTALL/SHA256SUMS" \
        || abort "Failed to download SHA256SUMS" 20

    log_step "Verifying SHA256 checksum..."
    (cd "$TMPDIR_INSTALL" && sha256sum -c SHA256SUMS --ignore-missing) \
        || abort "SHA256 checksum mismatch — download may be corrupted" 21

    if [ -n "${SKIP_COSIGN:-}" ]; then
        log_warn "cosign verification skipped (--skip-cosign)"
    elif ! command -v cosign >/dev/null 2>&1; then
        log_warn "cosign not found — skipping signature verification (install cosign 3 for supply-chain protection)"
    elif COSIGN_MAJOR=$(_cosign_major); [ -z "$COSIGN_MAJOR" ] || [ "$COSIGN_MAJOR" -lt 3 ]; then
        # Not treated as a verification failure: a 2.x binary cannot read the
        # bundle at all, and an unreadable version string says nothing about
        # the signature either.
        log_warn "cosign 3 or later is required to read the release bundle — skipping signature verification"
    else
        fetch_url_to "$BASE_URL/SHA256SUMS.bundle" "$TMPDIR_INSTALL/SHA256SUMS.bundle" \
            || abort "Failed to download SHA256SUMS.bundle" 20
        log_step "Verifying cosign signature..."
        if ! cosign verify-blob \
            --bundle "$TMPDIR_INSTALL/SHA256SUMS.bundle" \
            --certificate-identity-regexp "^https://github\\.com/$GITHUB_REPO/\\.github/workflows/release\\.yml@refs/tags/v[0-9]+\\.[0-9]+\\.[0-9]+\$" \
            --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
            "$TMPDIR_INSTALL/SHA256SUMS"; then
            abort "cosign signature verification failed" 22
        fi
        log_info "cosign signature verified"
    fi

    BINARY_SRC="$TMPDIR_INSTALL/$ASSET_NAME"
}

# ── use_local_binary ──────────────────────────────────────────────────────────

use_local_binary() {
    [ -f "$LOCAL_BINARY" ] || abort "Binary not found: $LOCAL_BINARY" 2
    TMPDIR_INSTALL=$(mktemp -d)
    BINARY_SRC="$LOCAL_BINARY"
    VERSION="local"

    if [ -z "${LOCAL_SUMS:-}" ]; then
        log_warn "No --sha256sums given: the integrity of $LOCAL_BINARY is not checked"
        return
    fi
    [ -f "$LOCAL_SUMS" ] || abort "SHA256SUMS file not found: $LOCAL_SUMS" 2

    log_step "Verifying SHA256 checksum against $LOCAL_SUMS..."
    LOCAL_SUM=$(sha256sum "$LOCAL_BINARY" | cut -d ' ' -f 1)
    MATCHED_ASSET=$(awk -v sum="$LOCAL_SUM" -v suffix="-linux-$ARCH" '
        $1 == sum {
            name = $2
            sub(/^\*/, "", name)
            if (name ~ /^maintenant-/ && substr(name, length(name) - length(suffix) + 1) == suffix) {
                print name
                exit
            }
        }' "$LOCAL_SUMS")
    [ -n "$MATCHED_ASSET" ] \
        || abort "SHA256 of $LOCAL_BINARY matches no linux-$ARCH binary listed in $LOCAL_SUMS" 21

    VERSION="${MATCHED_ASSET#maintenant-}"
    VERSION="${VERSION%-linux-"$ARCH"}"
    log_info "Checksum matches $MATCHED_ASSET"
    log_warn "Offline install: the cosign signature of $LOCAL_SUMS is not verified"
}

# ── ensure_user ───────────────────────────────────────────────────────────────

ensure_user() {
    if id "$SERVICE_USER" >/dev/null 2>&1; then
        log_info "User $SERVICE_USER already exists"
    else
        log_step "Creating system user $SERVICE_USER..."
        useradd -r -s /usr/sbin/nologin -d "$DATA_DIR" \
            -c "Maintenant service user" "$SERVICE_USER" \
            || abort "Failed to create user $SERVICE_USER"
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

# ── resolve_paths ─────────────────────────────────────────────────────────────
# A flag given now wins over the env file, which wins over the defaults.

_env_file_value() {
    [ -f "$2" ] || return 0
    awk -v key="$1" -v q="'" '
        {
            line = $0
            sub(/^[ \t]+/, "", line)
            if (!match(line, /^[A-Za-z_][A-Za-z0-9_]*[ \t]*=/)) next
            k = substr(line, 1, RLENGTH - 1)
            sub(/[ \t]+$/, "", k)
            if (k != key) next
            v = substr(line, RLENGTH + 1)
            sub(/^[ \t]+/, "", v)
            sub(/[ \t]+$/, "", v)
            if (length(v) >= 2 && (v ~ /^".*"$/ || v ~ ("^" q ".*" q "$"))) v = substr(v, 2, length(v) - 2)
            found = v
        }
        END { printf "%s", found }
    ' "$2"
}

_strip_trailing_slashes() {
    p="$1"
    while [ "$p" != "/" ]; do
        case "$p" in
            */) p="${p%/}" ;;
            *) break ;;
        esac
    done
    printf '%s' "$p"
}

resolve_paths() {
    ENV_FILE="$CONFIG_DIR/maintenant.env"

    if _has_binary_flag data-dir; then
        DATA_DIR=$(_binary_flag_value data-dir)
    else
        from_file=$(_env_file_value MAINTENANT_DATA_DIR "$ENV_FILE")
        [ -z "$from_file" ] || DATA_DIR="$from_file"
    fi
    DATA_DIR=$(_strip_trailing_slashes "$DATA_DIR")
    case "$DATA_DIR" in
        /?*) ;;
        *) abort "The data directory must be an absolute path other than /: $DATA_DIR" 2 ;;
    esac

    if _has_binary_flag db; then
        DB_PATH=$(_binary_flag_value db)
    else
        DB_PATH=$(_env_file_value MAINTENANT_DB "$ENV_FILE")
    fi
    [ -n "$DB_PATH" ] || DB_PATH="./maintenant.db"
    while :; do
        case "$DB_PATH" in
            ./*) DB_PATH="${DB_PATH#./}" ;;
            *) break ;;
        esac
    done
    case "$DB_PATH" in
        /*) ;;
        *) DB_PATH="$DATA_DIR/$DB_PATH" ;;
    esac
    DB_DIR=$(dirname "$DB_PATH")

    for unit_path in "$INSTALL_DIR" "$CONFIG_DIR" "$DATA_DIR" "$DB_DIR"; do
        case "$unit_path" in
            *[!A-Za-z0-9._/@+-]*)
                abort "Unsupported character in path (letters, digits and ._/@+- only): $unit_path" 2 ;;
        esac
    done
}

# ── prepare_dirs ──────────────────────────────────────────────────────────────

_refuse_foreign_dir() {
    [ -d "$1" ] || return 0
    [ -z "$(find "$1" -prune -user "$SERVICE_USER" 2>/dev/null)" ] || return 0
    [ -z "$(ls -A "$1")" ] && return 0
    abort "$1 already exists, is not empty and does not belong to $SERVICE_USER: use a dedicated directory, or chown it to $SERVICE_USER first" 30
}

_own_dir() {
    _fs mkdir -p "$1"
    _fs chown "$SERVICE_USER:$SERVICE_USER" "$1"
    _fs chmod 0750 "$1"
}

prepare_dirs() {
    _refuse_foreign_dir "$DATA_DIR"
    _refuse_foreign_dir "$DB_DIR"

    _fs mkdir -p "$CONFIG_DIR"
    _fs chown "root:$SERVICE_USER" "$CONFIG_DIR"
    _fs chmod 0750 "$CONFIG_DIR"

    _own_dir "$DATA_DIR"
    [ "$DB_DIR" = "$DATA_DIR" ] || _own_dir "$DB_DIR"
}

# ── install_binary ────────────────────────────────────────────────────────────

install_binary() {
    log_step "Installing binary..."
    BINARY_TMP=$(mktemp "$INSTALL_DIR/.maintenant.XXXXXX") \
        || abort "Cannot create a temporary file in $INSTALL_DIR" 30
    _fs install -m 0755 -o root -g root "$BINARY_SRC" "$BINARY_TMP"
    _fs mv -f "$BINARY_TMP" "$INSTALL_DIR/maintenant"
    BINARY_TMP=""
    log_info "Binary installed to $INSTALL_DIR/maintenant"
}

# ── install_service ───────────────────────────────────────────────────────────

render_unit() {
    rw_paths="$DATA_DIR"
    [ "$DB_DIR" = "$DATA_DIR" ] || rw_paths="$rw_paths $DB_DIR"
    cat <<UNIT
[Unit]
Description=Maintenant infrastructure monitoring
Documentation=https://docs.maintenant.dev
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
Environment=MAINTENANT_DATA_DIR=$DATA_DIR
EnvironmentFile=-$CONFIG_DIR/maintenant.env
ExecStart=$INSTALL_DIR/maintenant
WorkingDirectory=$DATA_DIR
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$rw_paths
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictNamespaces=true
RestrictRealtime=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
UNIT
}

install_service() {
    if [ -n "${NO_SERVICE:-}" ]; then
        log_info "Skipping service installation (--no-service)"
        if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet maintenant 2>/dev/null; then
            log_warn "maintenant.service still runs the previous binary: systemctl restart maintenant"
        fi
        return
    fi

    log_step "Installing systemd service..."
    render_unit > "$SERVICE_FILE" || abort "Failed to write $SERVICE_FILE" 30

    systemctl daemon-reload || abort "systemctl daemon-reload failed" 31
    systemctl enable maintenant || abort "systemctl enable maintenant failed" 31
    if systemctl is-active --quiet maintenant; then
        log_step "Restarting the service on the new binary..."
        systemctl restart maintenant || abort "systemctl restart maintenant failed" 31
    else
        log_step "Starting the service..."
        systemctl start maintenant || abort "systemctl start maintenant failed" 31
    fi

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
    if _has_binary_flag addr; then
        LISTEN_ADDR=$(_binary_flag_value addr)
    else
        LISTEN_ADDR=$(_env_file_value MAINTENANT_ADDR "$CONFIG_DIR/maintenant.env")
    fi
    [ -n "$LISTEN_ADDR" ] || LISTEN_ADDR="127.0.0.1:8080"
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
# Sets: NO_SERVICE, DO_UNINSTALL, DO_PURGE, SKIP_COSIGN, LOCAL_BINARY, LOCAL_SUMS
# Populates: BINARY_FLAG_KEYS / BINARY_FLAG_VALS, two parallel line lists

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

_binary_flag_line() {
    printf '%s\n' "$BINARY_FLAG_KEYS" | awk -v want="$1" '$0 == want { n = NR } END { print n + 0 }'
}

_has_binary_flag() {
    [ "$(_binary_flag_line "$1")" -gt 0 ]
}

_binary_flag_value() {
    line_no=$(_binary_flag_line "$1")
    [ "$line_no" -gt 0 ] || return 0
    printf '%s\n' "$BINARY_FLAG_VALS" | sed -n "${line_no}p"
}

_in_list() {
    case " $2 " in
        *" $1 "*) return 0 ;;
    esac
    return 1
}

_bool_value() {
    case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
        1|t|true|y|yes|on) printf 'true' ;;
        0|f|false|n|no|off) printf 'false' ;;
        *) return 1 ;;
    esac
}

parse_maintenant_flags() {
    NO_SERVICE=""
    DO_UNINSTALL=""
    DO_PURGE=""
    # Kept from the environment when already set: MAINTENANT_SKIP_COSIGN and the
    # test harness both drive it that way.
    SKIP_COSIGN="${SKIP_COSIGN:-${MAINTENANT_SKIP_COSIGN:-}}"
    LOCAL_BINARY=""
    LOCAL_SUMS=""
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
                case "$FLAG" in
                    *=*) VAL="${FLAG#*=}"; FLAG="${FLAG%%=*}"; INLINE=1 ;;
                    *)   VAL=""; INLINE="" ;;
                esac
                shift

                if [ "$FLAG" = binary ] || [ "$FLAG" = sha256sums ]; then
                    if [ -z "$INLINE" ]; then
                        [ $# -gt 0 ] || abort "Flag --$FLAG requires a path" 2
                        VAL="$1"
                        shift
                    fi
                    [ -n "$VAL" ] || abort "Flag --$FLAG requires a path" 2
                    if [ "$FLAG" = binary ]; then LOCAL_BINARY="$VAL"; else LOCAL_SUMS="$VAL"; fi
                    continue
                fi

                if _in_list "$FLAG" "$BOOL_FLAGS"; then
                    [ -n "$INLINE" ] || VAL=true
                    VAL=$(_bool_value "$VAL") \
                        || abort "Flag --$FLAG takes no value or =true/=false" 2
                elif _in_list "$FLAG" "$VALUE_FLAGS"; then
                    if [ -z "$INLINE" ]; then
                        [ $# -gt 0 ] || abort "Flag --$FLAG requires a value" 2
                        case "$1" in
                            --*) abort "Flag --$FLAG requires a value (write --$FLAG=VALUE for a value starting with --)" 2 ;;
                        esac
                        VAL="$1"
                        shift
                    fi
                    [ -n "$VAL" ] || abort "Flag --$FLAG requires a value" 2
                else
                    abort "Unknown argument: --$FLAG" 2
                fi
                case "$VAL" in
                    *"$NL"*) abort "Flag --$FLAG: the value must fit on one line" 2 ;;
                esac
                _store_binary_flag "$FLAG" "$VAL"
                ;;
            *)
                abort "Unknown argument: $1" 2
                ;;
        esac
    done

    [ -z "$LOCAL_SUMS" ] || [ -n "$LOCAL_BINARY" ] || abort "--sha256sums requires --binary" 2
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
# Key-by-key merge into /etc/maintenant/maintenant.env: the keys passed as flags
# are replaced or appended, every other line of the file is kept as it is.

merge_env_file() {
    ENV_FILE="$CONFIG_DIR/maintenant.env"
    NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    HEADER="# Generated by install.sh
# Last updated: $NOW
# Script version: $SCRIPT_VERSION
# Edit this file directly then: systemctl restart maintenant"

    idx=0
    printf '%s\n' "$BINARY_FLAG_KEYS" | while IFS= read -r flagname; do
        idx=$((idx + 1))
        [ -n "$flagname" ] || continue
        printf '%s=%s\n' "$(flag_to_env "$flagname")" \
            "$(printf '%s\n' "$BINARY_FLAG_VALS" | sed -n "${idx}p")"
    done > "$TMPDIR_INSTALL/new_flags.env"

    if [ ! -f "$ENV_FILE" ]; then
        {
            printf '%s\n\n' "$HEADER"
            cat "$TMPDIR_INSTALL/new_flags.env"
        } > "$TMPDIR_INSTALL/merged.env" || abort "Cannot write $TMPDIR_INSTALL/merged.env" 30
        _install_env_file
        log_info "Created $ENV_FILE"
        return
    fi

    {
        printf '%s\n\n' "$HEADER"
        awk -v counts="$TMPDIR_INSTALL/merge.counts" '
            BEGIN { head = 1 }
            FILENAME == ARGV[1] {
                i = index($0, "=")
                if (i > 1) {
                    k = substr($0, 1, i - 1)
                    if (!(k in val)) order[++n] = k
                    val[k] = substr($0, i + 1)
                }
                next
            }
            head && /^# (Generated by install\.sh$|Last updated: |Script version: |Edit this file directly then: )/ { generated = 1; next }
            head && generated && /^[ \t]*$/ { head = 0; next }
            { head = 0 }
            {
                line = $0
                sub(/^[ \t]+/, "", line)
                if (match(line, /^[A-Za-z_][A-Za-z0-9_]*[ \t]*=/)) {
                    k = substr(line, 1, RLENGTH - 1)
                    sub(/[ \t]+$/, "", k)
                    if (k in val) {
                        print k "=" val[k]
                        written[k] = 1
                        updated++
                        next
                    }
                }
                print
            }
            END {
                for (j = 1; j <= n; j++) {
                    if (!(order[j] in written)) {
                        print order[j] "=" val[order[j]]
                        added++
                    }
                }
                print (updated + 0) " " (added + 0) > counts
            }
        ' "$TMPDIR_INSTALL/new_flags.env" "$ENV_FILE"
    } > "$TMPDIR_INSTALL/merged.env" || abort "Cannot merge into $ENV_FILE" 30

    read -r UPDATED ADDED < "$TMPDIR_INSTALL/merge.counts"
    _install_env_file
    log_info "$ENV_FILE updated (${UPDATED} keys replaced, ${ADDED} keys added, other lines kept)"
}

_install_env_file() {
    _fs chown "root:$SERVICE_USER" "$TMPDIR_INSTALL/merged.env"
    _fs chmod 0640 "$TMPDIR_INSTALL/merged.env"
    _fs mv -f "$TMPDIR_INSTALL/merged.env" "$ENV_FILE"
}

# ── uninstall ─────────────────────────────────────────────────────────────────

uninstall() {
    log_step "Uninstalling Maintenant..."
    resolve_paths

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
    case "$DB_DIR/" in
        "$DATA_DIR"/*) ;;
        *) [ ! -d "$DB_DIR" ] || log_warn "Database directory kept, it lies outside $DATA_DIR: $DB_DIR" ;;
    esac
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
    resolve_paths
    if [ -n "$LOCAL_BINARY" ]; then
        use_local_binary
    else
        resolve_version
        download_and_verify
    fi
    ensure_user
    prepare_dirs

    # Apply binary flags to env file if any were provided
    if [ -n "$BINARY_FLAG_KEYS" ]; then
        log_step "Persisting configuration flags..."
        merge_env_file
    fi

    install_binary
    install_service
    print_summary
}

# Allow sourcing without execution (used by bats tests via _INSTALL_SH_TESTING=1)
[ -n "${_INSTALL_SH_TESTING:-}" ] || main "$@"
